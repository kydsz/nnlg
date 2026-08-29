package middleware

import (
	"net/http"

	"backend-go/internal/model"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func forbidden(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, response.Body{
		Code: http.StatusForbidden, Message: "权限不足", Data: nil,
	})
}

// RequirePermission 权限码检查（需全部满足）
func RequirePermission(db *gorm.DB, required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil {
			forbidden(c)
			return
		}
		// 系统管理员恒拥有最高权限，识别到即放行
		if user.HasRole(model.RoleSystemAdmin) {
			c.Next()
			return
		}
		perms := user.Permissions(db)
		permSet := map[string]bool{}
		for _, p := range perms {
			permSet[p] = true
		}
		for _, r := range required {
			if !permSet[r] {
				forbidden(c)
				return
			}
		}
		c.Next()
	}
}

// RequireAnyPermission 权限码检查（满足任一即可）
func RequireAnyPermission(db *gorm.DB, allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil {
			forbidden(c)
			return
		}
		// 系统管理员恒拥有最高权限，识别到即放行
		if user.HasRole(model.RoleSystemAdmin) {
			c.Next()
			return
		}
		perms := user.Permissions(db)
		for _, p := range perms {
			for _, a := range allowed {
				if p == a {
					c.Next()
					return
				}
			}
		}
		forbidden(c)
	}
}

// RequireRole 角色检查（满足任一即可）
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil || !user.HasAnyRole(roles...) {
			forbidden(c)
			return
		}
		c.Next()
	}
}
