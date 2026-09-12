package service

import (
	"errors"
	"testing"
	"time"

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

// loginTestDB 建登录相关表集合（Login 会 Preload 角色/学院/教研室关联）。
func loginTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserRole{}, &model.UserCollege{},
		&model.UserRoom{}, &model.College{}, &model.ResearchRoom{}); err != nil {
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
		if err := svc.ChangePassword(db, snapshotUser, "old-password", "new-password1"); err != nil {
			t.Fatalf("改密应成功: %v", err)
		}
		var got model.User
		if err := db.First(&got, u.ID).Error; err != nil {
			t.Fatalf("回查失败: %v", err)
		}
		if got.MustChangePassword {
			t.Fatal("改密后应清除强制改密标记")
		}
		if !pwd.Verify("new-password1", got.Password) {
			t.Fatal("新密码应已落库")
		}
	})

	t.Run("弱口令被拒绝", func(t *testing.T) {
		// 纯字母口令不满足「含字母和数字」，且与工号相同也必须拦截
		if err := svc.ChangePassword(db, snapshotUser, "old-password", "new-password"); err == nil {
			t.Fatal("不含数字的口令应被拒绝")
		}
		if err := svc.ChangePassword(db, snapshotUser, "old-password", "T001"); err == nil {
			t.Fatal("口令等于工号应被拒绝")
		}
	})

	t.Run("旧密码错误仍被拒", func(t *testing.T) {
		if err := svc.ChangePassword(db, snapshotUser, "wrong-password", "another123"); err == nil {
			t.Fatal("旧密码错误应被拒绝")
		}
	})

	t.Run("直连 DB 用户路径无回归", func(t *testing.T) {
		// 完整用户（含密码，直连 DB 场景）同样可改密
		var fullUser model.User
		if err := db.First(&fullUser, u.ID).Error; err != nil {
			t.Fatalf("取用户失败: %v", err)
		}
		if err := svc.ChangePassword(db, &fullUser, "new-password1", "final-pass1"); err != nil {
			t.Fatalf("直连路径改密应成功: %v", err)
		}
	})
}

// createUserTestDB 建新增用户所需表集合（角色校验 + 关联预加载）
func createUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Role{}, &model.UserRole{},
		&model.UserCollege{}, &model.UserRoom{}, &model.College{}, &model.ResearchRoom{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	// checkRoleAssignable 会校验角色编码存在
	if err := db.Create(&model.Role{Code: model.RoleTeacher, Name: "教师"}).Error; err != nil {
		t.Fatalf("初始化角色失败: %v", err)
	}
	return db
}

// TestCreateUserDefaultPasswordNotUserNo 新建用户未指定口令时使用初始口令，
// 绝不使用工号（工号公开可知，等同账号裸奔），并强制首登改密。
func TestCreateUserDefaultPasswordNotUserNo(t *testing.T) {
	db := createUserTestDB(t)
	svc := NewUser("init-pass-2026")
	// 系统管理员可分配任意角色
	caller := &model.User{Role: model.RoleSystemAdmin}

	u, err := svc.Create(db, caller, CreateUserParams{UserNo: "T123", Username: "张三", Role: model.RoleTeacher})
	if err != nil {
		t.Fatalf("新建用户失败: %v", err)
	}
	if !u.MustChangePassword {
		t.Fatal("新建用户应要求首次登录改密")
	}
	if pwd.Verify("T123", u.Password) {
		t.Fatal("初始口令不得等于工号")
	}
	if !pwd.Verify("init-pass-2026", u.Password) {
		t.Fatal("初始口令应为配置值")
	}

	// 显式口令必须满足最小强度
	if _, err := svc.Create(db, caller,
		CreateUserParams{UserNo: "T124", Username: "李四", Role: model.RoleTeacher, Password: "short1"}); err == nil {
		t.Fatal("过短口令应被拒绝")
	}
	if _, err := svc.Create(db, caller,
		CreateUserParams{UserNo: "T125", Username: "王五", Role: model.RoleTeacher, Password: "T125"}); err == nil {
		t.Fatal("口令等于工号应被拒绝")
	}
	if _, err := svc.Create(db, caller,
		CreateUserParams{UserNo: "T126", Username: "赵六", Role: model.RoleTeacher, Password: "teacheronly"}); err == nil {
		t.Fatal("不含数字的口令应被拒绝")
	}
}

// TestUpdateUserPasswordStrength 管理员重置口令同样受最小强度约束
func TestUpdateUserPasswordStrength(t *testing.T) {
	db := createUserTestDB(t)
	svc := NewUser("init-pass-2026")
	caller := &model.User{Role: model.RoleSystemAdmin}
	u, err := svc.Create(db, caller, CreateUserParams{UserNo: "T200", Username: "钱七", Role: model.RoleTeacher})
	if err != nil {
		t.Fatalf("新建用户失败: %v", err)
	}

	weak := "12345678"
	if _, err := svc.Update(db, caller, u.ID, UpdateUserParams{Password: &weak}); err == nil {
		t.Fatal("弱口令重置应被拒绝")
	}
	asUserNo := "T200"
	if _, err := svc.Update(db, caller, u.ID, UpdateUserParams{Password: &asUserNo}); err == nil {
		t.Fatal("重置为工号应被拒绝")
	}
	strong := "reset-pass1"
	if _, err := svc.Update(db, caller, u.ID, UpdateUserParams{Password: &strong}); err != nil {
		t.Fatalf("合规口令重置应成功: %v", err)
	}
}

// TestLoginErrorIndistinguishable 防账号枚举：
//  1. 账号不存在与密码错误返回完全相同的错误（文案一致）；
//  2. 账号不存在时也执行一次 bcrypt 校验，耗时数量级与密码错误一致，
//     使响应时间不能作为「工号是否存在」的侧信道。
func TestLoginErrorIndistinguishable(t *testing.T) {
	db := loginTestDB(t)
	svc := &Auth{}
	seedPasswordUser(t, db, "T900", "correct-password")

	startMissing := time.Now()
	_, errMissing := svc.Login(db, "T999", "whatever")
	missingCost := time.Since(startMissing)

	startWrong := time.Now()
	_, errWrong := svc.Login(db, "T900", "wrong-password")
	wrongCost := time.Since(startWrong)

	if !errors.Is(errMissing, ErrInvalidCredentials) || !errors.Is(errWrong, ErrInvalidCredentials) {
		t.Fatalf("两类失败应返回同一错误: %v / %v", errMissing, errWrong)
	}
	if errMissing.Error() != errWrong.Error() {
		t.Fatalf("错误文案必须一致: %q / %q", errMissing.Error(), errWrong.Error())
	}
	if missingCost < wrongCost/10 {
		t.Fatalf("账号不存在的响应耗时明显偏短（%v vs %v），存在枚举侧信道", missingCost, wrongCost)
	}
}

// TestLoginDisabledUserNeedsValidPassword 禁用账号的状态只在口令正确时暴露，
// 用错误口令探测无法区分「账号被禁用」与「账号不存在」。
func TestLoginDisabledUserNeedsValidPassword(t *testing.T) {
	db := loginTestDB(t)
	svc := &Auth{}
	u := seedPasswordUser(t, db, "T901", "correct-password")
	if err := db.Model(&model.User{}).Where("id = ?", u.ID).Update("status", 0).Error; err != nil {
		t.Fatalf("禁用用户失败: %v", err)
	}

	if _, err := svc.Login(db, "T901", "correct-password"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("口令正确时应返回禁用错误, 实际: %v", err)
	}
	if _, err := svc.Login(db, "T901", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("口令错误时应返回统一凭证错误, 实际: %v", err)
	}
}
