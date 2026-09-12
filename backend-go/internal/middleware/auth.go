package middleware

import (
	"net/http"
	"strings"

	"backend-go/internal/cache"
	"backend-go/internal/model"
	"backend-go/pkg/jwtutil"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const ctxUserKey = "currentUser"

// CurrentUser 从上下文取当前登录用户（需先经过 Auth）
func CurrentUser(c *gin.Context) *model.User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*model.User)
	return u
}

// Auth 认证中间件：token 优先取 cookie "token"，其次 Authorization: Bearer。
// 访问用户快照缓存（未命中回源 DB），并校验 token_type=access、用户启用、会话 epoch。
func Auth(secret string, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := c.Cookie("token")
		if token == "" {
			if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
				token = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if token == "" {
			abortAuth(c, "未提供认证凭证")
			return
		}

		info, err := jwtutil.ParseAccess(secret, token)
		if err != nil {
			abortAuth(c, "无效的认证凭证")
			return
		}

		uid := int(info.UserID)
		user, err := cache.LoadUser(c.Request.Context(), db, uid)
		if err != nil {
			abortAuth(c, "无效的认证凭证")
			return
		}

		if user.Status != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Body{
				Code: http.StatusForbidden, Message: "用户已被禁用", Data: nil,
			})
			return
		}

		// 会话撤销检查（DB 权威，Redis 仅加速）：登出/禁用/改密自增 epoch，旧 token 立即失效。
		// DB 读取失败按 fail-closed 处理（拒绝访问），不得因撤销源不可用而放行。
		epoch, err := cache.GetSessionEpoch(c.Request.Context(), db, uid)
		if err != nil {
			abortAuth(c, "会话状态校验失败，请稍后重试")
			return
		}
		if epoch != info.Epoch {
			abortAuth(c, "会话已失效，请重新登录")
			return
		}

		// 初始口令未修改：除白名单外一律拦截，防止绕过强制改密继续使用系统。
		if MustChangePasswordBlocked(user, c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Body{
				Code:    http.StatusForbidden,
				Message: "首次登录须先修改初始密码",
				// 机器可读标记：前端据此直接唤起改密弹窗，而不是只弹一条错误提示
				Data: gin.H{"must_change_password": true},
			})
			return
		}

		c.Set(ctxUserKey, user)
		c.Next()
	}
}

// mustChangePasswordAllow 未修改初始口令时仍允许访问的接口：
// 改密本身、读取自己的信息、登出（否则用户既改不了密码也退不出去）。
var mustChangePasswordAllow = map[string]bool{
	"/api/v1/auth/password": true,
	"/api/v1/auth/me":       true,
	"/api/v1/auth/logout":   true,
}

// MustChangePasswordBlocked 判断该请求是否应被「强制修改初始口令」拦截。
// 导出以便单测；u 为 nil（未认证）时不拦截，交由认证流程处理。
func MustChangePasswordBlocked(u *model.User, path string) bool {
	return u != nil && u.MustChangePassword && !mustChangePasswordAllow[path]
}

func abortAuth(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, response.Body{
		Code: http.StatusUnauthorized, Message: msg, Data: nil,
	})
}
