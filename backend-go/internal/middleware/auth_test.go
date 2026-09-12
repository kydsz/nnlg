package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend-go/internal/model"
	"backend-go/pkg/jwtutil"

	"github.com/gin-gonic/gin"
)

// 说明：Auth 的 DB 查询路径（用户不存在 401 / 禁用 403 / 正常放行）依赖真实数据库，
// 由集成测试覆盖；本文件只覆盖无需 DB 的分支：token 提取、cookie > Bearer 优先级、
// 非法/过期 token 的 401 行为，以及 CurrentUser 辅助函数。
const testSecret = "unit-test-secret-16"

func newAuthRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/me", Auth(testSecret, nil), func(c *gin.Context) {
		u := CurrentUser(c)
		c.JSON(http.StatusOK, gin.H{"id": u.ID})
	})
	return r
}

func do(t *testing.T, r *gin.Engine, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	r.ServeHTTP(w, req)
	return w
}

func bodyMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("响应体不是统一格式 JSON: %v, body=%s", err, w.Body.String())
	}
	return b.Message
}

func TestAuthNoToken(t *testing.T) {
	r := newAuthRouter()
	w := do(t, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401, 实际 %d", w.Code)
	}
	if msg := bodyMessage(t, w); msg != "未提供认证凭证" {
		t.Fatalf("message 不符: %s", msg)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	r := newAuthRouter()
	w := do(t, r, &http.Cookie{Name: "token", Value: "garbage.token.value"})
	if w.Code != http.StatusUnauthorized || bodyMessage(t, w) != "无效的认证凭证" {
		t.Fatalf("期望 401 无效的认证凭证, 实际 %d %s", w.Code, w.Body.String())
	}
}

func TestAuthExpiredToken(t *testing.T) {
	r := newAuthRouter()
	token, _ := jwtutil.SignAccess(testSecret, 1, -1, 0)
	w := do(t, r, &http.Cookie{Name: "token", Value: token})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("过期 token 应 401, 实际 %d", w.Code)
	}
}

// 空 Bearer 头（无 token 内容）不应被视为已提供凭证
func TestAuthEmptyBearer(t *testing.T) {
	r := newAuthRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer ")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || bodyMessage(t, w) != "未提供认证凭证" {
		t.Fatalf("空 Bearer 应 401 未提供认证凭证, 实际 %d %s", w.Code, w.Body.String())
	}
}

// 非 Bearer 前缀的 Authorization 头应被忽略
func TestAuthNonBearerAuthorization(t *testing.T) {
	r := newAuthRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Basic 认证头应被忽略, 实际 %d", w.Code)
	}
}

// cookie 无效 + Bearer 有效（格式上合法但走不到 DB）也无法区分时，
// 用「无效 cookie 存在时 Bearer 不参与」验证 cookie 优先：
// 即使 Bearer 是合法 token，无效 cookie 仍导致 401 且走不到用户查询。
// （此用例通过 nil DB 断言：若误用了 Bearer token，会在 DB 查询处 panic，
// 测试即失败，从而证明 cookie 优先级正确。）
func TestAuthCookieTakesPrecedenceOverBearer(t *testing.T) {
	r := newAuthRouter()
	bearerToken, _ := jwtutil.SignAccess(testSecret, 1, 30, 0)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "invalid"})
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("cookie 存在时不应回退 Bearer, 实际 %d", w.Code)
	}
}

// TestMustChangePasswordBlocked 初始口令未修改时：除改密/查看自己/登出外一律拦截
func TestMustChangePasswordBlocked(t *testing.T) {
	pending := &model.User{MustChangePassword: true}
	done := &model.User{MustChangePassword: false}

	if MustChangePasswordBlocked(nil, "/api/v1/stats/overview") {
		t.Fatal("未认证（nil 用户）不应由此函数拦截")
	}
	if MustChangePasswordBlocked(done, "/api/v1/stats/overview") {
		t.Fatal("已改密用户不应被拦截")
	}
	if !MustChangePasswordBlocked(pending, "/api/v1/stats/overview") {
		t.Fatal("未改密用户访问业务接口应被拦截")
	}
	if !MustChangePasswordBlocked(pending, "/api/v1/users") {
		t.Fatal("未改密用户访问用户管理应被拦截")
	}
	for _, p := range []string{"/api/v1/auth/password", "/api/v1/auth/me", "/api/v1/auth/logout"} {
		if MustChangePasswordBlocked(pending, p) {
			t.Fatalf("白名单接口 %s 应放行，否则用户既改不了密码也退不出去", p)
		}
	}
}

func TestCurrentUserWithoutAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/who", func(c *gin.Context) {
		if CurrentUser(c) != nil {
			t.Error("未经过 Auth 时 CurrentUser 应为 nil")
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/who", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("路由异常: %d", w.Code)
	}
}
