package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func corsRouter(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(origins))
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	return r
}

// 默认空配置=同源：外部 Origin 不回显凭证 CORS 头，但响应含 Vary: Origin。
func TestCORSDefaultSameOrigin(t *testing.T) {
	r := corsRouter(nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("默认配置不应回显 Allow-Origin, 实际 %q", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal("应响应 Vary: Origin")
	}
}

// 白名单外 Origin 不回显；白名单内回显且携带 Vary: Origin。
func TestCORSWhitelist(t *testing.T) {
	r := corsRouter([]string{"https://app.example.com", "https://admin.example.com"})

	// 不在白名单
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("非白名单 Origin 不应回显, 实际 %q", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal("应响应 Vary: Origin")
	}

	// 在白名单
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req2.Header.Set("Origin", "https://app.example.com")
	r.ServeHTTP(w2, req2)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("白名单 Origin 应回显自身, 实际 %q", got)
	}
	if w2.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("应携带凭证 CORS 头")
	}
	if w2.Header().Get("Vary") != "Origin" {
		t.Fatal("应响应 Vary: Origin")
	}
}

// 显式 "*" 才放行任意源（早先空配置即 allowAll 的隐式宽松已禁止）。
func TestCORSExplicitStar(t *testing.T) {
	r := corsRouter([]string{"*"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.example.com" {
		t.Fatalf("显式 * 应回显任意 Origin, 实际 %q", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal("应响应 Vary: Origin")
	}
}

// 白名单外 Origin 的 OPTIONS 预检：返回 204 但不回显 CORS 头，
// 浏览器据此判定跨域失败拦截实际请求，符合"白名单外不回显"。
func TestCORSPreflightRejectedOutsideWhitelist(t *testing.T) {
	r := corsRouter([]string{"https://allowed.example.com"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("预检应返回 204, 实际 %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("白名单外预检不应回显 Allow-Origin, 实际 %q", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal("应响应 Vary: Origin")
	}
}

// 无 Origin 头（同源/非浏览器场景）：不加 CORS 头，正常处理请求。
func TestCORSNoOriginHeader(t *testing.T) {
	r := corsRouter([]string{"https://allowed.example.com"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("无 Origin 不应回显 CORS 头, 实际 %q", got)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("无 Origin 请求应正常处理, 实际 %d", w.Code)
	}
}
