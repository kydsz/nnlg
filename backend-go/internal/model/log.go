package model

import "encoding/json"

// OperationLog 操作日志表
type OperationLog struct {
	Model
	UserID        *int            `gorm:"column:user_id" json:"user_id"`
	UserName      string          `gorm:"column:user_name;size:64" json:"user_name"`
	OperationType string          `gorm:"column:operation_type;size:32" json:"operation_type"`
	Module        string          `gorm:"column:module;size:32" json:"module"`
	TargetID      *int            `gorm:"column:target_id" json:"target_id"`
	TargetType    string          `gorm:"column:target_type;size:32" json:"target_type"`
	Content       json.RawMessage `gorm:"column:content;type:json" json:"content"`
	IPAddress     string          `gorm:"column:ip_address;size:45" json:"ip_address"`
	UserAgent     string          `gorm:"column:user_agent;size:512" json:"user_agent"`
}

func (OperationLog) TableName() string { return "operation_log" }
