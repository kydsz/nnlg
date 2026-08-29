package model

import "encoding/json"

// 评教任务状态
const (
	TaskStatusPending   int16 = 1 // 待评
	TaskStatusEvaluated int16 = 2 // 已评
	TaskStatusCancelled int16 = 3 // 取消
)

// TaskStatusNames 状态 -> 中文名
var TaskStatusNames = map[int16]string{
	TaskStatusPending:   "待评",
	TaskStatusEvaluated: "已评",
	TaskStatusCancelled: "取消",
}

// EvaluationTask 评教任务表
type EvaluationTask struct {
	Model
	TeacherID         int        `gorm:"column:teacher_id" json:"teacher_id"`
	TeacherName       string     `gorm:"column:teacher_name;size:64" json:"teacher_name"`
	CourseName        string     `gorm:"column:course_name;size:128" json:"course_name"`
	ClassTime         *LocalTime `gorm:"column:class_time" json:"class_time"`
	Classroom         *string    `gorm:"column:classroom;size:64" json:"classroom"`
	Status            int16      `gorm:"column:status" json:"status"`
	StartTime         *LocalTime `gorm:"column:start_time" json:"start_time"`
	EndTime           *LocalTime `gorm:"column:end_time" json:"end_time"`
	EvaluationCount   int        `gorm:"column:evaluation_count" json:"evaluation_count"`
	HasSupervisorEval bool       `gorm:"column:has_supervisor_eval" json:"has_supervisor_eval"`
	CreateBy          *int       `gorm:"column:create_by" json:"create_by"`
	IsDeleted         bool       `gorm:"column:is_deleted" json:"is_deleted"`
}

func (EvaluationTask) TableName() string { return "evaluation_task" }

// EvaluationRecord 评教记录表
type EvaluationRecord struct {
	Model
	TaskID          int             `gorm:"column:task_id" json:"task_id"`
	EvaluatorID     *int            `gorm:"column:evaluator_id" json:"evaluator_id"`
	EvaluatorName   string          `gorm:"column:evaluator_name;size:64" json:"evaluator_name"`
	EvaluatorRole   string          `gorm:"column:evaluator_role;size:20" json:"evaluator_role"`
	DimensionValues json.RawMessage `gorm:"column:dimension_values;type:json" json:"dimension_values"`
	SubmitTime      *LocalTime      `gorm:"column:submit_time" json:"submit_time"`
	IsAnonymous     bool            `gorm:"column:is_anonymous" json:"is_anonymous"`
}

func (EvaluationRecord) TableName() string { return "evaluation_record" }

// IsSupervisorRole 判断角色编码是否督导类
func IsSupervisorRole(code string) bool {
	return code == RoleSupervisor || code == RoleSchoolSupervisor || code == RoleCollegeSupervisor
}
