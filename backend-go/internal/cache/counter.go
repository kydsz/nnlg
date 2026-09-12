package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// 固定窗口计数器：供登录限流等「多实例共享额度」的场景使用。
// 传入的 key 会自动加统一前缀（tev:），调用方无需关心命名空间。
// 所有函数在 Redis 未启用/不可用时返回 ok=false，调用方必须回退本地计数，
// 保证 Redis 故障时安全能力不被整体绕过（只是退化为单实例口径）。

// IncrWindow 递增计数并续期（窗口内累计次数）。ok=false 表示 Redis 不可用。
func IncrWindow(ctx context.Context, key string, window time.Duration) (int64, bool) {
	c := GetClient()
	if c == nil || !c.Enabled || c.rdb == nil {
		return 0, false
	}
	fullKey := c.key(key)
	pipe := c.rdb.Pipeline()
	incr := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, window) // 每次尝试都续期：持续爆破时窗口滚动前移，封锁不会自然放开
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, false
	}
	return incr.Val(), true
}

// PeekCounter 读取当前计数（不递增、不续期）。ok=false 表示 Redis 不可用。
func PeekCounter(ctx context.Context, key string) (int64, bool) {
	c := GetClient()
	if c == nil || !c.Enabled || c.rdb == nil {
		return 0, false
	}
	val, err := c.rdb.Get(ctx, c.key(key)).Int64()
	if err == redis.Nil {
		return 0, true // 键不存在即计数为 0
	}
	if err != nil {
		return 0, false
	}
	return val, true
}

// ResetCounter 清除计数（登录成功时清零失败次数）。Redis 不可用时静默跳过。
func ResetCounter(ctx context.Context, key string) {
	c := GetClient()
	if c == nil || !c.Enabled || c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, c.key(key)).Err()
}
