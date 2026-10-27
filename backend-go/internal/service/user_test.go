package service

import (
	"testing"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// userListTestDB 复用了 teacherScopeTestDB 的用户/交叉记录样例，
// 并补充学院记录供 User.List 预加载 UserColleges.College 使用：
// 学院 5=文理学院，6=马克思主义学院。
func userListTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := teacherScopeTestDB(t)
	colleges := []model.College{
		{Model: model.Model{ID: 5}, Code: "W", Name: "文理学院", Status: 1},
		{Model: model.Model{ID: 6}, Code: "M", Name: "马克思主义学院", Status: 1},
	}
	if err := db.Create(&colleges).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	return db
}

// userListCallers 三类调用者：系统管理员 / 文理学院管理员 / 未绑定学院的学院管理员。
// 只用于判定角色与主学院，不需要落库。
func userListCallers() (systemAdmin, wlAdmin, noCollegeAdmin *model.User) {
	return &model.User{Model: model.Model{ID: 9001}, Role: model.RoleSystemAdmin},
		&model.User{Model: model.Model{ID: 9002}, Role: model.RoleCollegeAdmin, CollegeID: intPtr(5)},
		&model.User{Model: model.Model{ID: 9003}, Role: model.RoleCollegeAdmin}
}

func TestUserListCollegeFilter(t *testing.T) {
	db := userListTestDB(t)
	svc := &User{}
	_, wlAdmin, _ := userListCallers()

	t.Run("按学院筛选只匹配主学院，忽略 user_college 交叉记录", func(t *testing.T) {
		users, total, err := svc.List(db, wlAdmin, UserParams{Page: 1, PageSize: 50, CollegeID: "5"})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if total != 1 || len(got) != 1 || got[0] != "文理教师" {
			t.Fatalf("college_id=5 应只返回文理教师, 得到 total=%d %v", total, got)
		}
	})
}

func TestUserListScope(t *testing.T) {
	db := userListTestDB(t)
	svc := &User{}
	systemAdmin, wlAdmin, noCollegeAdmin := userListCallers()

	t.Run("学院管理员默认只返回本学院用户", func(t *testing.T) {
		users, total, err := svc.List(db, wlAdmin, UserParams{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if total != 1 || len(got) != 1 || got[0] != "文理教师" {
			t.Fatalf("学院管理员应只见本学院用户, 得到 total=%d %v", total, got)
		}
	})

	t.Run("学院管理员传外院筛选得到空列表", func(t *testing.T) {
		users, total, err := svc.List(db, wlAdmin, UserParams{Page: 1, PageSize: 50, CollegeID: "6"})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 0 || len(users) != 0 {
			t.Fatalf("外院筛选应为空, 得到 total=%d %v", total, teacherIDsOf(t, users))
		}
	})

	t.Run("学院管理员多学院筛选只能收窄到本学院", func(t *testing.T) {
		users, total, err := svc.List(db, wlAdmin, UserParams{Page: 1, PageSize: 50, CollegeID: "5,6"})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if total != 1 || len(got) != 1 || got[0] != "文理教师" {
			t.Fatalf("多学院筛选应收窄为本学院, 得到 total=%d %v", total, got)
		}
	})

	t.Run("系统管理员返回全部用户", func(t *testing.T) {
		_, total, err := svc.List(db, systemAdmin, UserParams{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 5 {
			t.Fatalf("系统管理员应见全部 5 个用户, 得到 %d", total)
		}
	})

	t.Run("未绑定学院的学院管理员 fail-closed 返回空", func(t *testing.T) {
		_, total, err := svc.List(db, noCollegeAdmin, UserParams{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 0 {
			t.Fatalf("无学院的管理员不应看到任何用户, 得到 %d", total)
		}
	})

	t.Run("督导等非管理员角色没有用户管理范围", func(t *testing.T) {
		supervisor := &model.User{Model: model.Model{ID: 9004}, Role: model.RoleSchoolSupervisor}
		_, total, err := svc.List(db, supervisor, UserParams{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if total != 0 {
			t.Fatalf("督导不应看到用户列表, 得到 %d", total)
		}
	})
}
