package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件。credentials 模式下不能返回 *，需回显具体 Origin。
// 默认收紧：未配置 origins 视为同源（不对外部 Origin 回显凭证 CORS 头）；
// 仅显式包含 "*" 时才回显任意 Origin。禁止"空配置即放行全部"的隐式宽松。
func CORS(origins []string) gin.HandlerFunc {
	allowAll := false
	allowSet := map[string]bool{}
	for _, o := range origins {
		if o == "*" {
			allowAll = true
		}
		allowSet[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		// Vary: Origin 告知缓存按 Origin 区分响应（无论是回显还是被拒），
		// 防止 CDN/代理把 A 源的回显复用给 B 源（后者本应被拒或不同响应）。
		c.Header("Vary", "Origin")
		if origin != "" && (allowAll || allowSet[origin]) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
			c.Header("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
