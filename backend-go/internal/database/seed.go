package database

import (
	"fmt"
	"time"

	"backend-go/internal/config"
	"backend-go/internal/model"
	"backend-go/pkg/pwd"

	"gorm.io/gorm"
)

// seedAccounts 默认账号（对齐 README「默认账号」与 V2 init_data.py）。
var seedAccounts = []struct {
	UserNo   string
	Username string
	Role     string
}{
	{"admin", "系统管理员", model.RoleSystemAdmin},
	{"S001", "学院管理员", model.RoleCollegeAdmin},
	{"T001", "测试老师", model.RoleTeacher},
	{"D001", "督导老师", model.RoleSupervisor},
}

// SeedAccounts 空库首次启动时创建默认账号（幂等：工号已存在则跳过，不覆盖已有账号）。
// 账号种子不走 SQL：初始口令需按环境变量 DEFAULT_USER_PASSWORD 做 bcrypt，
// SQL 无法表达，故对齐 V2 init_data.py 在应用层创建。口令即 DEFAULT_USER_PASSWORD，
// 并标记 must_change_password 强制首登改密。
func SeedAccounts(db *gorm.DB, cfg *config.Config) error {
	hash, err := pwd.Hash(cfg.DefaultUserPassword)
	if err != nil {
		return fmt.Errorf("hash DEFAULT_USER_PASSWORD: %w", err)
	}
	for _, a := range seedAccounts {
		var cnt int64
		if err := db.Model(&model.User{}).Where("user_no = ?", a.UserNo).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue
		}
		now := model.LocalTimePtr(time.Now())
		u := model.User{
			Model:              model.Model{CreateTime: now, UpdateTime: now},
			UserNo:             a.UserNo,
			Username:           a.Username,
			Password:           hash,
			Role:               a.Role,
			Status:             1,
			MustChangePassword: true,
		}
		if err := db.Create(&u).Error; err != nil {
			return fmt.Errorf("seed user %s: %w", a.UserNo, err)
		}
		ur := model.UserRole{UserID: u.ID, Role: a.Role, AssignTime: now}
		ur.CreateTime, ur.UpdateTime = now, now
		if err := db.Create(&ur).Error; err != nil {
			return fmt.Errorf("seed user_role %s: %w", a.UserNo, err)
		}
	}
	return nil
}
