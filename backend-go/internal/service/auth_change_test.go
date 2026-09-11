package service

import (
	"testing"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// changePasswordTestDB 建最小表集合供改密测试（仅需 user 表）。
func changePasswordTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

func seedPasswordUser(t *testing.T, db *gorm.DB, userNo, password string) *model.User {
	t.Helper()
	hash, err := pwd.Hash(password)
	if err != nil {
		t.Fatalf("生成密码失败: %v", err)
	}
	u := &model.User{UserNo: userNo, Username: "教师", Role: model.RoleTeacher, Password: hash, Status: 1, MustChangePassword: true}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("灌入用户失败: %v", err)
	}
	return u
}

// AuthUserSnapshot 不含 Password 字段，ToUser 重建出的用户 Password 为空串。
// 改密校验必须回源 DB 加载真实记录，否则 Redis 缓存命中（快照未过期）时旧密码恒校验失败。
func TestChangePasswordWithSnapshotUser(t *testing.T) {
	db := changePasswordTestDB(t)
	svc := &Auth{}
	u := seedPasswordUser(t, db, "T001", "old-password")

	// 模拟认证快照重建用户：Password 为空、MustChangePassword 为快照值
	snapshotUser := &model.User{Model: model.Model{ID: u.ID}, UserNo: u.UserNo, Username: u.Username, Role: u.Role, Status: 1, MustChangePassword: true}

	t.Run("快照用户正确旧密码可改密并清标记", func(t *testing.T) {
		if err := svc.ChangePassword(db, snapshotUser, "old-password", "new-password"); err != nil {
			t.Fatalf("改密应成功: %v", err)
		}
		var got model.User
		if err := db.First(&got, u.ID).Error; err != nil {
			t.Fatalf("回查失败: %v", err)
		}
		if got.MustChangePassword {
			t.Fatal("改密后应清除强制改密标记")
		}
		if !pwd.Verify("new-password", got.Password) {
			t.Fatal("新密码应已落库")
		}
	})

	t.Run("旧密码错误仍被拒", func(t *testing.T) {
		if err := svc.ChangePassword(db, snapshotUser, "wrong-password", "another"); err == nil {
			t.Fatal("旧密码错误应被拒绝")
		}
	})

	t.Run("直连 DB 用户路径无回归", func(t *testing.T) {
		// 完整用户（含密码，直连 DB 场景）同样可改密
		var fullUser model.User
		if err := db.First(&fullUser, u.ID).Error; err != nil {
			t.Fatalf("取用户失败: %v", err)
		}
		if err := svc.ChangePassword(db, &fullUser, "new-password", "final-password"); err != nil {
			t.Fatalf("直连路径改密应成功: %v", err)
		}
	})
}