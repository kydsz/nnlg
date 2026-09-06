package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginLimiterAllow(t *testing.T) {
	l := newLoginLimiter(time.Minute, 3)
	for i := 0; i < 3; i++ {
		if !l.allow("ip") {
			t.Fatalf("第 %d 次应允许", i+1)
		}
	}
	if l.allow("ip") {
		t.Fatal("超出上限应被限流")
	}
}

func TestLoginLimiterUserFailures(t *testing.T) {
	l := newLoginLimiter(time.Minute, 3)
	if l.blocked("u") {
		t.Fatal("初始不应被拦截")
	}
	for i := 0; i < 3; i++ {
		l.recordFailure("u")
	}
	if !l.blocked("u") {
		t.Fatal("达到失败上限应被拦截")
	}
	l.reset("u")
	if l.blocked("u") {
		t.Fatal("重置后应放行")
	}
}

func TestClientIP(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	req := httptest.NewRequest("POST", "/", nil)
	c.Request = req

	// 优先 X-Real-IP
	req.Header.Set("X-Real-IP", "198.51.100.9")
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.2")
	if got := clientIP(c); got != "198.51.100.9" {
		t.Fatalf("应优先取 X-Real-IP，得到 %q", got)
	}

	// 无 X-Real-IP 时取 XFF 末段（nginx 追加的真实 IP）
	req.Header.Del("X-Real-IP")
	if got := clientIP(c); got != "10.0.0.2" {
		t.Fatalf("应取 X-Forwarded-For 末段，得到 %q", got)
	}

	// 无代理头时回退 RemoteAddr
	req.Header.Del("X-Forwarded-For")
	req.RemoteAddr = "192.0.2.5:1234"
	if got := clientIP(c); got != "192.0.2.5" {
		t.Fatalf("应回退 RemoteAddr，得到 %q", got)
	}
}
