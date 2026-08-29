package model

import (
	"encoding/json"
	"testing"
)

func userIDPtr(i int) *int { return &i }

func TestRoleCodesFromUserRolesDedup(t *testing.T) {
	u := User{
		UserRoles: []UserRole{
			{Role: RoleSupervisor},
			{Role: RoleTeacher},
			{Role: RoleSupervisor}, // 重复应去重
		},
	}
	got := u.RoleCodes()
	if len(got) != 2 || got[0] != RoleSupervisor || got[1] != RoleTeacher {
		t.Fatalf("期望 [supervisor teacher], 实际 %v", got)
	}
}

func TestRoleCodesFallbackToLegacyField(t *testing.T) {
	// 无 user_role 关联时回退到旧 role 字段（向后兼容约定）
	u := User{Role: RoleTeacher}
	got := u.RoleCodes()
	if len(got) != 1 || got[0] != RoleTeacher {
		t.Fatalf("期望回退到旧字段, 实际 %v", got)
	}
	u2 := User{}
	if got := u2.RoleCodes(); got != nil {
		t.Fatalf("无任何角色应返回 nil, 实际 %v", got)
	}
}

func TestHasRole(t *testing.T) {
	u := User{Role: RoleTeacher, UserRoles: []UserRole{{Role: RoleSupervisor}}}
	if !u.HasRole(RoleSupervisor) {
		t.Fatal("user_role 关联角色应命中")
	}
	if !u.HasRole(RoleTeacher) {
		t.Fatal("旧 role 字段应兜底命中")
	}
	if u.HasRole(RoleSystemAdmin) {
		t.Fatal("不存在的角色不应命中")
	}
}

func TestHasAnyRole(t *testing.T) {
	u := User{Role: RoleTeacher}
	if !u.HasAnyRole(RoleSystemAdmin, RoleTeacher) {
		t.Fatal("满足任一角色应返回 true")
	}
	if u.HasAnyRole(RoleSystemAdmin, RoleCollegeAdmin) {
		t.Fatal("都不满足应返回 false")
	}
}

func TestHasCollegeID(t *testing.T) {
	u := User{
		CollegeID:    userIDPtr(1),
		UserColleges: []UserCollege{{CollegeID: 3}},
	}
	if !u.HasCollegeID(1) {
		t.Fatal("主学院应命中")
	}
	if !u.HasCollegeID(3) {
		t.Fatal("督导关联学院应命中")
	}
	if u.HasCollegeID(2) {
		t.Fatal("未关联学院不应命中")
	}
	u2 := User{}
	if u2.HasCollegeID(1) {
		t.Fatal("无任何学院关联不应命中")
	}
}

func TestCollegeInfosSkipsNilCollege(t *testing.T) {
	ucs := []UserCollege{
		{CollegeID: 1, JoinTime: nil}, // College 未 Preload → 应跳过
		{CollegeID: 2, College: &College{Model: Model{ID: 2}, Code: "CS", Name: "计算机学院"}},
	}
	infos := CollegeInfos(ucs)
	if len(infos) != 1 {
		t.Fatalf("应只返回已 Preload 的 1 条, 实际 %d", len(infos))
	}
	if infos[0].CollegeID != 2 || infos[0].CollegeCode != "CS" || infos[0].CollegeName != "计算机学院" {
		t.Fatalf("字段映射不符: %+v", infos[0])
	}
}

func TestRolePermissionList(t *testing.T) {
	r := Role{Permissions: json.RawMessage(`["evaluation:view_all","user:view"]`)}
	perms := r.PermissionList()
	if len(perms) != 2 || perms[0] != "evaluation:view_all" {
		t.Fatalf("权限解析不符: %v", perms)
	}
	// 非法 JSON / 空值 → nil（不 panic）
	bad := Role{Permissions: json.RawMessage(`{invalid`)}
	if got := bad.PermissionList(); got != nil {
		t.Fatalf("非法 JSON 应返回 nil, 实际 %v", got)
	}
	empty := Role{}
	if got := empty.PermissionList(); got != nil {
		t.Fatalf("空权限应返回 nil, 实际 %v", got)
	}
}

func TestRoleName(t *testing.T) {
	if RoleName(RoleSystemAdmin) != "系统管理员" {
		t.Fatal("已知角色应返回中文名")
	}
	if RoleName("unknown_role") != "unknown_role" {
		t.Fatal("未知角色应原样返回")
	}
}
