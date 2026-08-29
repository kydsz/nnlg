package model

import "encoding/json"

// Role 角色表
type Role struct {
	Model
	Name        string          `gorm:"column:name;size:64" json:"name"`
	Code        string          `gorm:"column:code;size:32" json:"code"`
	Description string          `gorm:"column:description;size:255" json:"description"`
	Level       int             `gorm:"column:level" json:"level"`
	Permissions json.RawMessage `gorm:"column:permissions;type:json" json:"permissions"`
	DataScope   string          `gorm:"column:data_scope;size:20" json:"data_scope"`
	IsSystem    bool            `gorm:"column:is_system" json:"is_system"`
	Status      int16           `gorm:"column:status" json:"status"`
}

func (Role) TableName() string { return "role" }

func (r *Role) PermissionList() []string { return parseJSONSlice(r.Permissions) }

// College 学院表
type College struct {
	Model
	Code      string `gorm:"column:code;size:32" json:"code"`
	Name      string `gorm:"column:name;size:64" json:"name"`
	CampusID  *int   `gorm:"column:campus_id" json:"campus_id"`
	SortOrder int16  `gorm:"column:sort_order" json:"sort_order"`
	Status    int16  `gorm:"column:status" json:"status"`
}

func (College) TableName() string { return "college" }

// 角色编码常量
const (
	RoleSystemAdmin       = "system_admin"
	RoleCollegeAdmin      = "college_admin"
	RoleSchoolAdmin       = "school_admin" // 向后兼容
	RoleSupervisor        = "supervisor"
	RoleSchoolSupervisor  = "school_supervisor"
	RoleCollegeSupervisor = "college_supervisor"
	RoleTeacher           = "teacher"
)

// RoleNames 角色编码 -> 中文名
var RoleNames = map[string]string{
	RoleSystemAdmin:       "系统管理员",
	RoleCollegeAdmin:      "学院管理员",
	RoleSchoolAdmin:       "学院管理员",
	RoleSupervisor:        "督导老师",
	RoleSchoolSupervisor:  "校级督导",
	RoleCollegeSupervisor: "院级督导",
	RoleTeacher:           "教师",
}

// RoleName 取角色中文名，未知编码原样返回
func RoleName(code string) string {
	if n, ok := RoleNames[code]; ok {
		return n
	}
	return code
}

// UserRoleInfo 用户角色信息（接口返回用）
type UserRoleInfo struct {
	Role       string     `json:"role"`
	RoleName   string     `json:"role_name"`
	AssignTime *LocalTime `json:"assign_time"`
}
