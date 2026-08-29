package middleware

import (
	"net/http"
	"strings"

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

// Auth 认证中间件：token 优先取 cookie "token"，其次 Authorization: Bearer
func Auth(secret string, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := c.Cookie("token")
		if token == "" {
			if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
				token = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Body{
				Code: http.StatusUnauthorized, Message: "未提供认证凭证", Data: nil,
			})
			return
		}

		uid, err := jwtutil.Parse(secret, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Body{
				Code: http.StatusUnauthorized, Message: "无效的认证凭证", Data: nil,
			})
			return
		}

		var user model.User
		if err := db.
			Preload("UserRoles").
			Preload("UserColleges.College").
			Preload("UserRooms.ResearchRoom.College").
			Preload("College").
			Preload("ResearchRoom").
			First(&user, uid).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Body{
				Code: http.StatusUnauthorized, Message: "无效的认证凭证", Data: nil,
			})
			return
		}

		if user.Status != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, response.Body{
				Code: http.StatusForbidden, Message: "用户已被禁用", Data: nil,
			})
			return
		}

		c.Set(ctxUserKey, &user)
		c.Next()
	}
}
