package model

// SystemSetting 系统配置表（键值对，当前仅评教任务人数上限）
type SystemSetting struct {
	Key         string     `gorm:"column:setting_key;primaryKey;size:64" json:"key"`
	Value       string     `gorm:"column:setting_value;size:255" json:"value"`
	Description *string    `gorm:"column:description;size:255" json:"description"`
	UpdateBy    *int       `gorm:"column:update_by" json:"update_by"`
	CreateTime  *LocalTime `gorm:"column:create_time" json:"create_time"`
	UpdateTime  *LocalTime `gorm:"column:update_time" json:"update_time"`
}

func (SystemSetting) TableName() string { return "system_setting" }
