package model

import (
	"encoding/json"

	"gorm.io/gorm"
)

// User 用户表（表名 user 为 MySQL 保留字，gorm 会自动加反引号）
type User struct {
	Model
	UserNo             string     `gorm:"column:user_no;size:32" json:"user_no"`
	Username           string     `gorm:"column:username;size:64" json:"username"`
	Password           string     `gorm:"column:password;size:255" json:"-"`
	Role               string     `gorm:"column:role;size:20" json:"role"`
	CollegeID          *int       `gorm:"column:college_id" json:"college_id"`
	ResearchRoomID     *int       `gorm:"column:research_room_id" json:"research_room_id"`
	Status             int16      `gorm:"column:status" json:"status"` // 1-启用 0-禁用
	MustChangePassword bool       `gorm:"column:must_change_password" json:"must_change_password"`
	LastLoginTime      *LocalTime `gorm:"column:last_login_time" json:"last_login_time"`

	UserRoles       []UserRole       `gorm:"foreignKey:UserID" json:"-"`
	UserColleges    []UserCollege    `gorm:"foreignKey:UserID" json:"-"`
	UserRooms       []UserRoom       `gorm:"foreignKey:UserID" json:"-"`
	College         *College         `gorm:"foreignKey:CollegeID" json:"-"`
	ResearchRoom    *ResearchRoom    `gorm:"foreignKey:ResearchRoomID" json:"-"`
}

func (User) TableName() string { return "user" }

// RoleCodes 全部角色编码：user_roles 关联 + 旧 role 字段兜底
func (u *User) RoleCodes() []string {
	seen := map[string]bool{}
	var codes []string
	for _, ur := range u.UserRoles {
		if !seen[ur.Role] {
			seen[ur.Role] = true
			codes = append(codes, ur.Role)
		}
	}
	if len(codes) == 0 && u.Role != "" {
		codes = append(codes, u.Role)
	}
	return codes
}

func (u *User) HasRole(code string) bool {
	for _, ur := range u.UserRoles {
		if ur.Role == code {
			return true
		}
	}
	return u.Role == code
}

// HasAnyRole 满足任一角色即可
func (u *User) HasAnyRole(codes ...string) bool {
	for _, c := range codes {
		if u.HasRole(c) {
			return true
		}
	}
	return false
}

// Permissions 用户全部权限（角色表 permissions 字段的并集）
func (u *User) Permissions(db *gorm.DB) []string {
	codes := u.RoleCodes()
	if len(codes) == 0 {
		return nil
	}
	var roles []Role
	if err := db.Where("code IN ? AND status = 1", codes).Find(&roles).Error; err != nil {
		return nil
	}
	seen := map[string]bool{}
	var perms []string
	for _, r := range roles {
		for _, p := range r.PermissionList() {
			if !seen[p] {
				seen[p] = true
				perms = append(perms, p)
			}
		}
	}
	return perms
}

// UserRole 用户-角色关联表
type UserRole struct {
	Model
	UserID     int        `gorm:"column:user_id" json:"user_id"`
	Role       string     `gorm:"column:role;size:20" json:"role"`
	AssignTime *LocalTime `gorm:"column:assign_time" json:"assign_time"`
}

func (UserRole) TableName() string { return "user_role" }

// UserCollege 用户-学院关联表（督导负责的学院）
type UserCollege struct {
	Model
	UserID    int        `gorm:"column:user_id" json:"user_id"`
	CollegeID int        `gorm:"column:college_id" json:"college_id"`
	JoinTime  *LocalTime `gorm:"column:join_time" json:"join_time"`
	College   *College   `gorm:"foreignKey:CollegeID" json:"college,omitempty"`
}

func (UserCollege) TableName() string { return "user_college" }

// UserRoom 用户-教研室关联表（user_research_room）
type UserRoom struct {
	Model
	UserID         int           `gorm:"column:user_id" json:"user_id"`
	ResearchRoomID int           `gorm:"column:research_room_id" json:"research_room_id"`
	JoinTime       *LocalTime    `gorm:"column:join_time" json:"join_time"`
	ResearchRoom   *ResearchRoom `gorm:"foreignKey:ResearchRoomID" json:"research_room,omitempty"`
}

func (UserRoom) TableName() string { return "user_research_room" }

// ResearchRoom 教研室表（框架示例只声明常用字段）
type ResearchRoom struct {
	Model
	Code      string   `gorm:"column:code;size:32" json:"code"`
	Name      string   `gorm:"column:name;size:64" json:"name"`
	CollegeID int      `gorm:"column:college_id" json:"college_id"`
	Status    int16    `gorm:"column:status" json:"status"`
	College   *College `gorm:"foreignKey:CollegeID" json:"-"`
}

func (ResearchRoom) TableName() string { return "research_room" }

// UserCollegeInfo 用户关联学院信息（接口返回用）
type UserCollegeInfo struct {
	CollegeID   int        `json:"college_id"`
	CollegeName string     `json:"college_name"`
	CollegeCode string     `json:"college_code"`
	JoinTime    *LocalTime `json:"join_time"`
}

// CollegeInfos 导出 UserCollege 列表为接口信息
func CollegeInfos(ucs []UserCollege) []UserCollegeInfo {
	var list []UserCollegeInfo
	for _, uc := range ucs {
		if uc.College == nil {
			continue
		}
		list = append(list, UserCollegeInfo{
			CollegeID:   uc.CollegeID,
			CollegeName: uc.College.Name,
			CollegeCode: uc.College.Code,
			JoinTime:    uc.JoinTime,
		})
	}
	return list
}

// UserRoomBriefInfo 用户关联教研室简要信息（督导范围接口用）
type UserRoomBriefInfo struct {
	RoomID    int    `json:"research_room_id"`
	RoomName  string `json:"research_room_name"`
	CollegeID int    `json:"college_id"`
}

// UserResearchRoomInfo 用户关联教研室完整信息（对齐旧端 get_user_research_rooms）
type UserResearchRoomInfo struct {
	RoomID    *int       `json:"research_room_id"`
	RoomName  string     `json:"research_room_name"`
	CollegeID *int       `json:"college_id"`
	JoinTime  *LocalTime `json:"join_time"`
}

// HasCollegeID 用户是否关联某学院（主学院或督导学院）
func (u *User) HasCollegeID(collegeID int) bool {
	if u.CollegeID != nil && *u.CollegeID == collegeID {
		return true
	}
	for _, uc := range u.UserColleges {
		if uc.CollegeID == collegeID {
			return true
		}
	}
	return false
}

// PermissionJSON 权限 JSON 列解析辅助
func parseJSONSlice(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil
	}
	return list
}
