package service

import (
	"errors"

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

// Login 登录校验，成功返回用户
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
	if err := db.Select("id", "password").First(&u, user.ID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if !pwd.Verify(oldPassword, u.Password) {
		return errors.New("旧密码错误")
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
