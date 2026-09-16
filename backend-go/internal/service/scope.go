package service

import (
	"errors"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// HTTPError 带 HTTP 状态码的业务错误（用于对齐旧端 400/403/404 语义）
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

func errForbidden(msg string) error { return &HTTPError{Status: 403, Message: msg} }
func errNotFound(msg string) error  { return &HTTPError{Status: 404, Message: msg} }

// AsHTTPError 提取 HTTP 状态码；非 HTTPError 返回 (400, false)
func AsHTTPError(err error) (int, bool) {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status, true
	}
	return 400, false
}

// IsAllScope 是否全校数据范围（system_admin / school_supervisor）
func IsAllScope(u *model.User) bool {
	return u.HasAnyRole(model.RoleSystemAdmin, model.RoleSchoolSupervisor)
}

// IsAdminRole 是否管理类角色（system_admin / college_admin / school_admin）
func IsAdminRole(u *model.User) bool {
	return u.HasAnyRole(model.RoleSystemAdmin, model.RoleCollegeAdmin, model.RoleSchoolAdmin)
}

// IsSupervisor 是否督导类角色
func IsSupervisor(u *model.User) bool {
	return u.HasAnyRole(model.RoleSupervisor, model.RoleSchoolSupervisor, model.RoleCollegeSupervisor)
}

// AccessibleCollegeIDs 用户可访问的学院 ID 集合（主学院 + 督导负责学院）；nil 表示全校
// 对齐旧端：college_admin/school_admin 在无任何学院时也视为全校
func AccessibleCollegeIDs(u *model.User) []int {
	if IsAllScope(u) {
		return nil
	}
	seen := map[int]bool{}
	var ids []int
	if u.CollegeID != nil {
		seen[*u.CollegeID] = true
		ids = append(ids, *u.CollegeID)
	}
	for _, uc := range u.UserColleges {
		if !seen[uc.CollegeID] {
			seen[uc.CollegeID] = true
			ids = append(ids, uc.CollegeID)
		}
	}
	if len(ids) == 0 && u.HasAnyRole(model.RoleCollegeAdmin, model.RoleSchoolAdmin) {
		return nil
	}
	return ids
}

// CollegeInScope 判断学院是否在用户可访问范围内
func CollegeInScope(u *model.User, collegeID *int) bool {
	ids := AccessibleCollegeIDs(u)
	if ids == nil {
		return true
	}
	if collegeID == nil {
		return false
	}
	for _, id := range ids {
		if id == *collegeID {
			return true
		}
	}
	return false
}

// CanViewOthersEvaluation 是否可查看他人评教记录/详情
// 系统管理员恒可；其他角色需被分配 evaluation:view_all 权限（系统管理员可在角色管理中勾选分配）
func CanViewOthersEvaluation(db *gorm.DB, u *model.User) bool {
	if u.HasRole(model.RoleSystemAdmin) {
		return true
	}
	for _, p := range u.Permissions(db) {
		if p == "evaluation:view_all" {
			return true
		}
	}
	return false
}

// CanViewAllTasks 是否可查看「与我无关」的评教任务（task:view_all）。
//
// 系统管理员恒可；其他角色需被分配 task:view_all 权限（管理员可在「角色管理」页勾选分配）。
// 未获授权者只能看到与我相关的任务：我创建的、我被评的、我评过的（见 relatedToMeCond）。
// 注意：授权只是取消「仅本人相关」这一层收敛，可见的他人任务仍受学院数据范围
// （AccessibleCollegeIDs + applyCollegeFilter）约束，两者是「且」的关系。
func CanViewAllTasks(db *gorm.DB, u *model.User) bool {
	if u.HasRole(model.RoleSystemAdmin) {
		return true
	}
	for _, p := range u.Permissions(db) {
		if p == "task:view_all" {
			return true
		}
	}
	return false
}

// CanViewTask 是否可查看单个评教任务（详情接口用）。
// 与列表的可见范围同一口径，两层是「且」的关系：
//  1. 学院数据范围——被评教师须在 caller 的可访问学院内（与 applyCollegeFilter 等价，
//     含「教师未绑定学院则不在任何人的范围内」这一点）；
//  2. 是否与我相关——我创建的、我被评的、我评过的；否则需要 task:view_all（见 CanViewAllTasks）。
func CanViewTask(db *gorm.DB, caller *model.User, t *model.EvaluationTask) bool {
	teacher, err := LoadTeacherUser(db, t.TeacherID)
	if err != nil {
		return false
	}
	if !CollegeInScope(caller, teacher.CollegeID) {
		return false
	}
	if t.CreateBy != nil && *t.CreateBy == caller.ID {
		return true
	}
	if t.TeacherID == caller.ID {
		return true
	}
	if CanViewAllTasks(db, caller) {
		return true
	}
	var n int64
	db.Model(&model.EvaluationRecord{}).
		Where("task_id = ? AND evaluator_id = ? AND is_deleted = 0", t.ID, caller.ID).
		Count(&n)
	return n > 0
}

// CanDeleteEvaluation 是否可删除任意评教记录
// 系统管理员恒可；其他角色需被分配 evaluation:delete 权限（评教人本人不再默认可删，避免抹掉已提交评教影响接收人数据）
func CanDeleteEvaluation(db *gorm.DB, u *model.User) bool {
	if u.HasRole(model.RoleSystemAdmin) {
		return true
	}
	for _, p := range u.Permissions(db) {
		if p == "evaluation:delete" {
			return true
		}
	}
	return false
}

// CanDeleteOwnEvaluation 是否可删除自己提交的评教记录（evaluation:delete_own）
func CanDeleteOwnEvaluation(db *gorm.DB, u *model.User) bool {
	for _, p := range u.Permissions(db) {
		if p == "evaluation:delete_own" {
			return true
		}
	}
	return false
}

// CanDeleteTask 是否可删除任意评教任务
// 系统管理员恒可；其他角色需被分配 task:delete 权限
func CanDeleteTask(db *gorm.DB, u *model.User) bool {
	if u.HasRole(model.RoleSystemAdmin) {
		return true
	}
	for _, p := range u.Permissions(db) {
		if p == "task:delete" {
			return true
		}
	}
	return false
}

// CanDeleteOwnTask 是否可删除自己创建的评教任务（task:delete_own）
func CanDeleteOwnTask(db *gorm.DB, u *model.User) bool {
	for _, p := range u.Permissions(db) {
		if p == "task:delete_own" {
			return true
		}
	}
	return false
}


