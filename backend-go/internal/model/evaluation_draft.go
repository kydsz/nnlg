package model

import "encoding/json"

// EvaluationDraft 评教草稿表（暂存：同一任务同一评教人仅一条）
type EvaluationDraft struct {
	Model
	TaskID          int             `gorm:"column:task_id;uniqueIndex:uk_task_evaluator" json:"task_id"`
	EvaluatorID     int             `gorm:"column:evaluator_id;uniqueIndex:uk_task_evaluator" json:"evaluator_id"`
	EvaluatorName   string          `gorm:"column:evaluator_name;size:64" json:"evaluator_name"`
	DimensionValues json.RawMessage `gorm:"column:dimension_values;type:json" json:"dimension_values"`
	IsAnonymous     bool            `gorm:"column:is_anonymous" json:"is_anonymous"`
}

func (EvaluationDraft) TableName() string { return "evaluation_draft" }