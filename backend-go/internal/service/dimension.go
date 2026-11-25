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

// ---------- 模板口径 ----------

// templateGroupCond 返回“分组属于指定模板（含 common 通用分组）”的 SQL 条件（含两个占位符）。
// column 为分组表 template_code 的列名：单表查询传 "template_code"，JOIN 后传 "g.template_code"。
// 评教表单、提交校验、满分合计、维度管理统一走此条件，避免多处手写导致口径漂移。
func templateGroupCond(column string) string {
	return "(" + column + " = ? OR " + column + " = ?)"
}

// activeDimensionsOfTemplate 取指定模板的启用分组与启用维度。
// 分组按 sort_order 排序；维度已按「分组排序 + 维度排序」排列（与评教表单展示顺序一致）。
// 未分组维度视为存量理论课维度，仅理论课模板返回。
func activeDimensionsOfTemplate(db *gorm.DB, template string) ([]model.DimensionGroup, []model.EvaluationDimension, error) {
	template = model.NormalizeTemplate(template)
	var groups []model.DimensionGroup
	if err := db.Where("status = 1 AND "+templateGroupCond("template_code"), template, model.TemplateCommon).
		Order("sort_order ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, nil, err
	}

	byGroup := make(map[int][]model.EvaluationDimension, len(groups))
	ids := make([]int, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	if len(ids) > 0 {
		var dims []model.EvaluationDimension
		if err := db.Preload("Group").
			Where("status = 1 AND group_id IN ?", ids).
			Order("sort_order ASC, id ASC").Find(&dims).Error; err != nil {
			return nil, nil, err
		}
		for _, d := range dims {
			byGroup[*d.GroupID] = append(byGroup[*d.GroupID], d)
		}
	}

	var orphan []model.EvaluationDimension
	if template == model.TemplateTheory {
		if err := db.Where("status = 1 AND group_id IS NULL").
			Order("sort_order ASC, id ASC").Find(&orphan).Error; err != nil {
			return nil, nil, err
		}
	}

	out := make([]model.EvaluationDimension, 0, len(orphan))
	for _, g := range groups {
		out = append(out, byGroup[g.ID]...)
	}
	return groups, append(out, orphan...), nil
}

// ---------- 分组 ----------

// GroupList 分组列表（附带启用维度数）；status 为 nil 时默认仅返回启用（对齐旧端语义）
// template 非空时按模板过滤（含 common 通用分组）。
func (s *Dimension) GroupList(db *gorm.DB, status *int16, template string) ([]map[string]interface{}, error) {
	var groups []model.DimensionGroup
	q := db.Model(&model.DimensionGroup{})
	if status != nil {
		q = q.Where("status = ?", *status)
	} else {
		q = q.Where("status = 1")
	}
	if template != "" {
		q = q.Where(templateGroupCond("template_code"), template, model.TemplateCommon)
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
			"id": g.ID, "code": g.Code, "name": g.Name, "template_code": g.TemplateCode,
			"sort_order": g.SortOrder, "status": g.Status,
			"dimension_count": counts[g.ID], "create_time": g.CreateTime,
		})
	}
	return out, nil
}

// GroupParams 分组参数
type GroupParams struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	TemplateCode string `json:"template_code"`
	SortOrder    *int   `json:"sort_order"`
	Status       *int16 `json:"status"`
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
	if p.TemplateCode == "" {
		g.TemplateCode = model.TemplateTheory
	} else if !model.ValidGroupTemplate(p.TemplateCode) {
		return nil, errors.New("非法评价模板: " + p.TemplateCode)
	} else {
		g.TemplateCode = p.TemplateCode
	}
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
	if p.TemplateCode != "" {
		if !model.ValidGroupTemplate(p.TemplateCode) {
			return nil, errors.New("非法评价模板: " + p.TemplateCode)
		}
		updates["template_code"] = p.TemplateCode
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
	Template  string
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
	var groupName, templateCode interface{}
	if d.Group != nil {
		groupName = d.Group.Name
		templateCode = d.Group.TemplateCode
	}
	return map[string]interface{}{
		"id": d.ID, "group_id": d.GroupID, "group_name": groupName,
		"template_code": templateCode,
		"code":          d.Code, "name": d.Name,
		"field_type": d.FieldType, "field_type_name": FieldTypeName(d.FieldType),
		"field_config": d.FieldConfig, "description": d.Description,
		"sort_order": d.SortOrder, "is_required": d.IsRequired, "status": d.Status,
		"create_time": d.CreateTime,
	}
}

// DimList 维度列表（分页）
func (s *Dimension) DimList(db *gorm.DB, p DimParams, page, pageSize int) ([]map[string]interface{}, int64, error) {
	q := db.Model(&model.EvaluationDimension{})
	if p.Template != "" {
		// 子查询代替 JOIN，避免分页/计数时列名歧义；未分组维度按存量理论课维度处理，与评教表单口径一致。
		sub := db.Model(&model.DimensionGroup{}).Select("id").
			Where(templateGroupCond("template_code"), p.Template, model.TemplateCommon)
		if p.Template == model.TemplateTheory {
			q = q.Where("(group_id IN (?) OR group_id IS NULL)", sub)
		} else {
			q = q.Where("group_id IN (?)", sub)
		}
	}
	if p.Keyword != "" {
		kw := "%" + p.Keyword + "%"
		q = q.Where("evaluation_dimension.code LIKE ? OR evaluation_dimension.name LIKE ?", kw, kw)
	}
	if p.GroupID != "" {
		q = q.Where("evaluation_dimension.group_id = ?", p.GroupID)
	}
	if p.FieldType != "" {
		q = q.Where("evaluation_dimension.field_type = ?", p.FieldType)
	}
	if p.Status != nil {
		q = q.Where("evaluation_dimension.status = ?", *p.Status)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.EvaluationDimension
	err := q.Session(&gorm.Session{}).Preload("Group").
		Order("evaluation_dimension.sort_order ASC, evaluation_dimension.id ASC").
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

// DimActive 启用维度列表（评教表单用；按「分组排序 + 维度排序」输出）
// template 指定评价模板，返回该模板维度 + common 通用维度。
func (s *Dimension) DimActive(db *gorm.DB, template string) ([]map[string]interface{}, error) {
	_, dims, err := activeDimensionsOfTemplate(db, template)
	if err != nil {
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

// SchemaForEvaluation 评教用维度结构（仅启用项，按分组聚合；按模板过滤）
func (s *Dimension) SchemaForEvaluation(db *gorm.DB, template string) ([]map[string]interface{}, error) {
	groups, dims, err := activeDimensionsOfTemplate(db, template)
	if err != nil {
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
