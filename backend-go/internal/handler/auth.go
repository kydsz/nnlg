package handler

import (
	"errors"
	"net/http"
	"time"

	"backend-go/internal/cache"
	"backend-go/internal/config"
	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/jwtutil"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const timeFormat = "2006-01-02 15:04:05"

// 确保空列表序列化为 [] 而不是 null
func ensureList[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// Auth 认证接口
type Auth struct {
	cfg *config.Config
	db  *gorm.DB
	svc *service.Auth
}

func NewAuth(cfg *config.Config, db *gorm.DB, svc *service.Auth) *Auth {
	return &Auth{cfg: cfg, db: db, svc: svc}
}

type loginReq struct {
	UserNo   string `json:"user_no" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 登录：写 access + refresh 两个 HttpOnly cookie，返回 access_token 与用户信息。
func (h *Auth) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	ip := clientIP(c)
	if !loginIPLimiter.allow(ip) {
		response.Fail(c, http.StatusTooManyRequests, "登录过于频繁，请稍后再试")
		return
	}
	if loginUserLimiter.blocked(req.UserNo) {
		response.Fail(c, http.StatusTooManyRequests, "该账号登录失败次数过多，请稍后再试")
		return
	}

	user, err := h.svc.Login(h.db, req.UserNo, req.Password)
	if err != nil {
		loginUserLimiter.recordFailure(req.UserNo)
		if errors.Is(err, service.ErrUserDisabled) {
			response.Fail(c, http.StatusForbidden, err.Error())
		} else {
			response.Fail(c, http.StatusUnauthorized, err.Error())
		}
		return
	}
	loginUserLimiter.reset(req.UserNo)

	now := time.Now()
	// Py 端 SQLAlchemy onupdate 会同时刷新 update_time，这里对齐；
	// Omit(clause.Associations)：user 预加载了关联，避免 Update 触发关联 upsert 多余写入
	_ = h.db.Model(&model.User{}).Where("id = ?", user.ID).Omit(clause.Associations).Updates(map[string]interface{}{"last_login_time": now, "update_time": now}).Error
	user.LastLoginTime = model.LocalTimePtr(now)
	user.UpdateTime = model.LocalTimePtr(now)
	invalidateUserAuth(c, user.ID)

	epoch := user.SessionEpoch
	accessToken, err := jwtutil.SignAccess(h.cfg.SecretKey, int64(user.ID), h.cfg.TokenExpireMinutes, epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成令牌失败")
		return
	}
	refreshJTI, err := jwtutil.NewJTI()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成刷新令牌失败")
		return
	}
	refreshToken, err := jwtutil.SignRefresh(h.cfg.SecretKey, int64(user.ID), h.cfg.RefreshTokenExpireDays, refreshJTI, epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成刷新令牌失败")
		return
	}
	// 记录当前有效 refresh jti（DB 权威），供后续轮换与重放检测
	_ = h.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"refresh_jti":      refreshJTI,
		"refresh_jti_prev": "",
	}).Error
	// 同步 Redis epoch 加速缓存：避免升级前残留的陈旧缓存凌驾 DB 权威
	cache.SyncUserEpoch(c.Request.Context(), h.db, user.ID)

	h.setAccessCookie(c, accessToken, h.cfg.TokenExpireMinutes*60)
	h.setRefreshCookie(c, refreshToken, h.cfg.RefreshTokenExpireDays*24*3600)
	response.OKMsg(c, "登录成功", gin.H{
		"access_token": accessToken,
		"token_type":   "bearer",
		"user":         userPayload(h.db, user),
	})
}

// Refresh 用 refresh_token cookie 换取新的 access_token（可选轮换 refresh_token）。
func (h *Auth) Refresh(c *gin.Context) {
	refreshToken, _ := c.Cookie("refresh_token")
	if refreshToken == "" {
		response.Fail(c, http.StatusUnauthorized, "缺少刷新令牌")
		return
	}
	info, err := jwtutil.ParseRefresh(h.cfg.SecretKey, refreshToken)
	if err != nil {
		response.Fail(c, http.StatusUnauthorized, "刷新令牌无效")
		return
	}
	uid := int(info.UserID)

	// 会话撤销校验（DB 权威，Redis 仅加速）：登出/禁用/改密自增 epoch，旧 refresh 立即失效。
	// DB 读取失败 fail-closed（拒绝刷新），不静默放行。
	epoch, err := cache.GetSessionEpoch(c.Request.Context(), h.db, uid)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "会话状态校验失败，请稍后重试")
		return
	}
	if epoch != info.Epoch {
		response.Fail(c, http.StatusUnauthorized, "刷新令牌已失效")
		return
	}

	user, err := cache.LoadUser(c.Request.Context(), h.db, uid)
	if err != nil {
		response.Fail(c, http.StatusUnauthorized, "刷新令牌无效")
		return
	}
	if user.Status != 1 {
		response.Fail(c, http.StatusUnauthorized, "用户已被禁用")
		return
	}

	// refresh token 轮换与重放检测（DB 权威）：
	// refresh_jti 为当前有效 jti；轮换时旧 jti 移入 refresh_jti_prev。
	// 再次使用 refresh_jti_prev 判定为重放/盗用，撤销该用户全部会话。
	// 并发安全：条件 UPDATE（WHERE refresh_jti=旧值）保证同一 jti 只能轮换一次。
	var cur, prev string
	_ = h.db.Model(&model.User{}).Where("id = ?", uid).Select("refresh_jti", "refresh_jti_prev").Row().Scan(&cur, &prev)
	if info.JTI == prev {
		// 旧 jti 重放：检测到令牌重用，撤销全部会话
		cache.IncrSessionEpoch(c.Request.Context(), h.db, uid)
		response.Fail(c, http.StatusUnauthorized, "检测到令牌重用，会话已撤销，请重新登录")
		return
	}
	// 存量兼容：升级前未记录 jti（cur/prev 均为空）的用户，首次刷新直接放行并轮换
	if info.JTI != cur && cur != "" {
		response.Fail(c, http.StatusUnauthorized, "刷新令牌已失效")
		return
	}
	newJTI, err := jwtutil.NewJTI()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "刷新失败，请重试")
		return
	}
	res := h.db.Model(&model.User{}).
		Where("id = ? AND refresh_jti = ?", uid, cur).
		Updates(map[string]interface{}{
			"refresh_jti":      newJTI,
			"refresh_jti_prev": info.JTI,
		})
	if res.Error != nil {
		response.Fail(c, http.StatusInternalServerError, "刷新失败，请重试")
		return
	}
	if res.RowsAffected == 0 {
		// 并发窗口：同一 jti 已被并发请求轮换 → 拒绝，避免旧 refresh 并行续期
		response.Fail(c, http.StatusUnauthorized, "刷新令牌已失效")
		return
	}

	accessToken, err := jwtutil.SignAccess(h.cfg.SecretKey, int64(user.ID), h.cfg.TokenExpireMinutes, epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成令牌失败")
		return
	}
	h.setAccessCookie(c, accessToken, h.cfg.TokenExpireMinutes*60)

	// 轮换 refresh_token（同 epoch，新 jti）
	newRefresh, err := jwtutil.SignRefresh(h.cfg.SecretKey, int64(user.ID), h.cfg.RefreshTokenExpireDays, newJTI, epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成刷新令牌失败")
		return
	}
	h.setRefreshCookie(c, newRefresh, h.cfg.RefreshTokenExpireDays*24*3600)

	response.OKMsg(c, "刷新成功", gin.H{
		"access_token": accessToken,
		"token_type":   "bearer",
		"user":         userPayload(h.db, user),
	})
}

// Logout 登出：清除 cookie，并撤销该用户全部会话（自增 epoch + 删快照）。
func (h *Auth) Logout(c *gin.Context) {
	if uid, ok := currentUID(c, h.cfg.SecretKey); ok {
		// DB 撤销失败时 fail-closed：报告登出失败，避免用户在"以为已登出"时令牌仍有效
		if _, err := cache.IncrSessionEpoch(c.Request.Context(), h.db, uid); err != nil {
			response.Fail(c, http.StatusInternalServerError, "登出失败，请稍后重试")
			return
		}
	}
	h.setAccessCookie(c, "", -1)
	h.setRefreshCookie(c, "", -1)
	response.OKMsg(c, "登出成功", nil)
}

// Me 当前用户信息
func (h *Auth) Me(c *gin.Context) {
	response.OK(c, userPayload(h.db, middleware.CurrentUser(c)))
}

type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword 修改密码（成功后撤销该用户全部会话，需重新登录）。
func (h *Auth) ChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)
	if err := h.svc.ChangePassword(h.db, u, req.OldPassword, req.NewPassword); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// 清理认证快照后刷新（changePassword 之前 u 来自快照，密码不携带）
	invalidateUserAuth(c, u.ID)
	if _, err := cache.IncrSessionEpoch(c.Request.Context(), h.db, u.ID); err != nil {
		response.Fail(c, http.StatusInternalServerError, "修改成功，但会话撤销失败，请重新登录")
		return
	}
	response.OKMsg(c, "密码修改成功", nil)
}

func (h *Auth) setAccessCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", token, maxAge, "/api/v1", "", h.cfg.CookieSecure, true)
}

func (h *Auth) setRefreshCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", token, maxAge, "/api/v1", "", h.cfg.CookieSecure, true)
}

// currentUID 从请求的 access/refresh token 中解析用户 ID（登出时无需认证中间件）。
func currentUID(c *gin.Context, secret string) (int, bool) {
	if t, _ := c.Cookie("token"); t != "" {
		if info, err := jwtutil.ParseAccess(secret, t); err == nil {
			return int(info.UserID), true
		}
	}
	if t, _ := c.Cookie("refresh_token"); t != "" {
		if info, err := jwtutil.ParseRefresh(secret, t); err == nil {
			return int(info.UserID), true
		}
	}
	return 0, false
}

// userPayload 组装登录/me 接口的用户数据，字段与旧后端一致
func userPayload(db *gorm.DB, u *model.User) gin.H {
	supervisorColleges := []gin.H{}
	for _, uc := range u.UserColleges {
		if uc.College != nil {
			supervisorColleges = append(supervisorColleges, gin.H{
				"college_id":   uc.CollegeID,
				"college_name": uc.College.Name,
			})
		}
	}

	perms := u.Permissions(db)
	if u.HasRole(model.RoleSystemAdmin) {
		perms = service.AllPermissionCodes() // 系统管理员恒拥有全部权限
	}

	return gin.H{
		"id":                   u.ID,
		"user_no":              u.UserNo,
		"username":             u.Username,
		"role":                 u.Role,
		"roles":                ensureList(u.RoleCodes()),
		"permissions":          ensureList(perms),
		"college_id":           u.CollegeID,
		"college_name":         collegeName(u.College),
		"research_room_id":     u.ResearchRoomID,
		"research_room_name":   roomName(u.ResearchRoom),
		"status":               u.Status,
		"must_change_password": u.MustChangePassword,
		"last_login_time":      u.LastLoginTime,
		"create_time":          u.CreateTime,
		"update_time":          u.UpdateTime,
		"supervisor_colleges":  supervisorColleges,
	}
}
