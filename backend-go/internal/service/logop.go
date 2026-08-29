package service

import (
	"encoding/json"
	"log"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// LogRecord 写一条操作日志（失败仅打印，不影响主流程）
func LogRecord(db *gorm.DB, userID *int, userName, opType, module string, targetID *int, targetType string, content interface{}) {
	var raw json.RawMessage
	if content != nil {
		b, err := json.Marshal(content)
		if err == nil {
			raw = b
		}
	}
	// CreateTime/UpdateTime 显式写入，否则 GORM 会向列写 NULL（绕过 MySQL server_default）
	now := model.LocalTimePtr(time.Now())
	if err := db.Create(&model.OperationLog{
			UserID:        userID,
			UserName:      userName,
			OperationType: opType,
			Module:        module,
			TargetID:      targetID,
			TargetType:    targetType,
			Content:       raw,
			Model: model.Model{
				CreateTime: now,
				UpdateTime: now,
			},
		}).Error; err != nil {
		log.Printf("写操作日志失败: %v", err)
	}
}

// IPtr int 指针辅助
func IPtr(i int) *int { return &i }
