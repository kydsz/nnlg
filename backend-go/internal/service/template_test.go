package service

import (
	"testing"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// templateTestDB 评价模板测试库：一套理论课/通用/实践教学维度。
func templateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.DimensionGroup{}, &model.EvaluationDimension{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	theory := model.DimensionGroup{Code: "attitude", Name: "教学态度", TemplateCode: model.TemplateTheory, SortOrder: 4, Status: 1}
	common := model.DimensionGroup{Code: "attendance", Name: "考勤信息", TemplateCode: model.TemplateCommon, SortOrder: 1, Status: 1}
	practice := model.DimensionGroup{Code: "practice_content", Name: "教学内容", TemplateCode: model.TemplatePractice, SortOrder: 12, Status: 1}
	for _, g := range []*model.DimensionGroup{&theory, &common, &practice} {
		if err := db.Create(g).Error; err != nil {
			t.Fatalf("灌入分组失败: %v", err)
		}
	}

	rows := []model.EvaluationDimension{
		{GroupID: &theory.ID, Code: "attitude_1", Name: "理论课态度项", FieldType: model.FieldScore,
			FieldConfig: []byte(`{"min_score":0,"max_score":10,"step":1}`), IsRequired: true, SortOrder: 1, Status: 1},
		{GroupID: &common.ID, Code: "expected_count", Name: "应到人数", FieldType: model.FieldNumber,
			FieldConfig: []byte(`{"min":0}`), IsRequired: false, SortOrder: 1, Status: 1},
		{GroupID: &practice.ID, Code: "prac_content_1", Name: "实践教学内容项", FieldType: model.FieldScore,
			FieldConfig: []byte(`{"min_score":0,"max_score":12,"step":1}`), IsRequired: true, SortOrder: 1, Status: 1},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("灌入维度失败: %v", err)
		}
	}
	return db
}

func codesOf(dims []model.EvaluationDimension) map[string]bool {
	m := map[string]bool{}
	for _, d := range dims {
		m[d.Code] = true
	}
	return m
}

// TestActiveDimensionsByTemplate 维度按模板隔离：理论课/实践教学各自评分项 + 通用项。
func TestActiveDimensionsByTemplate(t *testing.T) {
	db := templateTestDB(t)

	theory := codesOf(loadActiveDimensionsOfTemplate(db, model.TemplateTheory))
	if !theory["attitude_1"] || !theory["expected_count"] {
		t.Fatalf("理论课应包含理论课维度与通用维度, got %v", theory)
	}
	if theory["prac_content_1"] {
		t.Fatal("理论课不应包含实践教学评分项")
	}

	practice := codesOf(loadActiveDimensionsOfTemplate(db, model.TemplatePractice))
	if !practice["prac_content_1"] || !practice["expected_count"] {
		t.Fatalf("实践教学应包含实践维度与通用维度, got %v", practice)
	}
	if practice["attitude_1"] {
		t.Fatal("实践教学不应包含理论课评分项")
	}
}

// TestTotalMaxScoreByTemplate 满分按模板口径计算，不被另一套模板抬高。
func TestTotalMaxScoreByTemplate(t *testing.T) {
	db := templateTestDB(t)
	if got := totalMaxScoreOfSchema(db, model.TemplateTheory); got != 10 {
		t.Fatalf("理论课满分应为 10, got %v", got)
	}
	if got := totalMaxScoreOfSchema(db, model.TemplatePractice); got != 12 {
		t.Fatalf("实践教学满分应为 12, got %v", got)
	}
	if got := totalMaxScoreOfSchema(db, ""); got != 10 {
		t.Fatalf("空模板应按理论课口径 10, got %v", got)
	}
}

// TestValidateDimensionValuesByTemplate 提交校验按任务模板取必填项与分数上限。
func TestValidateDimensionValuesByTemplate(t *testing.T) {
	db := templateTestDB(t)

	// 实践教学必填项缺失 -> 报错
	if _, err := validateDimensionValues(db, map[string]interface{}{}, model.TemplatePractice); err == nil {
		t.Fatal("实践教学缺少必填评分项应报错")
	}
	// 实践教学完整作答 -> 通过
	if _, err := validateDimensionValues(db, map[string]interface{}{"prac_content_1": 12.0}, model.TemplatePractice); err != nil {
		t.Fatalf("实践教学完整作答应通过: %v", err)
	}
	// 超出实践项上限 12 -> 报错
	if _, err := validateDimensionValues(db, map[string]interface{}{"prac_content_1": 13.0}, model.TemplatePractice); err == nil {
		t.Fatal("实践教学评分越界应报错")
	}
	// 理论课模板不要求实践项
	if _, err := validateDimensionValues(db, map[string]interface{}{"attitude_1": 10.0}, model.TemplateTheory); err != nil {
		t.Fatalf("理论课作答应通过: %v", err)
	}
}

// TestUnGroupedDimensionBelongsToTheory 未分组维度按存量理论课维度处理。
func TestUnGroupedDimensionBelongsToTheory(t *testing.T) {
	db := templateTestDB(t)
	legacy := model.EvaluationDimension{
		Code: "legacy_required", Name: "存量必填项", FieldType: model.FieldScore,
		FieldConfig: []byte(`{"min_score":0,"max_score":5,"step":1}`), IsRequired: true, SortOrder: 1, Status: 1,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("灌入存量维度失败: %v", err)
	}
	if _, err := validateDimensionValues(db, map[string]interface{}{"attitude_1": 1.0}, model.TemplateTheory); err == nil {
		t.Fatal("理论课模板应要求存量未分组必填项")
	}
	if _, err := validateDimensionValues(db, map[string]interface{}{"prac_content_1": 1.0}, model.TemplatePractice); err != nil {
		t.Fatalf("实践教学模板不应要求存量未分组项: %v", err)
	}
}

// TestGroupTemplateValidation 分组模板白名单：仅允许 common/theory/practice，未传默认理论课。
func TestGroupTemplateValidation(t *testing.T) {
	db := templateTestDB(t)
	svc := NewDimension()
	if _, err := svc.GroupCreate(db, GroupParams{Code: "bad_group", Name: "非法模板分组", TemplateCode: "foo"}); err == nil {
		t.Fatal("非法 template_code 应被拒绝")
	}
	if _, err := svc.GroupCreate(db, GroupParams{Code: "common_group", Name: "通用分组", TemplateCode: model.TemplateCommon}); err != nil {
		t.Fatalf("common 分组应允许创建: %v", err)
	}
	g, err := svc.GroupCreate(db, GroupParams{Code: "default_group", Name: "默认模板分组"})
	if err != nil {
		t.Fatalf("未传模板应创建成功: %v", err)
	}
	if g.TemplateCode != model.TemplateTheory {
		t.Fatalf("未传模板应默认理论课, got %v", g.TemplateCode)
	}
	if _, err := svc.GroupUpdate(db, g.ID, GroupParams{TemplateCode: "foo"}); err == nil {
		t.Fatal("更新为非法 template_code 应被拒绝")
	}
}

// TestDimListTemplateScope 管理端维度列表的模板筛选与评教表单口径一致：
// 理论课含未分组存量维度，实践教学不含。
func TestDimListTemplateScope(t *testing.T) {
	db := templateTestDB(t)
	legacy := model.EvaluationDimension{
		Code: "legacy_orphan", Name: "存量未分组维度", FieldType: model.FieldText, Status: 1,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("灌入未分组维度失败: %v", err)
	}
	svc := NewDimension()
	codesOfList := func(template string) map[string]bool {
		list, _, err := svc.DimList(db, DimParams{Template: template}, 1, 100)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		m := map[string]bool{}
		for _, item := range list {
			m[item["code"].(string)] = true
		}
		return m
	}

	theory := codesOfList(model.TemplateTheory)
	if !theory["attitude_1"] || !theory["expected_count"] || !theory["legacy_orphan"] {
		t.Fatalf("理论课应含理论课/通用/未分组维度, got %v", theory)
	}
	if theory["prac_content_1"] {
		t.Fatal("理论课不应含实践教学维度")
	}

	practice := codesOfList(model.TemplatePractice)
	if !practice["prac_content_1"] || !practice["expected_count"] {
		t.Fatalf("实践教学应含实践/通用维度, got %v", practice)
	}
	if practice["attitude_1"] || practice["legacy_orphan"] {
		t.Fatalf("实践教学不应含理论课/未分组维度, got %v", practice)
	}
}
