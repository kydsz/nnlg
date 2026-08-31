package handler

import (
	"log"

	"backend-go/internal/cache"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Queue 异步队列管理接口（仅系统管理员）：查看队列健康、重放死信。
// 面向评价异步落库的 Redis Stream 消费者组。
type Queue struct {
	db *gorm.DB
}

func NewQueue(db *gorm.DB) *Queue {
	return &Queue{db: db}
}

// Status 返回主队列与死信流的状态（待处理数 / 死信数），用于管理端监控。
func (h *Queue) Status(c *gin.Context) {
	rdb := cache.GetClient()
	if rdb == nil || !rdb.Enabled {
		response.OK(c, gin.H{"enabled": false})
		return
	}
	ctx := c.Request.Context()
	pending, _ := rdb.PendingCount(ctx, rdb.StreamName, cache.EvalGroupName())
	deadStream := rdb.DeadStream(rdb.StreamName)
	deadLen, _ := rdb.StreamLen(ctx, deadStream)
	response.OK(c, gin.H{
		"enabled": true, "stream": rdb.StreamName,
		"pending": pending, "dead": deadLen, "dead_stream": deadStream,
	})
}

// ReplayDead 将死信流中的消息重放回主队列，交由消费者重新处理。
// 重放后死信消息保留在死信流（不做删除），由人工确认处理结果。
// 幂等：重复调用只会把当前死信流内容再次入队，不会破坏已有数据。
func (h *Queue) ReplayDead(c *gin.Context) {
	rdb := cache.GetClient()
	if rdb == nil || !rdb.Enabled {
		response.Fail(c, 400, "Redis 未启用，无死信流可重放")
		return
	}
	ctx := c.Request.Context()
	deadStream := rdb.DeadStream(rdb.StreamName)
	replayed, err := rdb.ReplayDead(ctx, deadStream, rdb.StreamName, 1000)
	if err != nil {
		log.Printf("[queue] 死信重放失败: %v", err)
		response.Fail(c, 500, "死信重放失败: "+err.Error())
		return
	}
	response.OKMsg(c, "死信重放完成", gin.H{"replayed": replayed, "dead_stream": deadStream})
}
