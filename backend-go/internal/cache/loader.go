package cache

import (
	"context"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// LoadUser 从用户快照缓存加载用户；未命中回源 DB（含关联与权限解析）并写缓存。
// Redis 未启用时不写缓存、仅直连 DB。禁用用户（status!=1）不缓存。
func LoadUser(ctx context.Context, db *gorm.DB, uid int) (*model.User, error) {
	c := GetClient()
	if c != nil && c.Enabled {
		if snap, ok := c.GetAuthUser(ctx, uid); ok {
			return snap.ToUser(), nil
		}
	}

	var u model.User
	err := db.
		Preload("UserRoles").
		Preload("UserColleges.College").
		Preload("UserRooms.ResearchRoom.College").
		Preload("College").
		Preload("ResearchRoom").
		First(&u, uid).Error
	if err != nil {
		return nil, err
	}

	perms := u.Permissions(db)
	u.Perms = perms
	if c != nil && c.Enabled && u.Status == 1 {
		c.SetAuthUser(ctx, uid, SnapshotFromUser(&u))
	}
	return &u, nil
}

// InvalidateUserAuth 主动失效某用户快照（包级便利，Redis 未启用时静默）。
func InvalidateUserAuth(ctx context.Context, uid int) {
	if c := GetClient(); c != nil && c.Enabled {
		c.DelAuthUser(ctx, uid)
	}
}

// syncEpochCache 同步 Redis 会话 epoch 加速缓存。
// 写失败时删除 key：保证缓存要么是最新值、要么不存在（读回退 DB），
// 避免陈旧缓存凌驾 DB 权威导致新 token 被误判失效。
func syncEpochCache(ctx context.Context, uid int, epoch int64) {
	c := GetClient()
	if c == nil || !c.Enabled {
		return
	}
	if err := c.rdb.Set(ctx, c.SessionEpochKey(uid), epoch, 30*24*time.Hour).Err(); err != nil {
		c.rdb.Del(ctx, c.SessionEpochKey(uid))
	}
}

// SyncUserEpoch 将用户当前 DB 会话 epoch 同步到 Redis（登录成功后调用）：
// 消除升级前残留的陈旧 epoch 缓存，保证 Redis 加速值与 DB 权威一致。
func SyncUserEpoch(ctx context.Context, db *gorm.DB, uid int) {
	var epoch int64
	if err := db.Model(&model.User{}).Where("id = ?", uid).Select("session_epoch").Scan(&epoch).Error; err != nil {
		return
	}
	syncEpochCache(ctx, uid, epoch)
}

// GetSessionEpoch 读取会话 epoch（DB 权威撤销源）。
// Redis 缓存命中时直接返回（加速）；未命中或出错回退读取 user.session_epoch 列。
// DB 读取失败返回错误，调用方应 fail-closed（拒绝访问），不得静默放行。
func GetSessionEpoch(ctx context.Context, db *gorm.DB, uid int) (int64, error) {
	if c := GetClient(); c != nil && c.Enabled {
		if n, err := c.rdb.Get(ctx, c.SessionEpochKey(uid)).Int64(); err == nil {
			return n, nil
		}
		// 未命中（nil）或 Redis 出错：回退 DB 权威源
	}
	var epoch int64
	if err := db.Model(&model.User{}).Where("id = ?", uid).Select("session_epoch").Scan(&epoch).Error; err != nil {
		return 0, err
	}
	return epoch, nil
}

// IncrSessionEpoch 自增某用户会话 epoch 并删除快照（登出/禁用/改密/删除）。
// DB 自增为权威撤销源（原子 UPDATE + 读回），Redis 同步刷新值（失败则删 key 兜底回退 DB）。
func IncrSessionEpoch(ctx context.Context, db *gorm.DB, uid int) (int64, error) {
	if err := db.Model(&model.User{}).Where("id = ?", uid).
		UpdateColumn("session_epoch", gorm.Expr("session_epoch + 1")).Error; err != nil {
		return 0, err
	}
	var epoch int64
	if err := db.Model(&model.User{}).Where("id = ?", uid).Select("session_epoch").Scan(&epoch).Error; err != nil {
		return 0, err
	}
	if c := GetClient(); c != nil && c.Enabled {
		c.DelAuthUser(ctx, uid)
	}
	syncEpochCache(ctx, uid, epoch)
	return epoch, nil
}

// dimCacheTTL 启用维度缓存时长。
const dimCacheTTL = 60 * time.Second

// DimCacheKey 启用维度缓存 key：tev:dim:active
func (c *Client) DimCacheKey() string { return c.key("dim", "active") }

// GetActiveDimensions 读启用维度缓存（evaluation_dimension where status=1）。
func (c *Client) GetActiveDimensions(ctx context.Context) ([]model.EvaluationDimension, bool) {
	if c == nil || !c.Enabled {
		return nil, false
	}
	var dims []model.EvaluationDimension
	if !c.GetJSON(ctx, c.DimCacheKey(), &dims) {
		return nil, false
	}
	return dims, true
}

// SetActiveDimensions 写启用维度缓存。
func (c *Client) SetActiveDimensions(ctx context.Context, dims []model.EvaluationDimension) {
	if c == nil || !c.Enabled {
		return
	}
	if dims == nil {
		dims = []model.EvaluationDimension{}
	}
	c.SetJSON(ctx, c.DimCacheKey(), dims, dimCacheTTL)
}

// DelActiveDimensions 主动失效启用维度缓存（维度管理增删改时调用）。
func (c *Client) DelActiveDimensions(ctx context.Context) {
	if c == nil || !c.Enabled {
		return
	}
	c.Del(ctx, c.DimCacheKey())
}

// InvalidateDimensionCache 主动失效启用维度缓存（包级便利）。
func InvalidateDimensionCache(ctx context.Context) {
	if c := GetClient(); c != nil && c.Enabled {
		c.DelActiveDimensions(ctx)
	}
}
