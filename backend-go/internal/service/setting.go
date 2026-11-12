package service

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

const (
	// TaskMaxParticipantsKey 单个评教任务最多参与人数的系统配置键
	TaskMaxParticipantsKey = "task_max_participants"
	// DefaultTaskMaxParticipants 默认上限（含创建人）
	DefaultTaskMaxParticipants = 4
	// MaxTaskParticipantsLimit 配置上限的硬上界，避免误配置成过大值
	MaxTaskParticipantsLimit = 100
)

// TaskMaxParticipants 读取任务人数上限：缺失/非法/小于 1 时回退默认值，超过硬上界截断。
func TaskMaxParticipants(db *gorm.DB) int {
	var s model.SystemSetting
	if err := db.Where("setting_key = ?", TaskMaxParticipantsKey).First(&s).Error; err != nil {
		return DefaultTaskMaxParticipants
	}
	n, err := strconv.Atoi(strings.TrimSpace(s.Value))
	if err != nil || n < 1 {
		return DefaultTaskMaxParticipants
	}
	if n > MaxTaskParticipantsLimit {
		return MaxTaskParticipantsLimit
	}
	return n
}

// SetTaskMaxParticipants 保存任务人数上限（1-100），记录最后修改人。
func SetTaskMaxParticipants(db *gorm.DB, userID, n int) error {
	if n < 1 || n > MaxTaskParticipantsLimit {
		return errors.New("参与人数上限必须在 1-100 之间")
	}
	now := model.LocalTimePtr(time.Now())
	setting := model.SystemSetting{
		Key:        TaskMaxParticipantsKey,
		Value:      strconv.Itoa(n),
		UpdateBy:   &userID,
		UpdateTime: now,
	}
	var existing model.SystemSetting
	if err := db.Where("setting_key = ?", TaskMaxParticipantsKey).First(&existing).Error; err == nil {
		setting.CreateTime = existing.CreateTime
		return db.Save(&setting).Error
	}
	setting.CreateTime = now
	return db.Create(&setting).Error
}

// TaskSettings 当前任务相关配置（管理端设置页读取）。
func TaskSettings(db *gorm.DB) map[string]int {
	return map[string]int{TaskMaxParticipantsKey: TaskMaxParticipants(db)}
}
