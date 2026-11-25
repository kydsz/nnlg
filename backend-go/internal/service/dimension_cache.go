package service

import (
	"context"

	"backend-go/internal/cache"
	"backend-go/internal/model"

	"gorm.io/gorm"
)

// loadActiveDimensions 读取全部启用维度（缓存优先，未命中回源 DB 并写 60 秒缓存）。
// 供 FileDimMap / scoreDimCodeSet 等需要跨模板汇总的场景使用。
func loadActiveDimensions(db *gorm.DB) []model.EvaluationDimension {
	c := cache.GetClient()
	ctx := context.Background()
	if c != nil && c.Enabled {
		if dims, ok := c.GetActiveDimensions(ctx, ""); ok {
			return dims
		}
	}
	var dims []model.EvaluationDimension
	db.Where("status = 1").Order("sort_order ASC").Find(&dims)
	if c != nil && c.Enabled {
		c.SetActiveDimensions(ctx, dims, "")
	}
	return dims
}

// loadActiveDimensionsOfTemplate 读取指定模板的启用维度（含 common 通用维度）。
// 评教表单、提交校验、满分合计等按模板口径的调用方使用。
func loadActiveDimensionsOfTemplate(db *gorm.DB, template string) []model.EvaluationDimension {
	template = model.NormalizeTemplate(template)
	c := cache.GetClient()
	ctx := context.Background()
	if c != nil && c.Enabled {
		if dims, ok := c.GetActiveDimensions(ctx, template); ok {
			return dims
		}
	}
	// 查询失败时保持原语义：返回空列表（缓存不写），不阻断评教表单降级
	if _, dims, err := activeDimensionsOfTemplate(db, template); err == nil {
		if c != nil && c.Enabled {
			c.SetActiveDimensions(ctx, dims, template)
		}
		return dims
	}
	return nil
}
