package jwxt

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestEncodeBase64Custom(t *testing.T) {
	// ASCII 输入：与前端/标准 Base64 完全一致
	for _, in := range []string{"", "a", "ab", "abc", "2023001", "Abc@123456", strings.Repeat("x9", 37)} {
		if got, want := encodeBase64Custom(in), base64.StdEncoding.EncodeToString([]byte(in)); got != want {
			t.Fatalf("encodeBase64Custom(%q) = %q, 期望 %q", in, got, want)
		}
	}
	// 非 ASCII：按 UTF-8 字节编码，不得 panic（旧实现按 rune 索引字母表会越界）
	got := encodeBase64Custom("密码abc")
	want := base64.StdEncoding.EncodeToString([]byte("密码abc"))
	if got != want {
		t.Fatalf("非 ASCII 编码 = %q, 期望 %q", got, want)
	}
}

func TestLoginVerdict(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		want  int
		extra string
	}{
		{"JSON 成功", `{"success":true}`, loginOK, ""},
		{"JSON 成功(字符串)", ` {"success":"1"} `, loginOK, ""},
		{"JSON 失败带消息", `{"success":false,"msg":"用户名或密码错误"}`, loginFail, "用户名或密码错误"},
		{"JSON 失败无消息", `{"success":false}`, loginFail, "教务系统返回登录失败"},
		{"带失败标记的脚本", `<script>alert('密码错误');history.back();</script>`, loginFail, "密码错误"},
		{"带成功标记", `<html>欢迎使用<a href="framework/jsMain.jsp">主页</a></html>`, loginOK, ""},
		// 回归：JS 里出现 error 字样不再被误判为登录失败
		{"含 error 字样的页面", `<html><script>var error = 1;</script></html>`, loginUnknown, ""},
		{"空响应", ``, loginUnknown, ""},
	}
	for _, c := range cases {
		got, detail := loginVerdict([]byte(c.body))
		if got != c.want {
			t.Fatalf("%s: verdict = %d, 期望 %d", c.name, got, c.want)
		}
		if c.extra != "" && !strings.Contains(detail, c.extra) {
			t.Fatalf("%s: detail = %q, 期望包含 %q", c.name, detail, c.extra)
		}
	}
}

func TestCharsetFromContentType(t *testing.T) {
	cases := map[string]string{
		"text/html; charset=GBK":            "gbk",
		"text/html;charset=\"gb2312\"":      "gb2312",
		"text/html; charset=UTF-8; foo=bar": "utf-8",
		"application/json":                  "",
		"":                                  "",
	}
	for in, want := range cases {
		if got := charsetFromContentType(in); got != want {
			t.Fatalf("charsetFromContentType(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestToUTF8UsesDeclaredCharset(t *testing.T) {
	gbkBytes := []byte{0xD6, 0xD0, 0xCE, 0xC4} // GBK: 中文
	if got := toUTF8(gbkBytes, "text/html; charset=gbk"); got != "中文" {
		t.Fatalf("按声明 GBK 解码 = %q, 期望 %q", got, "中文")
	}
	// 未声明编码时回退探测（GBK 字节不是合法 UTF-8）
	if got := toUTF8(gbkBytes, "text/html"); got != "中文" {
		t.Fatalf("无声明回退解码 = %q, 期望 %q", got, "中文")
	}
	// UTF-8 + BOM 去 BOM
	bom := append([]byte{0xEF, 0xBB, 0xBF}, []byte("中文")...)
	if got := toUTF8(bom, "text/html; charset=utf-8"); got != "中文" {
		t.Fatalf("BOM 处理 = %q, 期望 %q", got, "中文")
	}
	if got := toUTF8(nil, ""); got != "" {
		t.Fatalf("空内容应返回空串, 实际 %q", got)
	}
}

// newTestAuth 构造指向测试服务器的认证器
func newTestAuth(t *testing.T, server *httptest.Server) *Auth {
	t.Helper()
	a := NewAuth(server.URL, server.URL)
	if a.LoginURL != server.URL+"/xk/LoginToXk" {
		t.Fatalf("LoginURL 不符: %s", a.LoginURL)
	}
	return a
}

// TestAuthConcurrentLogin 并发 Login/Relogin 不得产生数据竞争或登录态丢失
func TestAuthConcurrentLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "LoginToXk"):
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.Contains(r.URL.Path, "loginupdtoGld.do"):
			_, _ = w.Write([]byte(`{"val":"1","upd":"u","time":"t"}`))
		default:
			_, _ = w.Write([]byte(`<html>ok</html>`))
		}
	}))
	defer server.Close()

	a := newTestAuth(t, server)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				errs[idx] = a.Login("2023001", "Abc@123456")
				return
			}
			if err := a.Login("2023001", "Abc@123456"); err != nil {
				errs[idx] = err
				return
			}
			errs[idx] = a.Relogin()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 个并发登录失败: %v", i, err)
		}
	}
	if !a.IsLoggedIn() {
		t.Fatal("并发登录后登录态应为 true")
	}
}

// TestAuthReloginWithoutCredentials 未登录过的实例 Relogin 应报错而非 panic
func TestAuthReloginWithoutCredentials(t *testing.T) {
	a := NewAuth("http://127.0.0.1:1", "http://127.0.0.1:1")
	if err := a.Relogin(); err == nil || !strings.Contains(err.Error(), "凭据") {
		t.Fatalf("无凭据 Relogin 应报错: %v", err)
	}
}
