package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backend-go/internal/config"

	"github.com/redis/go-redis/v9"
)

// Client 对 go-redis v9 的轻量封装，提供本项目所需的高并发能力：
//  1. JSON 缓存读写（统计报表缓存）
//  2. SETNX 幂等（评价提交防重）
//  3. Redis Stream 生产/消费（评价异步落库削峰）
//
// Redis 不可用时应优雅降级：所有方法在客户端为 nil 或出错时返回零值/错误，
// 由调用方决定回退到直连 DB 或同步落库。
type Client struct {
	rdb *redis.Client
	// Enabled 表示 Redis 是否已初始化可用。为 false 时所有缓存/队列操作自动旁路。
	Enabled bool
	// StreamName 评价提交队列（默认 eval:queue，可通过配置覆盖）
	StreamName string
}

var (
	defaultClient *Client
	defaultCtx    = context.Background()
)

// New 根据配置初始化 Redis 客户端。addr 为空时返回禁用状态（不连接）。
func New(cfg *config.Config) *Client {
	c := &Client{}
	if cfg.RedisAddr == "" {
		return c // Enabled=false，全链路降级
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	// 首次连接探测，失败则标记禁用（避免每次请求都走网络超时拖慢接口）
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		// Redis 挂了不阻塞主流程，记录并禁用
		fmt.Printf("[cache] Redis 连接失败，已降级为直连模式: %v\n", err)
		return c
	}
	c.rdb = rdb
	c.Enabled = true
	c.StreamName = "eval:queue"
	defaultClient = c
	return c
}

// GetClient 返回全局单例，供 handler/service 使用。
func GetClient() *Client {
	return defaultClient
}

func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled {
		return fmt.Errorf("redis disabled")
	}
	return c.rdb.Ping(ctx).Err()
}

// Close 关闭连接（进程退出时调用）
func (c *Client) Close() error {
	if c.Enabled {
		return c.rdb.Close()
	}
	return nil
}

// ============ 通用 JSON 缓存 ============

// key 统一前缀，避免与其它应用冲突
func (c *Client) key(parts ...string) string {
	s := "tev:"
	for i, p := range parts {
		if i > 0 {
			s += ":"
		}
		s += p
	}
	return s
}

// KeyPrefix 返回统一前缀（供 handler 构建自定义 key）
func (c *Client) KeyPrefix() string {
	return "tev:"
}

// GetJSON 读缓存并反序列化到 out。未命中或失败返回 false。
func (c *Client) GetJSON(ctx context.Context, key string, out interface{}) bool {
	if !c.Enabled {
		return false
	}
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false
	}
	return true
}

// SetJSON 将 v 序列化后写入缓存并设置 TTL。
func (c *Client) SetJSON(ctx context.Context, key string, v interface{}, ttl time.Duration) {
	if !c.Enabled {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.rdb.Set(ctx, key, raw, ttl)
}

// Del 删除缓存 key（主动失效用）。
func (c *Client) Del(ctx context.Context, keys ...string) {
	if !c.Enabled || len(keys) == 0 {
		return
	}
	c.rdb.Del(ctx, keys...)
}

// ============ 幂等 SETNX ============

// SetNX 以 taskId:userId 为粒度做提交幂等。
// ok=true 表示本次获取锁成功（应放行）；ok=false 表示已提交过或 Redis 不可用（由调用方决定）。
// 注意：Redis 不可用时返回 ok=false 且 err!=nil，调用方需回退到 DB 判重。
func (c *Client) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if !c.Enabled {
		return false, fmt.Errorf("redis disabled")
	}
	ok, err := c.rdb.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// ============ Stream 生产 ============

// Publish 向 Stream 写入一条消息（评价提交入队）。返回消息 ID。
// Redis 不可用时返回错误，调用方应回退同步落库。
func (c *Client) Publish(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	if !c.Enabled {
		return "", fmt.Errorf("redis disabled")
	}
	return c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()
}
