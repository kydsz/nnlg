package handler

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"time"

	"backend-go/internal/cache"

	"github.com/gin-gonic/gin"
)

const (
	loginIPWindow   = time.Minute
	loginIPLimit    = 20
	loginUserWindow = 10 * time.Minute
	loginUserLimit  = 5
)

// loginIPLimiter 按客户端 IP 记录所有登录尝试（成功+失败），用于防接口刷量。
var loginIPLimiter = newLoginLimiter("ip", loginIPWindow, loginIPLimit)

// loginUserLimiter 按账号记录失败次数，登录成功即清零，用于防口令爆破。
var loginUserLimiter = newLoginLimiter("user", loginUserWindow, loginUserLimit)

// trustedProxyNets 可信反向代理网段：只有直接对端落在这些网段时，
// 才采信 X-Real-IP / X-Forwarded-For。否则客户端可以自行伪造这两个头，
// 每次请求换一个 IP，IP 限流即被完全绕过。
var (
	trustedProxyMu   sync.RWMutex
	trustedProxyNets []*net.IPNet
)

// ConfigureTrustedProxies 配置可信代理网段（启动时由 router 调用）。
// 支持 IP（127.0.0.1）与 CIDR（10.0.0.0/8）；空列表表示「不信任任何代理」。
func ConfigureTrustedProxies(entries []string) {
	nets := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, ipnet, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, ipnet)
		}
	}
	trustedProxyMu.Lock()
	trustedProxyNets = nets
	trustedProxyMu.Unlock()
}

// isTrustedProxy 判断对端 IP 是否为可信代理
func isTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	trustedProxyMu.RLock()
	defer trustedProxyMu.RUnlock()
	for _, n := range trustedProxyNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// loginLimiter 登录限流计数器：
//   - 多实例共享：Redis 可用时计数落在 Redis（固定窗口），所有实例共用一份额度；
//   - 本地降级：Redis 未启用/不可用时回退进程内存计数，安全能力不被整体绕过（口径退化为单实例）。
type loginLimiter struct {
	name        string
	window      time.Duration
	limit       int
	mu          sync.Mutex
	items       map[string]*loginBucket
	lastCleanup time.Time
}

type loginBucket struct {
	start time.Time
	count int
}

func newLoginLimiter(name string, window time.Duration, limit int) *loginLimiter {
	return &loginLimiter{name: name, window: window, limit: limit, items: map[string]*loginBucket{}}
}

// redisKey 计数键：账号名/IP 先哈希，避免把工号、IP 明文写进 Redis；
// 前缀含 name，使 IP 与账号两类额度互不覆盖。
func (l *loginLimiter) redisKey(key string) string {
	sum := md5.Sum([]byte(key))
	return "login:" + l.name + ":" + hex.EncodeToString(sum[:])
}

// allow 消耗一个额度（IP 限流），窗口内达到上限返回 false。
func (l *loginLimiter) allow(ctx context.Context, key string) bool {
	if n, ok := cache.IncrWindow(ctx, l.redisKey(key), l.window); ok {
		return n <= int64(l.limit)
	}
	return l.allowLocal(key)
}

// blocked 仅查询当前是否已达上限（账号限流），不消耗额度。
func (l *loginLimiter) blocked(ctx context.Context, key string) bool {
	if n, ok := cache.PeekCounter(ctx, l.redisKey(key)); ok {
		return n >= int64(l.limit)
	}
	return l.blockedLocal(key)
}

// recordFailure 记录一次失败（账号限流），窗口内计数封顶。
func (l *loginLimiter) recordFailure(ctx context.Context, key string) {
	if _, ok := cache.IncrWindow(ctx, l.redisKey(key), l.window); ok {
		return
	}
	l.recordFailureLocal(key)
}

// reset 清除记录（登录成功时调用）。
func (l *loginLimiter) reset(ctx context.Context, key string) {
	cache.ResetCounter(ctx, l.redisKey(key))
	l.resetLocal(key)
}

// ---------- 本地兜底计数（Redis 不可用时的降级路径） ----------

func (l *loginLimiter) allowLocal(key string) bool {
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

func (l *loginLimiter) blockedLocal(key string) bool {
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

func (l *loginLimiter) recordFailureLocal(key string) {
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

func (l *loginLimiter) resetLocal(key string) {
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
//
// 安全前提：只有当直接对端（RemoteAddr）位于可信代理网段时，才采信
// X-Real-IP / X-Forwarded-For；否则一律使用 RemoteAddr。
// 若无条件采信这两个头，客户端只要自己设置头部即可每次换 IP，绕过限流。
//
// 采信头部时的取值顺序（部署形态为 nginx 反代）：
//   - X-Real-IP 由 nginx 用 $remote_addr 覆盖，客户端无法伪造，优先级最高
//   - X-Forwarded-For 由 nginx 追加真实 IP（$proxy_add_x_forwarded_for），取末段
//   - 直连后端时回退 RemoteAddr
func clientIP(c *gin.Context) string {
	remote := remoteHost(c)
	if !isTrustedProxy(remote) {
		return remote
	}
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
	return remote
}

// remoteHost 取 TCP 对端地址（去掉端口）
func remoteHost(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return host
}
