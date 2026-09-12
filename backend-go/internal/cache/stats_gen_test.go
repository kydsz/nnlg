package cache

import (
	"context"
	"strconv"
	"testing"

	"backend-go/internal/config"
)

// 集成测试：统计缓存代际（权限/学院变更后让统计缓存立即失效）。
// 需本机临时 Redis（端口 6380），未连接则跳过：
//
//	docker run -d --name tev_test_redis -p 6380:6379 redis:7-alpine redis-server
func TestStatsGenerationInvalidation(t *testing.T) {
	c := New(&config.Config{RedisAddr: "localhost:6380"})
	if !c.Enabled {
		t.Skip("Redis 未连接，跳过集成测试")
	}
	ctx := context.Background()
	uid := 424242
	cleanup := func() {
		c.rdb.Del(ctx, c.key("stats", "gen", "all"))
		c.rdb.Del(ctx, c.key("stats", "gen", "u", strconv.Itoa(uid)))
		c.rdb.Del(ctx, c.key("stats", "gen", "u", strconv.Itoa(uid+1)))
	}
	cleanup()
	defer cleanup()

	g0, u0 := StatsGenerations(ctx, uid)
	if g0 != 0 || u0 != 0 {
		t.Fatalf("初始代际应为 0: global=%d user=%d", g0, u0)
	}

	// 用户维度失效：仅该用户代际 +1，全局代际不变
	InvalidateStats(ctx, uid)
	g1, u1 := StatsGenerations(ctx, uid)
	if u1 != u0+1 {
		t.Fatalf("用户代际应 +1: %d -> %d", u0, u1)
	}
	if g1 != g0 {
		t.Fatalf("用户维度失效不应改变全局代际: %d -> %d", g0, g1)
	}
	if _, other := StatsGenerations(ctx, uid+1); other != 0 {
		t.Fatalf("未失效用户的代际应保持 0，实际 %d", other)
	}

	// 全局失效：所有用户可见的全局代际 +1，用户代际不变
	InvalidateAllStats(ctx)
	g2, u2 := StatsGenerations(ctx, uid)
	if g2 != g1+1 {
		t.Fatalf("全局代际应 +1: %d -> %d", g1, g2)
	}
	if u2 != u1 {
		t.Fatalf("全局失效不应改变用户代际: %d -> %d", u1, u2)
	}
	if g, _ := StatsGenerations(ctx, uid+1); g != g2 {
		t.Fatalf("全局代际应对所有用户生效: %d != %d", g, g2)
	}
}

// Redis 未启用（或未初始化）时，代际读取与失效调用必须安全降级为 no-op。
func TestStatsGenerationsDegraded(t *testing.T) {
	saved := defaultClient
	defaultClient = nil
	defer func() { defaultClient = saved }()

	ctx := context.Background()
	if g, u := StatsGenerations(ctx, 1); g != 0 || u != 0 {
		t.Fatalf("降级应返回 0：global=%d user=%d", g, u)
	}
	// 以下调用不得 panic（Redis 关闭时统计退化为 TTL 失效）
	InvalidateStats(ctx, 1)
	InvalidateStats(ctx, 0)
	InvalidateAllStats(ctx)

	defaultClient = &Client{}
	if g, u := StatsGenerations(ctx, 1); g != 0 || u != 0 {
		t.Fatalf("禁用客户端应返回 0：global=%d user=%d", g, u)
	}
	InvalidateStats(ctx, 1)
	InvalidateAllStats(ctx)
}
