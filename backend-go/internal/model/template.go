package model

// 评价模板编码
const (
	TemplateTheory   = "theory"   // 理论课听评课表
	TemplatePractice = "practice" // 实践教学评价表
	TemplateCommon   = "common"   // 通用分组（所有模板共享：考勤/评语等）
)

// NormalizeTemplate 空值/未知模板一律回落到理论课，保证存量任务口径不变。
func NormalizeTemplate(code string) string {
	switch code {
	case TemplatePractice:
		return TemplatePractice
	case TemplateTheory:
		return TemplateTheory
	default:
		return TemplateTheory
	}
}

// ValidGroupTemplate 分组可选模板：common 通用 / theory 理论课 / practice 实践教学。
// 分组不接受空值，由调用方决定默认值。
func ValidGroupTemplate(code string) bool {
	switch code {
	case TemplateCommon, TemplateTheory, TemplatePractice:
		return true
	default:
		return false
	}
}

// TemplateName 模板展示名（未知/空值按理论课，与 NormalizeTemplate 口径一致）。
func TemplateName(code string) string {
	if NormalizeTemplate(code) == TemplatePractice {
		return "实践教学"
	}
	return "理论课"
}
