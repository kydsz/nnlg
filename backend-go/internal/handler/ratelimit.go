package handler

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loginIPWindow   = time.Minute
	loginIPLimit    = 20
	loginUserWindow = 10 * time.Minute
	loginUserLimit  = 5
)

// loginIPLimiter 按客户端 IP 记录所有登录尝试（成功+失败），用于防接口刷量。
var loginIPLimiter = newLoginLimiter(loginIPWindow, loginIPLimit)

// loginUserLimiter 按账号记录失败次数，登录成功即清零，用于防口令爆破。
var loginUserLimiter = newLoginLimiter(loginUserWindow, loginUserLimit)

type loginLimiter struct {
	mu          sync.Mutex
	window      time.Duration
	limit       int
	items       map[string]*loginBucket
	lastCleanup time.Time
}

type loginBucket struct {
	start time.Time
	count int
}

func newLoginLimiter(window time.Duration, limit int) *loginLimiter {
	return &loginLimiter{window: window, limit: limit, items: map[string]*loginBucket{}}
}

// allow 消耗一个额度（IP 限流），窗口内达到上限返回 false。
func (l *loginLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanup(now)
	b, ok := l.items[key]
	if !ok || now.Sub(b.start) >= l.window {
		l.items[key] = &loginBucket{start: now, count: 1}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

// blocked 仅查询当前是否已达上限（账号限流），不消耗额度。
func (l *loginLimiter) blocked(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanup(now)
	b, ok := l.items[key]
	if !ok || now.Sub(b.start) >= l.window {
		return false
	}
	return b.count >= l.limit
}

// recordFailure 记录一次失败（账号限流），窗口内计数封顶。
func (l *loginLimiter) recordFailure(key string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanup(now)
	b, ok := l.items[key]
	if !ok || now.Sub(b.start) >= l.window {
		l.items[key] = &loginBucket{start: now, count: 1}
		return
	}
	if b.count < l.limit {
		b.count++
	}
}

// reset 清除记录（登录成功时调用）。
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.items, key)
}

// cleanup 周期性清理过期条目，防止内存无限增长。
func (l *loginLimiter) cleanup(now time.Time) {
	if now.Sub(l.lastCleanup) < time.Minute {
		return
	}
	l.lastCleanup = now
	for k, b := range l.items {
		if now.Sub(b.start) >= l.window {
			delete(l.items, k)
		}
	}
}

// clientIP 提取真实客户端 IP，用于登录限流。
// 部署形态为 nginx 反代：
//   - X-Real-IP 由 nginx 用 $remote_addr 覆盖，客户端无法伪造，优先级最高
//   - X-Forwarded-For 由 nginx 追加真实 IP（$proxy_add_x_forwarded_for），取末段
//   - 直连后端时回退 RemoteAddr
func clientIP(c *gin.Context) string {
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); net.ParseIP(ip) != nil {
			return ip
		}
	}
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
			return last
		}
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
