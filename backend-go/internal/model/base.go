package model

// Model 公共字段，与现有表结构一致（id/create_time/update_time）
type Model struct {
	ID         int        `gorm:"primaryKey;autoIncrement" json:"id"`
	CreateTime *LocalTime `gorm:"column:create_time" json:"create_time"`
	UpdateTime *LocalTime `gorm:"column:update_time" json:"update_time"`
}
