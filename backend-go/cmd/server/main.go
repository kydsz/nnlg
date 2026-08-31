package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"backend-go/internal/cache"
	"backend-go/internal/config"
	"backend-go/internal/database"
	"backend-go/internal/model"
	"backend-go/internal/router"
	"backend-go/internal/service"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// .env 可选，环境变量优先
	_ = godotenv.Load()
	normalizeDebug()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 初始化 Redis（未配置自动禁用，不影响原有功能）
	rdb := cache.New(cfg)
	if rdb.Enabled {
		log.Printf("[cache] Redis 已启用 %s", cfg.RedisAddr)
		// 启动评价异步落库消费者（批量攒批落库，降低 MySQL 写放大）
		svc := service.NewEvaluation()
		rdb.RunConsumer(context.Background(), db, func(ctx context.Context, d *gorm.DB, msgs []cache.EvalSubmitMsg) []string {
			var pending []service.PendingSubmit
			var ackIDs []string
			for _, msg := range msgs {
				// 幂等兜底：消费前校验（防 Redis 幂等键异常导致重复落库）。
				// 校验失败（如已被其它消费者落库/任务取消/权限变化）视为已处理，直接确认。
				var viewer model.User
				if err := d.First(&viewer, msg.UserID).Error; err != nil {
					log.Printf("[consumer] 用户不存在 uid=%d, 确认消息", msg.UserID)
					ackIDs = append(ackIDs, msg.MsgID)
					continue
				}
				var values map[string]interface{}
				if err := json.Unmarshal(msg.DimensionValues, &values); err != nil {
					log.Printf("[consumer] 维度值解析失败 msg=%s, 确认消息", msg.MsgID)
					ackIDs = append(ackIDs, msg.MsgID)
					continue
				}
				p := service.SubmitParams{TaskID: msg.TaskID, DimensionValues: values, IsAnonymous: msg.IsAnonymous}
				task, role, err := svc.ValidateSubmit(d, &viewer, p)
				if err != nil {
					// 仅对"确定不可处理"的业务拒绝确认丢弃；其余（含 DB 瞬时故障被折叠为"任务不存在"）
					// 不 ACK，留在 Pending 重试（死信清扫兜底），避免瞬时故障静默丢数据。
					if isPermanentRejection(err) {
						ackIDs = append(ackIDs, msg.MsgID)
					} else {
						log.Printf("[consumer] 校验失败但可能瞬时，消息留待重试 id=%s: %v", msg.MsgID, err)
					}
					continue
				}
				pending = append(pending, service.PendingSubmit{
					MsgID: msg.MsgID, Task: task, EvaluatorRole: role, Params: p, Viewer: &viewer,
				})
			}
			// 攒批落库：一次事务批量 INSERT + 按 task 聚合计数 UPDATE
			if len(pending) > 0 {
				if err := svc.PersistBatch(d, pending); err != nil {
					// 落库失败不 ACK，整批留在 Pending List 待重试
					log.Printf("[consumer] 批量落库失败 %d 条: %v", len(pending), err)
					return ackIDs
				}
				for _, it := range pending {
					ackIDs = append(ackIDs, it.MsgID)
				}
			}
			return ackIDs
		})
	} else {
		log.Println("[cache] Redis 未配置，运行于直连 DB 模式")
	}

	r := router.Setup(cfg, db)

	log.Printf("%s v%s 启动于 :%s", cfg.AppName, cfg.Version, cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// normalizeDebug 清理非布尔值的 DEBUG 环境变量
// （如本机全局 DEBUG=release 会干扰解析，移除后让 .env 的 DEBUG 生效）
func normalizeDebug() {
	switch v := os.Getenv("DEBUG"); strings.ToLower(v) {
	case "", "true", "false", "1", "0":
	default:
		_ = os.Unsetenv("DEBUG")
	}
}

// permanentRejections 校验中"确定不可处理"的业务拒绝消息。
// 命中这些错误的消息在消费者中直接确认丢弃（重试无意义）；未命中（含 DB 瞬时故障被折叠为
// "任务不存在"等）则留待重试，避免瞬时故障静默丢失已提交的评教。
var permanentRejections = []string{
	"您已提交过本次评教",
	"任务已取消，不能提交评教",
	"不能评自己",
	"您只能评自己负责学院的教师",
	"您没有该学院的评教权限",
	"只能评同学院的教师",
}

// isPermanentRejection 判断校验错误是否为确定不可处理的业务拒绝。
func isPermanentRejection(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, p := range permanentRejections {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
