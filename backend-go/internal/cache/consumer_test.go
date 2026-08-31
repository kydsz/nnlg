package cache

import (
	"context"
	"testing"
	"time"

	"backend-go/internal/config"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 验证 RunConsumer 消费者 goroutine 能正确消费并回调 persist。
func TestConsumerLoop(t *testing.T) {
	cfg := &config.Config{RedisAddr: "localhost:6380"}
	c := New(cfg)
	if !c.Enabled {
		t.Skip("Redis 未连接，跳过")
	}
	c.StreamName = "test:consumer:queue"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumed := make(chan EvalSubmitMsg, 5)
	var nilDB *gorm.DB
	c.RunConsumer(ctx, nilDB, func(ctx context.Context, db *gorm.DB, msgs []EvalSubmitMsg) []string {
		var ack []string
		for _, m := range msgs {
			consumed <- m
			ack = append(ack, m.MsgID)
		}
		return ack
	})

	// 等待消费者建立 group
	time.Sleep(500 * time.Millisecond)

	// 生产 2 条
	for i := 0; i < 2; i++ {
		if _, err := c.Publish(ctx, c.StreamName, map[string]interface{}{
			"task_id": "1", "user_id": "1", "is_anonymous": "false",
			"dimension_values": `{"x":"1"}`,
		}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// 等待消费
	got := []EvalSubmitMsg{}
	for i := 0; i < 2; i++ {
		select {
		case m := <-consumed:
			got = append(got, m)
		case <-time.After(3 * time.Second):
			t.Fatalf("消费者超时未消费，已收 %d/2", len(got))
		}
	}
	if got[0].TaskID != 1 {
		t.Fatalf("TaskID 解析错误: %+v", got[0])
	}
	if len(got[1].DimensionValues) == 0 {
		t.Fatal("DimensionValues 为空")
	}

	// Pending 应归零（都被 ACK）
	time.Sleep(200 * time.Millisecond)
	cnt, _ := c.PendingCount(ctx, c.StreamName, evalGroup)
	if cnt != 0 {
		t.Fatalf("消息未全部 ACK, pending=%d", cnt)
	}

	c.rdb.Del(ctx, c.StreamName)
	t.Log("消费者循环验证通过")
}

// 验证死信机制 + 自动重放一次：
//   - 首次清扫把"卡死"消息挪入死信流并自动重放回主队列（方案 B）。
//   - 重放副本再次失败被清扫时，因带 _replay_count 不再重放，滞留死信流待人工。
// 用真实 Redis（localhost:6380），无 Redis 自动跳过。
func TestDeadLetterSweep(t *testing.T) {
	cfg := &config.Config{RedisAddr: "localhost:6380"}
	c := New(cfg)
	if !c.Enabled {
		t.Skip("Redis 未连接，跳过")
	}
	ctx := context.Background()
	stream := "test:dlq:queue"
	group := "dlq-group"
	// 清理上次失败运行可能遗留的状态，保证幂等
	c.rdb.Del(ctx, stream, c.DeadStream(stream))
	if err := c.EnsureGroup(ctx, stream, group); err != nil {
		t.Fatalf("EnsureGroup: %v", err)
	}

	// 生产一条"卡死"消息并消费（进入 Pending 但故意不 ACK）
	if _, err := c.Publish(ctx, stream, map[string]interface{}{
		"task_id": "1", "user_id": "1", "dimension_values": `{"x":"1"}`,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, err := c.Consume(ctx, stream, group, "c1", 10, 2*time.Second); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	// 手动把投递次数抬到阈值以上
	for i := 0; i < maxDelivery; i++ {
		c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: stream, Group: group, Consumer: "c1", MinIdle: 0, Start: "-", Count: 100,
		}).Result()
	}

	// 首次清扫：应挪入死信流 + 自动重放回主队列
	c.sweepOnce(ctx, stream, group, "c1")
	dead := c.DeadStream(stream)
	deadLen, _ := c.rdb.XLen(ctx, dead).Result()
	if deadLen != 1 {
		t.Fatalf("首次清扫死信流应为 1 条, 实际 %d", deadLen)
	}
	// 死信流条目应带 _replay_count=1
	dmsgs, _ := c.rdb.XRange(ctx, dead, "-", "+").Result()
	if len(dmsgs) != 1 || dmsgs[0].Values["_replay_count"] != "1" {
		t.Fatalf("死信流首条应带 _replay_count=1: %+v", dmsgs)
	}
	// 主队列应有自动重放的副本（消费它，验证携带 _replay_count）
	replayedMsgs, err := c.Consume(ctx, stream, group, "c1", 10, 2*time.Second)
	if err != nil {
		t.Fatalf("消费自动重放副本: %v", err)
	}
	if len(replayedMsgs) == 0 {
		t.Fatal("首次清扫后主队列应有自动重放副本")
	}
	if replayedMsgs[0].Values["_replay_count"] != "1" {
		t.Fatalf("自动重放副本应携带 _replay_count=1: %+v", replayedMsgs[0].Values)
	}

	// 把自动重放的副本抬到投递阈值以上
	for i := 0; i < maxDelivery; i++ {
		c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: stream, Group: group, Consumer: "c1", MinIdle: 0, Start: "-", Count: 100,
		}).Result()
	}
	// 二次清扫：副本带 _replay_count，只入死信不再重放，主队列 Pending 应清空
	c.sweepOnce(ctx, stream, group, "c1")
	deadLen2, _ := c.rdb.XLen(ctx, dead).Result()
	if deadLen2 != 2 {
		t.Fatalf("二次清扫死信流应为 2 条（首次+最终）, 实际 %d", deadLen2)
	}
	cnt, _ := c.PendingCount(ctx, stream, group)
	if cnt != 0 {
		t.Fatalf("二次清扫后主队列 Pending 应归零（不再重放）, 实际 %d", cnt)
	}

	// 死信重放（人工）回主队列
	replayed, err := c.ReplayDead(ctx, dead, stream, 100)
	if err != nil {
		t.Fatalf("ReplayDead: %v", err)
	}
	if replayed != 2 {
		t.Fatalf("死信重放应为 2 条, 实际 %d", replayed)
	}

	// 清理
	c.rdb.Del(ctx, stream, dead)
	t.Log("死信机制 + 自动重放一次 验证通过")
}

// 单元测试 isDeadLetter 判定逻辑（不依赖 Redis）：
// 覆盖投递次数超阈值、停留时长超阈值两个死信分支。
func TestIsDeadLetter(t *testing.T) {
	// 投递次数超阈值 → 死信
	if !isDeadLetter(redis.XPendingExt{ID: "a", RetryCount: maxDelivery, Idle: time.Second}) {
		t.Fatal("RetryCount 达阈值应判死信")
	}
	// 停留时长超阈值 → 死信
	if !isDeadLetter(redis.XPendingExt{ID: "b", RetryCount: 1, Idle: maxIdleTime}) {
		t.Fatal("Idle 达阈值应判死信")
	}
	// 两者都未超 → 非死信
	if isDeadLetter(redis.XPendingExt{ID: "c", RetryCount: 1, Idle: time.Second}) {
		t.Fatal("正常消息不应判死信")
	}
	t.Log("isDeadLetter 判定验证通过")
}
