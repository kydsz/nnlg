package service

import (
	"context"

	"backend-go/internal/cache"
	"backend-go/internal/model"

	"gorm.io/gorm"
)

// loadActiveDimensions 读取启用维度列表（缓存优先，未命中回源 DB 并写 60 秒缓存）。
// validateDimensionValues / scoreDimCodeSet / totalMaxScoreOfSchema / FileDimMap 共用一份。
func loadActiveDimensions(db *gorm.DB) []model.EvaluationDimension {
	c := cache.GetClient()
	ctx := context.Background()
	if c != nil && c.Enabled {
		if dims, ok := c.GetActiveDimensions(ctx); ok {
			return dims
		}
	}
	var dims []model.EvaluationDimension
	db.Where("status = 1").Order("sort_order ASC").Find(&dims)
	if c != nil && c.Enabled {
		c.SetActiveDimensions(ctx, dims)
	}
	return dims
}
