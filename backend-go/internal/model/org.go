package model

// Campus 校区表
type Campus struct {
	Model
	Name      string `gorm:"column:name;size:64" json:"name"`
	SortOrder int    `gorm:"column:sort_order" json:"sort_order"`
	Status    int16  `gorm:"column:status" json:"status"`
}

func (Campus) TableName() string { return "campus" }
