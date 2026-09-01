package handler_test

import (
	"net/http"
	"testing"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"
)

func TestLoginWritesTwoCookiesAndRefresh(t *testing.T) {
	env := newTestServer(t)
	hash, err := pwd.Hash("secret123")
	if err != nil {
		t.Fatalf("生成密码失败: %v", err)
	}
	u := &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1}
	env.seedUser(t, u)

	w := env.do(t, http.MethodPost, "/api/v1/auth/login", `{"user_no":"T001","password":"secret123"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("登录状态码=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			User        struct {
				ID int `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != 200 || resp.Data.AccessToken == "" || resp.Data.TokenType != "bearer" || resp.Data.User.ID != u.ID {
		t.Fatalf("登录响应不符: %+v", resp)
	}

	cookies := w.Result().Cookies()
	names := map[string]bool{}
	var refresh *http.Cookie
	for _, ck := range cookies {
		names[ck.Name] = true
		if ck.Name == "refresh_token" {
			refresh = ck
		}
	}
	if !names["token"] || !names["refresh_token"] {
		t.Fatalf("应写 token/refresh_token 两个 cookie, 实际 %v", names)
	}
	if refresh == nil || refresh.Value == "" {
		t.Fatal("缺少 refresh_token cookie 值")
	}

	// 用 refresh_token cookie 换新 access_token
	header := http.Header{}
	header.Set("Cookie", "refresh_token="+refresh.Value)
	w2 := env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", header)
	if w2.Code != http.StatusOK {
		t.Fatalf("刷新状态码=%d body=%s", w2.Code, w2.Body.String())
	}
	var resp2 struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
			User        struct {
				ID int `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	decodeJSON(t, w2.Body.Bytes(), &resp2)
	if resp2.Code != 200 || resp2.Data.AccessToken == "" || resp2.Data.User.ID != u.ID {
		t.Fatalf("刷新响应不符: %+v", resp2)
	}
}
