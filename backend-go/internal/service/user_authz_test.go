package service

import (
	"testing"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// userAuthzTestDB 复用 teacherScopeTestDB（文理学院5/马院6/文理教研室1），
// 补充学院实体与 college_admin（文理学院）、system_admin 两个操作者，及马院教研室。
func userAuthzTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := teacherScopeTestDB(t)
	colleges := []model.College{
		{Model: model.Model{ID: 5}, Code: "W", Name: "文理学院", Status: 1},
		{Model: model.Model{ID: 6}, Code: "M", Name: "马克思主义学院", Status: 1},
	}
	if err := db.Create(&colleges).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	users := []model.User{
		{UserNo: "A001", Username: "文理管理员", Role: model.RoleCollegeAdmin, CollegeID: intPtr(5), Status: 1},
		{UserNo: "S001", Username: "系统管理员", Role: model.RoleSystemAdmin, Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("灌入操作者失败: %v", err)
	}
	// 马院教研室（学院6），用于"目标单位越权"用例
	if err := db.Create(&model.ResearchRoom{Code: "R2", Name: "马院教研室", CollegeID: 6, Status: 1}).Error; err != nil {
		t.Fatalf("灌入马院教研室失败: %v", err)
	}
	return db
}

// authzCaller 按 Username 取操作者
func authzCaller(t *testing.T, db *gorm.DB, username string) *model.User {
	t.Helper()
	u, err := (&User{}).GetByID(db, authzID(t, db, username))
	if err != nil {
		t.Fatalf("取操作者 %s 失败: %v", username, err)
	}
	return u
}

func authzID(t *testing.T, db *gorm.DB, username string) int {
	t.Helper()
	var u model.User
	if err := db.Where("username = ?", username).First(&u).Error; err != nil {
		t.Fatalf("查 %s 失败: %v", username, err)
	}
	return u.ID
}

func pwdParam(s string) *string { return &s }

func TestUserAuthzUpdate(t *testing.T) {
	db := userAuthzTestDB(t)
	svc := &User{}

	t.Run("学院管理员修改系统管理员密码被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		adminID := authzID(t, db, "系统管理员")
		_, err := svc.Update(db, caller, adminID, UpdateUserParams{Password: pwdParam("Hacked@123")})
		if err == nil {
			t.Fatalf("学院管理员修改系统管理员密码应被拒绝")
		}
	})

	t.Run("学院管理员修改本学院用户成功", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		u, err := svc.Update(db, caller, authzID(t, db, "文理教师"), UpdateUserParams{Password: pwdParam("NewPass@1")})
		if err != nil {
			t.Fatalf("本学院用户修改应成功: %v", err)
		}
		if u.UserNo != "T001" {
			t.Fatalf("返回用户错误: %+v", u)
		}
	})

	t.Run("学院管理员修改跨学院用户被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		_, err := svc.Update(db, caller, authzID(t, db, "马院教师B"), UpdateUserParams{Password: pwdParam("X")})
		if err == nil {
			t.Fatalf("跨学院修改应被拒绝")
		}
	})

	t.Run("学院管理员把用户主学院改到范围外被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		mayuan := 6
		_, err := svc.Update(db, caller, authzID(t, db, "文理教师"), UpdateUserParams{CollegeID: &mayuan})
		if err == nil {
			t.Fatalf("把用户主学院改到范围外应被拒绝")
		}
	})

	t.Run("系统管理员操作不受限", func(t *testing.T) {
		caller := authzCaller(t, db, "系统管理员")
		_, err := svc.Update(db, caller, authzID(t, db, "马院教师A"), UpdateUserParams{Password: pwdParam("Sys@12345")})
		if err != nil {
			t.Fatalf("系统管理员修改任意用户应成功: %v", err)
		}
	})
}

func TestUserAuthzDelete(t *testing.T) {
	db := userAuthzTestDB(t)
	svc := &User{}

	t.Run("学院管理员删除系统管理员被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		err := svc.Delete(db, caller, authzID(t, db, "系统管理员"))
		if err == nil {
			t.Fatalf("学院管理员删除系统管理员应被拒绝")
		}
	})

	t.Run("学院管理员删除跨学院用户被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		err := svc.Delete(db, caller, authzID(t, db, "马院教师B"))
		if err == nil {
			t.Fatalf("跨学院删除应被拒绝")
		}
	})

	t.Run("学院管理员删除本学院用户成功", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		if err := svc.Delete(db, caller, authzID(t, db, "文理教师")); err != nil {
			t.Fatalf("本学院删除应成功: %v", err)
		}
	})

	t.Run("系统管理员删除跨学院用户成功", func(t *testing.T) {
		caller := authzCaller(t, db, "系统管理员")
		if err := svc.Delete(db, caller, authzID(t, db, "马院教师B")); err != nil {
			t.Fatalf("系统管理员删除应成功: %v", err)
		}
	})
}

func TestUserAuthzRoom(t *testing.T) {
	db := userAuthzTestDB(t)
	svc := &User{}

	t.Run("目标用户不在范围时加入教研室被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		// 教研室1 属文理学院（在范围），但目标用户主学院在马院（不在范围）
		err := svc.AddRoom(db, caller, authzID(t, db, "马院教师B"), 1)
		if err == nil {
			t.Fatalf("目标用户不在管理范围时应被拒绝")
		}
	})

	t.Run("教研室不在范围时加入被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		// 马院教研室(学院6) 不在范围；目标文理教师在校内
		err := svc.AddRoom(db, caller, authzID(t, db, "文理教师"), roomID(t, db, "马院教研室"))
		if err == nil {
			t.Fatalf("教研室不在管理范围时应被拒绝")
		}
	})

	t.Run("范围内目标与教研室可加入", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		if err := svc.AddRoom(db, caller, authzID(t, db, "文理教师"), 1); err != nil {
			t.Fatalf("范围内加入教研室应成功: %v", err)
		}
	})

	t.Run("目标用户不在范围时移出教研室被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		err := svc.RemoveRoom(db, caller, authzID(t, db, "马院兼职教师"), 1)
		if err == nil {
			t.Fatalf("目标用户不在管理范围时应被拒绝")
		}
	})

	t.Run("系统管理员可跨学院操作教研室", func(t *testing.T) {
		db := userAuthzTestDB(t)
		caller := authzCaller(t, db, "系统管理员")
		if err := svc.AddRoom(db, caller, authzID(t, db, "马院教师A"), roomID(t, db, "马院教研室")); err != nil {
			t.Fatalf("系统管理员加入教研室应成功: %v", err)
		}
	})
}

func TestUserAuthzCollege(t *testing.T) {
	db := userAuthzTestDB(t)
	svc := &User{}

	t.Run("目标用户不在范围时加入督导学院被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		err := svc.AddCollege(db, caller, authzID(t, db, "马院教师B"), 5)
		if err == nil {
			t.Fatalf("目标用户不在管理范围时应被拒绝")
		}
	})

	t.Run("范围内目标加入督导学院成功", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		if err := svc.AddCollege(db, caller, authzID(t, db, "文理教师"), 5); err != nil {
			t.Fatalf("范围内加入学院应成功: %v", err)
		}
	})

	t.Run("系统管理员可操作任意用户", func(t *testing.T) {
		caller := authzCaller(t, db, "系统管理员")
		if err := svc.AddCollege(db, caller, authzID(t, db, "马院教师B"), 5); err != nil {
			t.Fatalf("系统管理员加入学院应成功: %v", err)
		}
	})
}

func TestUserAuthzAddRole(t *testing.T) {
	db := userAuthzTestDB(t)
	svc := &User{}

	t.Run("学院管理员给系统管理员加角色被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		err := svc.AddRole(db, caller, authzID(t, db, "系统管理员"), model.RoleTeacher)
		if err == nil {
			t.Fatalf("操作系统管理员应被拒绝")
		}
	})

	t.Run("学院管理员给跨学院用户加角色被拒", func(t *testing.T) {
		caller := authzCaller(t, db, "文理管理员")
		// 马院教师B 主学院6，不在文理管理员范围；teacher 角色本可分配，可验证为范围拦截
		err := svc.AddRole(db, caller, authzID(t, db, "马院教师B"), model.RoleTeacher)
		if err == nil {
			t.Fatalf("跨学院加角色应被拒绝")
		}
	})
}

func roomID(t *testing.T, db *gorm.DB, name string) int {
	t.Helper()
	var r model.ResearchRoom
	if err := db.Where("name = ?", name).First(&r).Error; err != nil {
		t.Fatalf("查教研室 %s 失败: %v", name, err)
	}
	return r.ID
}
