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

		// 会话撤销检查（Redis 启用时）：登出/禁用/改密会自增 epoch，旧 token 立即失效
		if rdb := cache.GetClient(); rdb != nil && rdb.Enabled {
			if epoch := rdb.GetSessionEpoch(c.Request.Context(), uid); epoch != info.Epoch {
				abortAuth(c, "会话已失效，请重新登录")
				return
			}
		}

		c.Set(ctxUserKey, user)
		c.Next()
	}
}

func abortAuth(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, response.Body{
		Code: http.StatusUnauthorized, Message: msg, Data: nil,
	})
}
