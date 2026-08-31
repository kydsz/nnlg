package cache

import (
	"context"
	"testing"
	"time"

	"backend-go/internal/config"
)

// 集成测试：连接本机临时 Redis（端口 6380）验证 Stream 生产/消费/ACK 全链路。
// 运行: go test -run TestRedisStreamIntegration ./internal/cache/
// 需先: docker run -d --name tev_test_redis -p 6380:6379 redis:7-alpine redis-server
func TestRedisStreamIntegration(t *testing.T) {
	cfg := &config.Config{RedisAddr: "localhost:6380"}
	c := New(cfg)
	if !c.Enabled {
		t.Skip("Redis 未连接，跳过集成测试")
	}
	ctx := context.Background()

	const stream = "test:queue"
	const group = "test-group"

	if err := c.EnsureGroup(ctx, stream, group); err != nil {
		t.Fatalf("EnsureGroup: %v", err)
	}

	msgID, err := c.Publish(ctx, stream, map[string]interface{}{
		"task_id": "123", "user_id": "456", "is_anonymous": "false",
		"dimension_values": `{"attitude":"5"}`,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if msgID == "" {
		t.Fatal("msgID 为空")
	}

	msgs, err := c.Consume(ctx, stream, group, "c1", 10, 2*time.Second)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("未消费到消息")
	}
	if msgs[0].Values["task_id"] != "123" {
		t.Fatalf("task_id 不符: %v", msgs[0].Values["task_id"])
	}

	if err := c.Ack(ctx, stream, group, msgs[0].ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	cnt, err := c.PendingCount(ctx, stream, group)
	if err != nil {
		t.Fatalf("PendingCount: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("Pending 应为 0, 实际 %d", cnt)
	}

	k := "test:dup:1:2"
	ok1, err := c.SetNX(ctx, k, time.Minute)
	if err != nil || !ok1 {
		t.Fatalf("SetNX 首次应成功: ok=%v err=%v", ok1, err)
	}
	ok2, _ := c.SetNX(ctx, k, time.Minute)
	if ok2 {
		t.Fatal("SetNX 重复应失败")
	}
	c.Del(ctx, k)

	c.rdb.Del(ctx, stream)
	t.Log("集成测试全部通过")
}
