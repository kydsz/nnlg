package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"
)

// loginAndExtract 登录并返回 access_token 与 refresh cookie 值。
func loginAndExtract(t *testing.T, env *testEnv, userNo, password string) (accessToken, refreshToken string) {
	t.Helper()
	w := env.do(t, http.MethodPost, "/api/v1/auth/login", `{"user_no":"`+userNo+`","password":"`+password+`"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("登录状态码=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != 200 || resp.Data.AccessToken == "" {
		t.Fatalf("登录响应缺 access_token: %+v", resp)
	}
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "refresh_token" {
			refreshToken = ck.Value
		}
	}
	if refreshToken == "" {
		t.Fatal("登录响应缺 refresh_token cookie")
	}
	return resp.Data.AccessToken, refreshToken
}

func bearerHeader(token string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	return h
}

func cookieHeader(name, value string) http.Header {
	h := http.Header{}
	h.Set("Cookie", name+"="+value)
	return h
}

func TestLogoutRevokesTokensWithoutRedis(t *testing.T) {
	env := newTestServer(t) // RedisAddr 为空 → Redis 未启用
	hash, _ := pwd.Hash("secret123")
	env.seedUser(t, &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1})

	access, refresh := loginAndExtract(t, env, "T001", "secret123")
	w := env.do(t, http.MethodGet, "/api/v1/auth/me", "", bearerHeader(access))
	if w.Code != http.StatusOK {
		t.Fatalf("登出前 /me 应 200, 实际 %d body=%s", w.Code, w.Body.String())
	}

	// 登出（无需认证，走 cookie 解析）
	env.do(t, http.MethodPost, "/api/v1/auth/logout", "", cookieHeader("token", access))

	// 旧 access_token 必须立即失效（Redis 未启用时 DB 兜底）
	w2 := env.do(t, http.MethodGet, "/api/v1/auth/me", "", bearerHeader(access))
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("登出后旧 access_token 应 401, 实际 %d body=%s", w2.Code, w2.Body.String())
	}

	// 旧 refresh_token 同样失效
	w3 := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", cookieHeader("refresh_token", refresh))
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("登出后旧 refresh_token 应 401, 实际 %d body=%s", w3.Code, w3.Body.String())
	}
}

func TestChangePasswordInvalidatesOldTokens(t *testing.T) {
	env := newTestServer(t)
	hash, _ := pwd.Hash("old-password")
	env.seedUser(t, &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1})

	access, _ := loginAndExtract(t, env, "T001", "old-password")
	w := env.do(t, http.MethodPost, "/api/v1/auth/password",
		`{"old_password":"old-password","new_password":"new-password"}`, bearerHeader(access))
	if w.Code != http.StatusOK {
		t.Fatalf("改密应 200, 实际 %d body=%s", w.Code, w.Body.String())
	}

	// 改密后旧 access_token 应立即失效
	w2 := env.do(t, http.MethodGet, "/api/v1/auth/me", "", bearerHeader(access))
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("改密后旧 access_token 应 401, 实际 %d body=%s", w2.Code, w2.Body.String())
	}
}

func TestRefreshRotatesAndRejectsOldToken(t *testing.T) {
	env := newTestServer(t)
	hash, _ := pwd.Hash("secret123")
	env.seedUser(t, &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1})

	_, refresh1 := loginAndExtract(t, env, "T001", "secret123")

	// 第一次刷新成功并轮换 refresh_token
	w := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", cookieHeader("refresh_token", refresh1))
	if w.Code != http.StatusOK {
		t.Fatalf("首次刷新应 200, 实际 %d body=%s", w.Code, w.Body.String())
	}
	var refresh2 string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "refresh_token" {
			refresh2 = ck.Value
		}
	}
	if refresh2 == "" || refresh2 == refresh1 {
		t.Fatalf("轮换后应下发新 refresh_token, 实际同旧值")
	}

	// 旧 refresh_token 再次使用 → 判定为重用/重放 → 拒绝
	w2 := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", cookieHeader("refresh_token", refresh1))
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("旧 refresh_token 重用应 401, 实际 %d body=%s", w2.Code, w2.Body.String())
	}

	// 重放触发撤销：连新 token 也一并失效
	w3 := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", cookieHeader("refresh_token", refresh2))
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("检测到重放后新 refresh_token 应随之失效, 实际 %d body=%s", w3.Code, w3.Body.String())
	}
}

func TestRefreshWithUnknownJTIRejected(t *testing.T) {
	env := newTestServer(t)
	hash, _ := pwd.Hash("secret123")
	env.seedUser(t, &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1})

	_, refresh := loginAndExtract(t, env, "T001", "secret123")
	// 篡改 refresh token（破坏签名）→ 401
	w := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", cookieHeader("refresh_token", strings.TrimSuffix(refresh, "a")+"b"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("非法 refresh_token 应 401, 实际 %d", w.Code)
	}
}

// 并发刷新同一旧 refresh_token：条件 UPDATE 保证同一 jti 只能成功续期一次，
// 其余请求 401（重放检测在并发窗口下不失效）。
func TestRefreshConcurrentSameTokenOnlySucceedsOnce(t *testing.T) {
	env := newTestServer(t)
	hash, _ := pwd.Hash("secret123")
	env.seedUser(t, &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1})

	_, refresh := loginAndExtract(t, env, "T001", "secret123")

	ok := 0
	for i := 0; i < 4; i++ {
		w := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", cookieHeader("refresh_token", refresh))
		if w.Code == http.StatusOK {
			ok++
		} else if w.Code != http.StatusUnauthorized {
			t.Fatalf("并发刷新应返回 200 或 401, 实际 %d body=%s", w.Code, w.Body.String())
		}
	}
	if ok == 0 {
		t.Fatalf("同一 jti 至少应有一次成功续期")
	}
	if ok > 1 {
		t.Fatalf("同一 jti 并发刷新仅应成功一次, 实际成功 %d 次", ok)
	}
}
