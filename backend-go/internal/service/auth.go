package service

import (
	"errors"
	"sync"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidCredentials = errors.New("工号或密码错误")
	ErrUserDisabled       = errors.New("用户已被禁用")
)

// Auth 认证相关业务逻辑
type Auth struct{}

func NewAuth() *Auth { return &Auth{} }

// dummyHash 惰性生成的固定口令哈希，用于「用户不存在」时消耗与真实校验等量的
// bcrypt 计算时间。成本参数与真实哈希一致（bcrypt.DefaultCost）。
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// equalizeVerifyTime 执行一次忽略结果的 bcrypt 校验，抹平响应耗时差异。
func equalizeVerifyTime(password string) {
	dummyHashOnce.Do(func() {
		if h, err := pwd.Hash("timing-equalization-placeholder"); err == nil {
			dummyHash = h
		}
	})
	if dummyHash != "" {
		_ = pwd.Verify(password, dummyHash)
	}
}

// Login 登录校验，成功返回用户。
//
// 防账号枚举：用户不存在与密码错误都返回同一个 ErrInvalidCredentials，
// 且前者也会执行一次 bcrypt 计算（耗时侧信道无法区分工号是否存在）。
// 禁用账号的判定放在口令校验之后，避免用错误口令探测账号状态。
func (s *Auth) Login(db *gorm.DB, userNo, password string) (*model.User, error) {
	var user model.User
	err := db.
		Preload("UserRoles").
		Preload("UserColleges.College").
		Preload("UserRooms.ResearchRoom.College").
		Preload("College").
		Preload("ResearchRoom").
		Where("user_no = ?", userNo).
		First(&user).Error
	if err != nil {
		equalizeVerifyTime(password)
		return nil, ErrInvalidCredentials
	}
	if !pwd.Verify(password, user.Password) {
		return nil, ErrInvalidCredentials
	}
	if user.Status != 1 {
		return nil, ErrUserDisabled
	}
	return &user, nil
}

// ChangePassword 修改密码并清除强制修改标记。
// 旧密码校验不依赖注入用户（可能来自认证快照，快照不含密码列），按 ID 回源 DB 加载真实记录。
func (s *Auth) ChangePassword(db *gorm.DB, user *model.User, oldPassword, newPassword string) error {
	var u model.User
	// 同时取 user_no/username：认证快照可能不含这两个字段，强度校验需要它们判断「口令不得等于工号」
	if err := db.Select("id", "password", "user_no", "username").First(&u, user.ID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if !pwd.Verify(oldPassword, u.Password) {
		return errors.New("旧密码错误")
	}
	// 最小强度校验（长度/不得等于工号或姓名/弱口令表）：默认口令取工号的历史问题在此收敛
	if err := pwd.Validate(newPassword, u.UserNo, u.Username); err != nil {
		return err
	}
	if newPassword == oldPassword {
		return errors.New("新密码不能与旧密码相同")
	}
	hash, err := pwd.Hash(newPassword)
	if err != nil {
		return err
	}
	return db.Model(&model.User{}).Where("id = ?", u.ID).Omit(clause.Associations).Updates(map[string]interface{}{
		"password":             hash,
		"must_change_password": false,
	}).Error
}
