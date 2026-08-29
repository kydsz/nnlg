package model

import "encoding/json"

// DimensionGroup 维度分组表
type DimensionGroup struct {
	Model
	Code       string                `gorm:"column:code;size:32" json:"code"`
	Name       string                `gorm:"column:name;size:64" json:"name"`
	SortOrder  int                   `gorm:"column:sort_order" json:"sort_order"`
	Status     int16                 `gorm:"column:status" json:"status"`
	Dimensions []EvaluationDimension `gorm:"foreignKey:GroupID" json:"-"`
}

func (DimensionGroup) TableName() string { return "dimension_group" }

// EvaluationDimension 评教维度表
type EvaluationDimension struct {
	Model
	GroupID     *int            `gorm:"column:group_id" json:"group_id"`
	Code        string          `gorm:"column:code;size:32" json:"code"`
	Name        string          `gorm:"column:name;size:64" json:"name"`
	FieldType   string          `gorm:"column:field_type;size:32" json:"field_type"`
	FieldConfig json.RawMessage `gorm:"column:field_config;type:json" json:"field_config"`
	Description *string         `gorm:"column:description;type:text" json:"description"`
	SortOrder   int             `gorm:"column:sort_order" json:"sort_order"`
	IsRequired  bool            `gorm:"column:is_required" json:"is_required"`
	Status      int16           `gorm:"column:status" json:"status"`
	Group       *DimensionGroup `gorm:"foreignKey:GroupID" json:"-"`
}

func (EvaluationDimension) TableName() string { return "evaluation_dimension" }

// 维度字段类型枚举
const (
	FieldScore         = "score"
	FieldSingleChoice  = "single_choice"
	FieldMultipleChoce = "multiple_choice"
	FieldText          = "text"
	FieldNumber        = "number"
	FieldDate          = "date"
	FieldDatetime      = "datetime"
	FieldRichText      = "rich_text"
	FieldImage         = "image"
	FieldFile          = "file"
)

// ValidFieldType 校验字段类型合法性
func ValidFieldType(t string) bool {
	switch t {
	case FieldScore, FieldSingleChoice, FieldMultipleChoce, FieldText, FieldNumber,
		FieldDate, FieldDatetime, FieldRichText, FieldImage, FieldFile:
		return true
	}
	return false
}
