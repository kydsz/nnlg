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

	epoch := cache.GetClient().GetSessionEpoch(c.Request.Context(), user.ID)
	accessToken, err := jwtutil.SignAccess(h.cfg.SecretKey, int64(user.ID), h.cfg.TokenExpireMinutes, epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成令牌失败")
		return
	}
	refreshToken, err := jwtutil.SignRefresh(h.cfg.SecretKey, int64(user.ID), h.cfg.RefreshTokenExpireDays, jwtutil.NewJTI(), epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成刷新令牌失败")
		return
	}

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

	// 会话撤销校验：登出/禁用/改密会自增会话 epoch，旧 refresh_token 立即失效
	rdb := cache.GetClient()
	if rdb != nil && rdb.Enabled {
		if epoch := rdb.GetSessionEpoch(c.Request.Context(), uid); epoch != info.Epoch {
			response.Fail(c, http.StatusUnauthorized, "刷新令牌已失效")
			return
		}
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

	accessToken, err := jwtutil.SignAccess(h.cfg.SecretKey, int64(user.ID), h.cfg.TokenExpireMinutes, info.Epoch)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成令牌失败")
		return
	}
	h.setAccessCookie(c, accessToken, h.cfg.TokenExpireMinutes*60)

	// 轮换 refresh_token（同 epoch，新 jti）
	newRefresh, err := jwtutil.SignRefresh(h.cfg.SecretKey, int64(user.ID), h.cfg.RefreshTokenExpireDays, jwtutil.NewJTI(), info.Epoch)
	if err == nil {
		h.setRefreshCookie(c, newRefresh, h.cfg.RefreshTokenExpireDays*24*3600)
	}

	response.OKMsg(c, "刷新成功", gin.H{
		"access_token": accessToken,
		"token_type":   "bearer",
		"user":         userPayload(h.db, user),
	})
}

// Logout 登出：清除 cookie，并撤销该用户全部会话（自增 epoch + 删快照）。
func (h *Auth) Logout(c *gin.Context) {
	if uid, ok := currentUID(c, h.cfg.SecretKey); ok {
		cache.IncrSessionEpoch(c.Request.Context(), uid)
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
	cache.IncrSessionEpoch(c.Request.Context(), u.ID)
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
