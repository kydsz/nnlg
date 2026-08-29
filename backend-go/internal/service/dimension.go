package service

import (
	"encoding/json"
	"errors"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// DimensionService 评教维度业务
type Dimension struct{}

func NewDimension() *Dimension { return &Dimension{} }

// ---------- 分组 ----------

// GroupList 分组列表（附带启用维度数）；status 为 nil 时默认仅返回启用（对齐旧端语义）
func (s *Dimension) GroupList(db *gorm.DB, status *int16) ([]map[string]interface{}, error) {
	var groups []model.DimensionGroup
	q := db.Model(&model.DimensionGroup{})
	if status != nil {
		q = q.Where("status = ?", *status)
	} else {
		q = q.Where("status = 1")
	}
	if err := q.Order("sort_order ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	type row struct {
		GroupID int
		Cnt     int64
	}
	var rows []row
	db.Model(&model.EvaluationDimension{}).
		Select("group_id, COUNT(*) AS cnt").Where("status = 1 AND group_id IS NOT NULL").
		Group("group_id").Scan(&rows)
	counts := map[int]int{}
	for _, r := range rows {
		counts[r.GroupID] = int(r.Cnt)
	}
	out := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		out = append(out, map[string]interface{}{
			"id": g.ID, "code": g.Code, "name": g.Name,
			"sort_order": g.SortOrder, "status": g.Status,
			"dimension_count": counts[g.ID], "create_time": g.CreateTime,
		})
	}
	return out, nil
}

// GroupParams 分组参数
type GroupParams struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder *int   `json:"sort_order"`
	Status    *int16 `json:"status"`
}

// GroupCreate 新建分组
func (s *Dimension) GroupCreate(db *gorm.DB, p GroupParams) (*model.DimensionGroup, error) {
	if p.Code == "" {
		return nil, errors.New("分组编码不能为空")
	}
	if p.Name == "" {
		return nil, errors.New("分组名称不能为空")
	}
	var cnt int64
	db.Model(&model.DimensionGroup{}).Where("code = ?", p.Code).Count(&cnt)
	if cnt > 0 {
		return nil, errors.New("分组编码已存在")
	}
	g := model.DimensionGroup{Code: p.Code, Name: p.Name, SortOrder: derefInt(p.SortOrder), Status: 1}
	if p.Status != nil {
		g.Status = *p.Status
	}
	if err := db.Create(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// GroupSort 批量排序
func (s *Dimension) GroupSort(db *gorm.DB, items []GroupSortItem) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			if err := tx.Model(&model.DimensionGroup{}).Where("id = ?", it.ID).Update("sort_order", it.SortOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GroupSortItem 排序条目
type GroupSortItem struct {
	ID        int `json:"id"`
	SortOrder int `json:"sort_order"`
}

// GroupUpdate 更新分组
func (s *Dimension) GroupUpdate(db *gorm.DB, id int, p GroupParams) (*model.DimensionGroup, error) {
	var g model.DimensionGroup
	if err := db.First(&g, id).Error; err != nil {
		return nil, errors.New("分组不存在")
	}
	updates := map[string]interface{}{}
	if p.Code != "" && p.Code != g.Code {
		var cnt int64
		db.Model(&model.DimensionGroup{}).Where("code = ? AND id <> ?", p.Code, id).Count(&cnt)
		if cnt > 0 {
			return nil, errors.New("分组编码已存在")
		}
		updates["code"] = p.Code
	}
	if p.Name != "" {
		updates["name"] = p.Name
	}
	if p.SortOrder != nil {
		updates["sort_order"] = *p.SortOrder
	}
	if p.Status != nil {
		updates["status"] = *p.Status
	}
	if len(updates) > 0 {
		if err := db.Model(&g).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return &g, nil
}

// GroupDelete 删除分组（有启用维度时禁止）
func (s *Dimension) GroupDelete(db *gorm.DB, id int) error {
	var cnt int64
	db.Model(&model.EvaluationDimension{}).Where("group_id = ? AND status = 1", id).Count(&cnt)
	if cnt > 0 {
		return errors.New("分组下存在启用的维度，禁止删除")
	}
	return db.Delete(&model.DimensionGroup{}, id).Error
}

// ---------- 维度 ----------

// DimParams 维度查询条件
type DimParams struct {
	Keyword   string
	GroupID   string
	FieldType string
	Status    *int16
}

// FieldTypeNames 字段类型中文名（对齐旧端 FIELD_TYPE_NAMES）
var FieldTypeNames = map[string]string{
	model.FieldScore:         "评分",
	model.FieldSingleChoice:  "单选",
	model.FieldMultipleChoce: "多选",
	model.FieldText:          "文本",
	model.FieldNumber:        "数字",
	model.FieldDate:          "日期",
	model.FieldDatetime:      "日期时间",
	model.FieldRichText:      "富文本",
	model.FieldImage:         "图片上传",
	model.FieldFile:          "文件上传",
}

// DimItem 导出维度输出（handler 组装创建/更新响应用，对齐旧端字段）
func DimItem(d *model.EvaluationDimension) map[string]interface{} { return dimItem(d) }

// FieldTypeName 字段类型中文名（未映射时返回原值，对齐旧端 FIELD_TYPE_NAMES.get(ft, ft)）
func FieldTypeName(ft string) string {
	if n, ok := FieldTypeNames[ft]; ok {
		return n
	}
	return ft
}

// dimItem 维度输出（对齐旧端列表/详情字段）
func dimItem(d *model.EvaluationDimension) map[string]interface{} {
	var groupName interface{}
	if d.Group != nil {
		groupName = d.Group.Name
	}
	return map[string]interface{}{
		"id": d.ID, "group_id": d.GroupID, "group_name": groupName,
		"code": d.Code, "name": d.Name,
		"field_type": d.FieldType, "field_type_name": FieldTypeName(d.FieldType),
		"field_config": d.FieldConfig, "description": d.Description,
		"sort_order": d.SortOrder, "is_required": d.IsRequired, "status": d.Status,
		"create_time": d.CreateTime,
	}
}

// DimList 维度列表（分页）
func (s *Dimension) DimList(db *gorm.DB, p DimParams, page, pageSize int) ([]map[string]interface{}, int64, error) {
	q := db.Model(&model.EvaluationDimension{})
	if p.Keyword != "" {
		kw := "%" + p.Keyword + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", kw, kw)
	}
	if p.GroupID != "" {
		q = q.Where("group_id = ?", p.GroupID)
	}
	if p.FieldType != "" {
		q = q.Where("field_type = ?", p.FieldType)
	}
	if p.Status != nil {
		q = q.Where("status = ?", *p.Status)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.EvaluationDimension
	err := q.Session(&gorm.Session{}).Preload("Group").
		Order("sort_order ASC, id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]interface{}, 0, len(list))
	for i := range list {
		out = append(out, dimItem(&list[i]))
	}
	return out, total, nil
}

// DimGet 维度详情
func (s *Dimension) DimGet(db *gorm.DB, id int) (map[string]interface{}, error) {
	var d model.EvaluationDimension
	if err := db.Preload("Group").First(&d, id).Error; err != nil {
		return nil, errors.New("评教维度不存在")
	}
	item := dimItem(&d)
	item["update_time"] = d.UpdateTime
	return item, nil
}

// DimActive 启用维度列表（评教表单用；对齐旧端：join 分组，先按分组 sort_order 再按维度 sort_order）
func (s *Dimension) DimActive(db *gorm.DB) ([]map[string]interface{}, error) {
	var dims []model.EvaluationDimension
	if err := db.Preload("Group").
		Joins("JOIN dimension_group g ON g.id = evaluation_dimension.group_id").
		Where("evaluation_dimension.status = 1").
		Order("g.sort_order ASC, evaluation_dimension.sort_order ASC").
		Find(&dims).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(dims))
	for _, d := range dims {
		var groupName interface{}
		if d.Group != nil {
			groupName = d.Group.Name
		}
		out = append(out, map[string]interface{}{
			"id": d.ID, "group_id": d.GroupID, "group_name": groupName,
			"code": d.Code, "name": d.Name,
			"field_type": d.FieldType, "field_type_name": FieldTypeName(d.FieldType),
			"field_config": d.FieldConfig, "description": d.Description,
			"sort_order": d.SortOrder, "is_required": d.IsRequired,
		})
	}
	return out, nil
}

// DimSort 批量更新维度排序
func (s *Dimension) DimSort(db *gorm.DB, items []GroupSortItem) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			if err := tx.Model(&model.EvaluationDimension{}).Where("id = ?", it.ID).
				Update("sort_order", it.SortOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DimensionParams 维度 create/update 参数
type DimensionParams struct {
	GroupID     *int            `json:"group_id"`
	Code        *string         `json:"code"`
	Name        *string         `json:"name"`
	FieldType   *string         `json:"field_type"`
	FieldConfig json.RawMessage `json:"field_config"`
	Description *string         `json:"description"`
	SortOrder   *int            `json:"sort_order"`
	IsRequired  *bool           `json:"is_required"`
	Status      *int16          `json:"status"`
}

// DimCreate 新建维度
func (s *Dimension) DimCreate(db *gorm.DB, p DimensionParams) (*model.EvaluationDimension, error) {
	if p.Code == nil || p.Name == nil || p.FieldType == nil {
		return nil, errors.New("code/name/field_type 为必填")
	}
	if !model.ValidFieldType(*p.FieldType) {
		return nil, errors.New("非法字段类型: " + *p.FieldType)
	}
	var cnt int64
	db.Model(&model.EvaluationDimension{}).Where("code = ?", *p.Code).Count(&cnt)
	if cnt > 0 {
		return nil, errors.New("维度编码已存在")
	}
	d := model.EvaluationDimension{
		GroupID: p.GroupID, Code: *p.Code, Name: *p.Name, FieldType: *p.FieldType,
		FieldConfig: p.FieldConfig, Description: p.Description,
		SortOrder: derefInt(p.SortOrder), IsRequired: true, Status: 1,
	}
	if p.IsRequired != nil {
		d.IsRequired = *p.IsRequired
	}
	if p.Status != nil {
		d.Status = *p.Status
	}
	if err := db.Create(&d).Error; err != nil {
		return nil, err
	}
	if d.GroupID != nil {
		db.Preload("Group").First(&d, d.ID) // 回填分组名（对齐旧端响应）
	}
	return &d, nil
}

// DimUpdate 更新维度；被评教记录引用后禁止修改 code/field_type
func (s *Dimension) DimUpdate(db *gorm.DB, id int, p DimensionParams) (*model.EvaluationDimension, error) {
	var d model.EvaluationDimension
	if err := db.First(&d, id).Error; err != nil {
		return nil, errors.New("维度不存在")
	}
	if p.FieldType != nil && !model.ValidFieldType(*p.FieldType) {
		return nil, errors.New("非法字段类型: " + *p.FieldType)
	}

	// 引用检查：dimension_values JSON 中包含 "code" 键
	if (p.Code != nil && *p.Code != d.Code) || (p.FieldType != nil && *p.FieldType != d.FieldType) {
		var cnt int64
		db.Model(&model.EvaluationRecord{}).
			Where("dimension_values LIKE ?", "%\""+d.Code+"\"%").Count(&cnt)
		if cnt > 0 {
			return nil, errors.New("该维度已被评教记录引用，禁止修改编码或字段类型")
		}
	}

	updates := map[string]interface{}{}
	if p.GroupID != nil {
		updates["group_id"] = *p.GroupID
	}
	if p.Code != nil {
		updates["code"] = *p.Code
	}
	if p.Name != nil {
		updates["name"] = *p.Name
	}
	if p.FieldType != nil {
		updates["field_type"] = *p.FieldType
	}
	if p.FieldConfig != nil {
		updates["field_config"] = p.FieldConfig
	}
	if p.Description != nil {
		updates["description"] = *p.Description
	}
	if p.SortOrder != nil {
		updates["sort_order"] = *p.SortOrder
	}
	if p.IsRequired != nil {
		updates["is_required"] = *p.IsRequired
	}
	if p.Status != nil {
		updates["status"] = *p.Status
	}
	if len(updates) > 0 {
		if err := db.Model(&d).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	db.Preload("Group").First(&d, d.ID) // 回填分组名（对齐旧端响应）
	return &d, nil
}

// DimDelete 删除维度
func (s *Dimension) DimDelete(db *gorm.DB, id int) error {
	return db.Delete(&model.EvaluationDimension{}, id).Error
}

// SchemaForEvaluation 评教用维度结构（仅启用项，按分组聚合）
func (s *Dimension) SchemaForEvaluation(db *gorm.DB) ([]map[string]interface{}, error) {
	var groups []model.DimensionGroup
	if err := db.Where("status = 1").Order("sort_order ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	var dims []model.EvaluationDimension
	if err := db.Where("status = 1").Order("sort_order ASC, id ASC").Find(&dims).Error; err != nil {
		return nil, err
	}
	byGroup := map[int][]map[string]interface{}{}
	orphan := []map[string]interface{}{}
	for _, d := range dims {
		item := map[string]interface{}{
			"id": d.ID, "code": d.Code, "name": d.Name, "field_type": d.FieldType,
			"field_config": json.RawMessage(d.FieldConfig), "description": d.Description,
			"is_required": d.IsRequired, "sort_order": d.SortOrder,
		}
		if d.FieldConfig == nil {
			item["field_config"] = nil
		}
		if d.FieldType == model.FieldScore {
			if max, ok := fieldConfigNum(d.FieldConfig, "max_score"); ok {
				item["max_score"] = max
			}
		}
		if d.GroupID != nil {
			byGroup[*d.GroupID] = append(byGroup[*d.GroupID], item)
		} else {
			orphan = append(orphan, item)
		}
	}
	out := []map[string]interface{}{}
	for _, g := range groups {
		ds := byGroup[g.ID]
		if ds == nil {
			ds = []map[string]interface{}{}
		}
		out = append(out, map[string]interface{}{
			"id": g.ID, "code": g.Code, "name": g.Name,
			"sort_order": g.SortOrder, "dimensions": ds,
		})
	}
	if len(orphan) > 0 {
		out = append(out, map[string]interface{}{
			"id": 0, "code": "ungrouped", "name": "未分组",
			"sort_order": 9999, "dimensions": orphan,
		})
	}
	return out, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
