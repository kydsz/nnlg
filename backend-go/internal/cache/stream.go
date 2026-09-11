package cache

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============ Stream 消费者组 ============

// streamMaxLen 主队列近似最大长度：已 ACK 消息若不清理会永久留在 Stream，
// Redis 内存随提交量线性增长直至 OOM。采用 XTRIM MAXLEN~ 近似裁剪（性能优先），
// 保留最近 N 条已确认消息（消息量远超阈值时按近似长度裁掉头部）。
const streamMaxLen = 100000

// deadStreamMaxLen 死信流近似最大长度：死信人工重放后消息已被 XDEL 清理，
// 此值兜底"长期未处理死信"的积压上限，避免死信流无限增长。
const deadStreamMaxLen = 50000

// Trim 对指定流做近似裁剪（XTRIM MAXLEN~），控制内存占用。流不存在时静默。
func (c *Client) Trim(ctx context.Context, stream string, maxLen int64) {
	if !c.Enabled {
		return
	}
	_ = c.rdb.XTrimMaxLenApprox(ctx, stream, maxLen, 0).Err()
}

// EnsureGroup 确保消费者组存在（幂等，首次调用自动创建）。
func (c *Client) EnsureGroup(ctx context.Context, stream, group string) error {
	if !c.Enabled {
		return redis.Nil
	}
	err := c.rdb.XGroupCreateMkStream(ctx, stream, group, "$").Err()
	if err != nil && err != redis.Nil {
		// BUSYGROUP 等已存在错误忽略
		if isBusyGroup(err) {
			return nil
		}
		return err
	}
	return nil
}

func isBusyGroup(err error) bool {
	return err != nil && (err.Error() == "BUSYGROUP Consumer Group name already exists" ||
		err.Error() == "BUSYGROUP")
}

// isNoGroup 判断是否为 NOGROUP 错误（消费者组或 stream 不存在）
func isNoGroup(err error) bool {
	return err != nil && strings.Contains(err.Error(), "NOGROUP")
}

// Consume 读取一批属于当前消费者的待处理消息（block 阻塞等待）。
// 返回消息 ID->字段映射；无消息时返回 nil（timeout 内无数据）。
func (c *Client) Consume(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]XMessage, error) {
	if !c.Enabled {
		return nil, redis.Nil
	}
	res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()
	if err == redis.Nil {
		return nil, nil // 阻塞超时无数据
	}
	if err != nil {
		return nil, err
	}
	var out []XMessage
	for _, st := range res {
		for _, msg := range st.Messages {
			out = append(out, XMessage{ID: msg.ID, Values: msg.Values})
		}
	}
	return out, nil
}

// Ack 确认消息已处理（移出 Pending List）。
func (c *Client) Ack(ctx context.Context, stream, group string, ids ...string) error {
	if !c.Enabled || len(ids) == 0 {
		return nil
	}
	return c.rdb.XAck(ctx, stream, group, ids...).Err()
}

// XMessage Stream 消息载体
type XMessage struct {
	ID     string
	Values map[string]interface{}
}

// PendingCount 返回待处理（未 ACK）消息数量，用于监控/补偿判断。
func (c *Client) PendingCount(ctx context.Context, stream, group string) (int64, error) {
	if !c.Enabled {
		return 0, redis.Nil
	}
	p, err := c.rdb.XPending(ctx, stream, group).Result()
	if err != nil {
		return 0, err
	}
	return p.Count, nil
}

// ============ 死信与补偿 ============

// DeadStream 返回某条主队列对应的死信流名（<stream>:dead）。
// 死信消息集中在此，便于人工/脚本统一重放，同样受 AOF 持久化保护。
func (c *Client) DeadStream(stream string) string {
	return stream + ":dead"
}

// StreamLen 返回某条流的消息条数（含死信流）。流不存在返回 0。
func (c *Client) StreamLen(ctx context.Context, stream string) (int64, error) {
	if !c.Enabled {
		return 0, redis.Nil
	}
	n, err := c.rdb.XLen(ctx, stream).Result()
	if err == redis.Nil {
		return 0, nil
	}
	return n, err
}

// PendingSummary 返回消费者组内所有待处理消息的摘要（ID、所属消费者、投递次数）。
// 用于补偿清扫：找出投递次数超过阈值的"卡死"消息。
func (c *Client) PendingSummary(ctx context.Context, stream, group string, count int64) ([]redis.XPendingExt, error) {
	if !c.Enabled {
		return nil, redis.Nil
	}
	return c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream, Group: group, Start: "-", End: "+", Count: count,
	}).Result()
}

// MoveToDead 将一批卡死消息认领到当前消费者，并挪入死信流后 ACK。
// 死信策略（方案 B：自动重放一次）：
//   - 首次进入死信（源消息无 _replay_count 字段）→ 写入死信流并**自动重放一次**回主队列，
//     给提交第二次机会；重放副本携带 _replay_count=1。
//   - 再次进入死信（源消息已带 _replay_count）→ 已经自动重放过仍失败，写入死信流后**不再自动重放**，
//     滞留待人工通过 ReplayDead 处理。
//
// 无论哪种情况都不丢数据：原消息先写死信流、再 ACK；AOF 落盘保护。
func (c *Client) MoveToDead(ctx context.Context, stream, group, consumer string, ids []string, retries map[string]int64) error {
	if !c.Enabled || len(ids) == 0 {
		return nil
	}
	deadStream := c.DeadStream(stream)
	// 逐条认领（XAUTOCLAIM 支持批量，这里按批处理）。
	// 认领失败的消息跳过，等待下轮清扫；成功挪入死信流 + ACK。
	var moved []string
	for _, id := range ids {
		// MinIdle=sweepInterval/2：只认领"空闲超过半个清扫周期"的消息，避免
		// 抢走正在被主消费者处理、尚未超阈值的新消息（原 MinIdle=0 会误认领）。
		claimed, _, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: stream, Group: group, Consumer: consumer,
			MinIdle: sweepInterval / 2, Start: id, Count: 1,
		}).Result()
		if err != nil {
			log.Printf("[consumer] 认领死信消息失败 id=%s: %v", id, err)
			continue
		}
		if len(claimed) == 0 {
			continue // 该消息已不在 Pending（可能已被处理/ACK）
		}
		vals := claimed[0].Values
		if vals == nil {
			vals = map[string]interface{}{}
		}
		// 判断是否已自动重放过：源消息带 _replay_count 说明之前已自动重放一次仍失败
		alreadyReplayed := false
		if _, ok := vals["_replay_count"]; ok {
			alreadyReplayed = true
		}
		now := time.Now().Format(time.RFC3339)
		vals["_dead_at"] = now
		vals["_retry_count"] = retries[id]
		if !alreadyReplayed {
			vals["_replay_count"] = "1"
		}
		// 1) 先写入死信流（AOF 落盘，作为审计与人工重放来源）
		if err := c.rdb.XAdd(ctx, &redis.XAddArgs{Stream: deadStream, Values: vals}).Err(); err != nil {
			log.Printf("[consumer] 写入死信流失败 id=%s: %v", id, err)
			continue
		}
		// 2) 首次进入死信 → 自动重放一次回主队列（副本剥离死信元信息，保留 _replay_count 标记）
		if !alreadyReplayed {
			replayVals := make(map[string]interface{}, len(vals))
			for k, v := range vals {
				if k == "_dead_at" || k == "_retry_count" {
					continue
				}
				replayVals[k] = v
			}
			if err := c.rdb.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: replayVals}).Err(); err != nil {
				// 自动重放失败：死信流已保留，靠人工 ReplayDead 兜底，不丢数据
				log.Printf("[consumer] 死信自动重放失败 id=%s: %v（保留在死信流待人工重放）", id, err)
			}
		}
		// 3) ACK 原消息（已成功挪入死信，从主队列移除）
		if err := c.rdb.XAck(ctx, stream, group, id).Err(); err != nil {
			log.Printf("[consumer] 死信 ACK 失败 id=%s: %v", id, err)
		}
		moved = append(moved, id)
	}
	if len(moved) > 0 {
		log.Printf("[consumer] %d 条消息判定为死信，已挪入 %s（首次自动重放，二次滞留待人工）: %v", len(moved), deadStream, moved)
		// 死信流近似裁剪，防止长期未处理死信积压撑爆内存（AC4）
		c.Trim(ctx, deadStream, deadStreamMaxLen)
	}
	return nil
}

// ReplayDead 将死信流中的消息重新投递回主队列（人工补偿）。
// 重放成功即从死信流删除该消息（XDEL），避免重复点击"重放死信"反复提交；
// 崩溃窗口由 DB 唯一约束兜底（重放副本若已落库，消费者按幂等成功处理）。
// 返回重放的消息数。
func (c *Client) ReplayDead(ctx context.Context, deadStream, targetStream string, count int64) (int64, error) {
	if !c.Enabled {
		return 0, redis.Nil
	}
	res, err := c.rdb.XRangeN(ctx, deadStream, "-", "+", count).Result()
	if err != nil {
		return 0, err
	}
	replayed := int64(0)
	for _, m := range res {
		vals := make(map[string]interface{}, len(m.Values))
		for k, v := range m.Values {
			if k == "_dead_at" || k == "_retry_count" || k == "_replay_count" {
				continue // 去掉所有死信/重放元信息，作为全新消息重试
			}
			vals[k] = v
		}
		if err := c.rdb.XAdd(ctx, &redis.XAddArgs{Stream: targetStream, Values: vals}).Err(); err != nil {
			log.Printf("[consumer] 死信重放失败 id=%s: %v", m.ID, err)
			continue
		}
		// 重放成功即删除死信原消息：保证重复点击重放不会重复入队
		if err := c.rdb.XDel(ctx, deadStream, m.ID).Err(); err != nil {
			log.Printf("[consumer] 死信重放后删除原消息失败 id=%s: %v", m.ID, err)
		}
		replayed++
	}
	return replayed, nil
}
