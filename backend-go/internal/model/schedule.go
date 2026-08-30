package model

// SemesterConfig 学期配置表
type SemesterConfig struct {
	Model
	Semester  string    `gorm:"column:semester;size:32" json:"semester"`
	StartDate LocalDate `gorm:"column:start_date;type:date" json:"start_date"`
	Weeks     int       `gorm:"column:weeks;default:20" json:"weeks"`
	IsCurrent bool      `gorm:"column:is_current" json:"is_current"`
}

func (SemesterConfig) TableName() string { return "semester_config" }

// CourseSchedule 课表主表
type CourseSchedule struct {
	Model
	TeacherID     *int                   `gorm:"column:teacher_id" json:"teacher_id"`
	TeacherName   string                 `gorm:"column:teacher_name;size:64" json:"teacher_name"`
	Semester      string                 `gorm:"column:semester;size:32" json:"semester"`
	Version       int                    `gorm:"column:version" json:"version"`
	IsCurrent     bool                   `gorm:"column:is_current" json:"is_current"`
	CrawlTime     *LocalTime             `gorm:"column:crawl_time" json:"crawl_time"`
	ChangeSummary *string                `gorm:"column:change_summary" json:"change_summary"`
	Details       []CourseScheduleDetail `gorm:"foreignKey:ScheduleID" json:"-"`
}

func (CourseSchedule) TableName() string { return "course_schedule" }

// CourseScheduleDetail 课表明细表
type CourseScheduleDetail struct {
	Model
	ScheduleID  int             `gorm:"column:schedule_id" json:"schedule_id"`
	CourseName  string          `gorm:"column:course_name;size:128" json:"course_name"`
	ClassInfo   string          `gorm:"column:class_info;size:256" json:"class_info"`
	WeekPattern string          `gorm:"column:week_pattern;size:64" json:"week_pattern"`
	WeekDay     *int8           `gorm:"column:week_day" json:"week_day"`
	Section     string          `gorm:"column:section;size:16" json:"section"`
	Classroom   string   `gorm:"column:classroom;size:64" json:"classroom"`
	RawData     *string  `gorm:"column:raw_data" json:"raw_data"`
}

func (CourseScheduleDetail) TableName() string { return "course_schedule_detail" }

// CourseScheduleVersion 课表版本表
type CourseScheduleVersion struct {
	Model
	Semester      string          `gorm:"column:semester;size:32" json:"semester"`
	Version       int             `gorm:"column:version" json:"version"`
	CrawlTime     *LocalTime      `gorm:"column:crawl_time" json:"crawl_time"`
	TeacherCount  int        `gorm:"column:teacher_count" json:"teacher_count"`
	CourseCount   int        `gorm:"column:course_count" json:"course_count"`
	ChangeSummary *string    `gorm:"column:change_summary" json:"change_summary"`
}

func (CourseScheduleVersion) TableName() string { return "course_schedule_version" }
