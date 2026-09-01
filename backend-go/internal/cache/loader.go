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

// IncrSessionEpoch 自增某用户会话 epoch 并删除快照（登出/禁用/改密/删除）。
func IncrSessionEpoch(ctx context.Context, uid int) int64 {
	if c := GetClient(); c != nil && c.Enabled {
		c.DelAuthUser(ctx, uid)
		return c.IncrSessionEpoch(ctx, uid)
	}
	return 0
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
