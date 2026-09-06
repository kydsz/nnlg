package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 消费者组名
const evalGroup = "eval-consumers"

// EvalGroupName 返回评价消费者组名（供管理端查询队列状态用）。
func EvalGroupName() string { return evalGroup }

// 死信判定阈值：单条消息投递（被 XREADGROUP 读取）超过该次数仍未被 ACK，
// 视为"卡死"（大概率永远无法成功处理），由清扫循环挪入死信流。
const maxDelivery = 3

// maxIdleTime 消息最长停留时间：单条消息在 Pending List 中停留（未被成功处理）
// 超过该时长，无论投递次数多少，都判定为"过期"挪入死信流并告警。
// 防止"投递次数不多但卡了很久"的消息无限滞留、用户以为已提交实则未落库。
const maxIdleTime = 10 * time.Minute

// sweepInterval 清扫周期：周期性认领超时未 ACK 的消息并检查投递次数/停留时长。
const sweepInterval = 30 * time.Second

// EvalSubmitMsg 评价提交入队消息体
type EvalSubmitMsg struct {
	MsgID           string          `json:"-"` // Redis Stream 消息 ID（用于 ACK）
	TaskID          int             `json:"task_id"`
	UserID          int             `json:"user_id"`
	DimensionValues json.RawMessage `json:"dimension_values"`
	IsAnonymous     bool            `json:"is_anonymous"`
}

// PersistFunc 批量处理回调签名（由应用层注入，避免 cache 包反向依赖 service）。
// 输入一批已解析的消息，返回需要确认（ACK）的消息 ID 列表。
// 未返回 ID 的消息视为处理失败，保留在 Pending List 供后续重试。
type PersistFunc func(ctx context.Context, db *gorm.DB, msgs []EvalSubmitMsg) []string

// RunConsumer 启动一个阻塞消费者 goroutine。ctx 取消时退出。
// persist 为批量处理回调（应用层实现：校验 + 批量落库 + 决定哪些消息确认）。
func (c *Client) RunConsumer(ctx context.Context, db *gorm.DB, persist PersistFunc) {
	if !c.Enabled {
		log.Println("[consumer] Redis 禁用，跳过消费者启动")
		return
	}
	if err := c.EnsureGroup(ctx, c.StreamName, evalGroup); err != nil {
		log.Printf("[consumer] 创建消费者组失败: %v", err)
	}

	consumerName := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
	// 启动周期性清扫 goroutine：把投递次数超阈值、卡在 Pending 的消息挪入死信流。
	c.startSweeper(ctx, c.StreamName, evalGroup, consumerName)
	go func() {
		log.Printf("[consumer] 消费者 %s 启动，stream=%s", consumerName, c.StreamName)
		for {
			select {
			case <-ctx.Done():
				log.Println("[consumer] 消费者退出")
				return
			default:
			}
		msgs, err := c.Consume(ctx, c.StreamName, evalGroup, consumerName, 50, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// 消费者组尚未创建（首次启动竞态）或 stream 不存在时自动重建
			if isNoGroup(err) {
				if e := c.EnsureGroup(ctx, c.StreamName, evalGroup); e != nil {
					log.Printf("[consumer] 重建消费者组失败: %v", e)
				}
			} else {
				log.Printf("[consumer] 读取消息失败: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
			if len(msgs) == 0 {
				continue
			}
			// 解析全部消息
			var parsed []EvalSubmitMsg
			var ackIDs []string
			for _, m := range msgs {
				var msg EvalSubmitMsg
				if err := parseMsg(m.Values, &msg); err != nil {
					log.Printf("[consumer] 消息解析失败 id=%s: %v", m.ID, err)
					ackIDs = append(ackIDs, m.ID) // 无法解析的消息直接确认，避免死循环
					continue
				}
				msg.MsgID = m.ID
				parsed = append(parsed, msg)
			}
			if len(parsed) == 0 {
				if len(ackIDs) > 0 {
					c.Ack(ctx, c.StreamName, evalGroup, ackIDs...)
				}
				continue
			}
			// 应用层批量处理，返回应确认的 ID
			ackIDs = persist(ctx, db, parsed)
			if len(ackIDs) > 0 {
				if err := c.Ack(ctx, c.StreamName, evalGroup, ackIDs...); err != nil {
					log.Printf("[consumer] ACK 失败: %v", err)
				}
			}
		}
	}()
}

// startSweeper 启动周期性清扫：认领 Pending 中投递次数超过 maxDelivery 的消息并挪入死信流。
// 目的是避免"永远无法成功的消息无限重试、积压在 Pending 且无告警"。ctx 取消时退出。
func (c *Client) startSweeper(ctx context.Context, stream, group, consumer string) {
	if !c.Enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			c.sweepOnce(ctx, stream, group, consumer)
		}
	}()
}

// sweepOnce 单次清扫：从 Pending Summary 里找出投递次数超阈值的消息，整体挪入死信流。
// 单实例安全；多消费者实例下多个清扫器可能重复认领，但 MoveToDead 内部幂等（认领不到就跳过）。
func (c *Client) sweepOnce(ctx context.Context, stream, group, consumer string) {
	pending, err := c.PendingSummary(ctx, stream, group, 1000)
	if err != nil {
		if ctx.Err() == nil && !isNoGroup(err) {
			log.Printf("[consumer] 清扫读取 Pending 失败: %v", err)
		}
		return
	}
	var deadIDs []string
	retries := map[string]int64{}
	for _, p := range pending {
		if isDeadLetter(p) {
			deadIDs = append(deadIDs, p.ID)
			retries[p.ID] = p.RetryCount
		}
	}
	if len(deadIDs) == 0 {
		return
	}
	if err := c.MoveToDead(ctx, stream, group, consumer, deadIDs, retries); err != nil {
		log.Printf("[consumer] 清扫挪死信失败: %v", err)
	}
}

// isDeadLetter 判定一条 Pending 消息是否为死信。满足其一即判死信：
//  1) 投递次数超阈值（RetryCount >= maxDelivery）：反复处理仍失败，大概率永远无法成功。
//  2) 停留时长超阈值（Idle >= maxIdleTime）：卡在 Pending 过久（如消费者长期未启动/崩溃），
//     用户已离开提交页，这条评教再落库意义不大，挪入死信流告警而非静默滞留。
func isDeadLetter(p redis.XPendingExt) bool {
	return p.RetryCount >= maxDelivery || p.Idle >= maxIdleTime
}

// parseMsg 从 Stream 字段映射解析为 EvalSubmitMsg。
func parseMsg(values map[string]interface{}, msg *EvalSubmitMsg) error {
	var (
		taskID  int
		userID  int
		anon    bool
		rawJSON json.RawMessage
	)
	var err error
	if taskID, err = intField(values, "task_id"); err != nil {
		return err
	}
	if userID, err = intField(values, "user_id"); err != nil {
		return err
	}
	if v, ok := values["is_anonymous"]; ok {
		anon = v == "true" || v == true
	}
	dv, ok := values["dimension_values"]
	if !ok {
		return fmt.Errorf("缺少 dimension_values")
	}
	rawJSON = []byte(fmt.Sprint(dv))
	msg.TaskID = taskID
	msg.UserID = userID
	msg.IsAnonymous = anon
	msg.DimensionValues = rawJSON
	return err
}

// intField 读取流字段（可能为 string 或 json.Number）
func intField(values map[string]interface{}, key string) (int, error) {
	v, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("缺少 %s", key)
	}
	switch n := v.(type) {
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err != nil {
			return 0, fmt.Errorf("%s 解析失败: %v", key, err)
		}
		return i, nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	case float64:
		return int(n), nil
	}
	return 0, fmt.Errorf("%s 类型不支持: %T", key, v)
}
