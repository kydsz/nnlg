package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 这些用例在无 Redis 环境下走本地兜底计数（Redis 可用时同一套断言同样成立）。
func TestLoginLimiterAllow(t *testing.T) {
	l := newLoginLimiter("test-ip", time.Minute, 3)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if !l.allow(ctx, "ip") {
			t.Fatalf("第 %d 次应允许", i+1)
		}
	}
	if l.allow(ctx, "ip") {
		t.Fatal("超出上限应被限流")
	}
}

func TestLoginLimiterUserFailures(t *testing.T) {
	l := newLoginLimiter("test-user", time.Minute, 3)
	ctx := context.Background()
	if l.blocked(ctx, "u") {
		t.Fatal("初始不应被拦截")
	}
	for i := 0; i < 3; i++ {
		l.recordFailure(ctx, "u")
	}
	if !l.blocked(ctx, "u") {
		t.Fatal("达到失败上限应被拦截")
	}
	l.reset(ctx, "u")
	if l.blocked(ctx, "u") {
		t.Fatal("重置后应放行")
	}
}

// redisKey 不得暴露工号/IP 明文，且不同限流维度互不覆盖。
func TestLoginLimiterRedisKeyHashed(t *testing.T) {
	userLimiter := newLoginLimiter("user", time.Minute, 3)
	ipLimiter := newLoginLimiter("ip", time.Minute, 3)
	key := userLimiter.redisKey("2021001")
	if strings.Contains(key, "2021001") {
		t.Fatalf("Redis 键不应包含工号明文: %s", key)
	}
	if !strings.HasPrefix(key, "login:user:") {
		t.Fatalf("Redis 键前缀不符: %s", key)
	}
	if key != userLimiter.redisKey("2021001") {
		t.Fatal("同一账号的 Redis 键应稳定")
	}
	if key == ipLimiter.redisKey("2021001") {
		t.Fatal("不同限流维度的键不应相同")
	}
}

func TestClientIPTrustedProxy(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	req := httptest.NewRequest("POST", "/", nil)
	c.Request = req

	t.Run("可信代理采信X-Real-IP", func(t *testing.T) {
		ConfigureTrustedProxies([]string{"10.0.0.0/8"})
		req.RemoteAddr = "10.0.0.2:1234"
		req.Header.Set("X-Real-IP", "198.51.100.9")
		req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.2")
		if got := clientIP(c); got != "198.51.100.9" {
			t.Fatalf("应优先取 X-Real-IP，得到 %q", got)
		}
	})

	t.Run("可信代理取XFF末段", func(t *testing.T) {
		ConfigureTrustedProxies([]string{"10.0.0.0/8"})
		req.RemoteAddr = "10.0.0.2:1234"
		req.Header.Del("X-Real-IP")
		if got := clientIP(c); got != "10.0.0.2" {
			t.Fatalf("应取 X-Forwarded-For 末段，得到 %q", got)
		}
	})

	t.Run("不可信来源忽略伪造头部", func(t *testing.T) {
		ConfigureTrustedProxies([]string{"10.0.0.0/8"})
		req.RemoteAddr = "198.51.100.5:443"
		req.Header.Set("X-Real-IP", "1.2.3.4")
		req.Header.Set("X-Forwarded-For", "5.6.7.8")
		if got := clientIP(c); got != "198.51.100.5" {
			t.Fatalf("直连客户端不得用头部换 IP，应取 RemoteAddr，得到 %q", got)
		}
	})

	t.Run("未配置可信代理时只用RemoteAddr", func(t *testing.T) {
		ConfigureTrustedProxies(nil)
		req.RemoteAddr = "127.0.0.1:5555"
		req.Header.Set("X-Real-IP", "1.2.3.4")
		if got := clientIP(c); got != "127.0.0.1" {
			t.Fatalf("未配置可信代理时应忽略头部，得到 %q", got)
		}
	})

	t.Run("非法头部值被忽略", func(t *testing.T) {
		ConfigureTrustedProxies([]string{"10.0.0.0/8"})
		req.RemoteAddr = "10.0.0.2:1234"
		req.Header.Set("X-Real-IP", "not-an-ip")
		req.Header.Set("X-Forwarded-For", "also-bad")
		if got := clientIP(c); got != "10.0.0.2" {
			t.Fatalf("非法头部应回退对端地址，得到 %q", got)
		}
	})

	// 恢复默认（避免影响其它用例）
	t.Cleanup(func() { ConfigureTrustedProxies(nil) })
}

func TestConfigureTrustedProxies(t *testing.T) {
	ConfigureTrustedProxies([]string{"127.0.0.1", "::1", "172.16.0.0/12", " ", "bad-cidr"})
	defer ConfigureTrustedProxies(nil)

	cases := map[string]bool{
		"127.0.0.1":   true,
		"::1":         true,
		"172.16.5.9":  true,
		"172.31.0.1":  true,
		"172.32.0.1":  false,
		"192.168.1.1": false,
		"8.8.8.8":     false,
		"not-an-ip":   false,
	}
	for ip, want := range cases {
		if got := isTrustedProxy(ip); got != want {
			t.Fatalf("isTrustedProxy(%s) = %v, 期望 %v", ip, got, want)
		}
	}
}
