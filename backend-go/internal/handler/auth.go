package handler

import (
	"errors"
	"net/http"
	"time"

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

// Login 登录：返回 token 并写入 HttpOnly cookie
func (h *Auth) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "请求参数错误")
		return
	}

	user, err := h.svc.Login(h.db, req.UserNo, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserDisabled) {
			response.Fail(c, http.StatusForbidden, err.Error())
		} else {
			response.Fail(c, http.StatusUnauthorized, err.Error())
		}
		return
	}

	now := time.Now()
	// Py 端 SQLAlchemy onupdate 会同时刷新 update_time，这里对齐；
	// Omit(clause.Associations)：user 预加载了关联，避免 Update 触发关联 upsert 多余写入
	_ = h.db.Model(&model.User{}).Where("id = ?", user.ID).Omit(clause.Associations).Updates(map[string]interface{}{"last_login_time": now, "update_time": now}).Error
	user.LastLoginTime = model.LocalTimePtr(now)
	user.UpdateTime = model.LocalTimePtr(now)

	token, err := jwtutil.Sign(h.cfg.SecretKey, int64(user.ID), h.cfg.TokenExpireMinutes)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "生成令牌失败")
		return
	}

	h.setTokenCookie(c, token, h.cfg.TokenExpireMinutes*60)
	response.OKMsg(c, "登录成功", gin.H{
		"token_type": "bearer",
		"user":       userPayload(h.db, user),
	})
}

// Logout 登出：清除 cookie
func (h *Auth) Logout(c *gin.Context) {
	h.setTokenCookie(c, "", -1)
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

// ChangePassword 修改密码
func (h *Auth) ChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	if err := h.svc.ChangePassword(h.db, middleware.CurrentUser(c), req.OldPassword, req.NewPassword); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OKMsg(c, "密码修改成功", nil)
}

func (h *Auth) setTokenCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", token, maxAge, "/api/v1", "", h.cfg.CookieSecure, true)
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

	return gin.H{
		"id":                   u.ID,
		"user_no":              u.UserNo,
		"username":             u.Username,
		"role":                 u.Role,
		"roles":                ensureList(u.RoleCodes()),
		"permissions":          ensureList(u.Permissions(db)),
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
