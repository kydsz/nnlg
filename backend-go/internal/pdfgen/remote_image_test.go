package pdfgen

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestIsForbiddenIP(t *testing.T) {
	forbidden := []string{
		"127.0.0.1", "127.0.0.2", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "fe80::1",
		"::ffff:127.0.0.1", "fc00::1",
	}
	for _, s := range forbidden {
		if ip := net.ParseIP(s); ip != nil && !isForbiddenIP(ip) {
			t.Errorf("%s 应为禁用地址", s)
		}
	}
	public := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"}
	for _, s := range public {
		if ip := net.ParseIP(s); ip != nil && isForbiddenIP(ip) {
			t.Errorf("%s 应为允许地址", s)
		}
	}
}

func TestValidateImageURL(t *testing.T) {
	if err := validateImageURL(mustURL(t, "http://127.0.0.1/a.png")); err == nil {
		t.Error("回环地址应被拒绝")
	}
	if err := validateImageURL(mustURL(t, "http://169.254.169.254/latest")); err == nil {
		t.Error("云元数据中心应被拒绝")
	}
	if err := validateImageURL(mustURL(t, "http://192.168.1.1/x")); err == nil {
		t.Error("私网地址应被拒绝")
	}
	if err := validateImageURL(mustURL(t, "https://example.com/a.png")); err != nil {
		t.Errorf("公网域名应放行: %v", err)
	}
	if err := validateImageURL(mustURL(t, "ftp://example.com/a")); err == nil {
		t.Error("非 http(s) 协议应被拒绝")
	}
	if err := validateImageURL(mustURL(t, "javascript:alert(1)")); err == nil {
		t.Error("javascript 协议应被拒绝")
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

func TestLoadImageHolderPathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadImageHolder("/api/v1/files/ok.txt", dir); err != nil {
		t.Fatalf("正常相对路径应可加载: %v", err)
	}
	attack := []string{
		"/files/../../etc/passwd",
		"/api/v1/files/../../../etc/passwd",
		"../../etc/passwd",
		"/files/../",
		"/files/..",
	}
	for _, u := range attack {
		if _, err := loadImageHolder(u, dir); err == nil {
			t.Errorf("目录穿越未被拦截: %s", u)
		}
	}
}
