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

// ChangePassword 修改密码并清除强制修改标记
func (s *Auth) ChangePassword(db *gorm.DB, user *model.User, oldPassword, newPassword string) error {
	if !pwd.Verify(oldPassword, user.Password) {
		return errors.New("旧密码错误")
	}
	hash, err := pwd.Hash(newPassword)
	if err != nil {
		return err
	}
	return db.Model(&model.User{}).Where("id = ?", user.ID).Omit(clause.Associations).Updates(map[string]interface{}{
		"password":             hash,
		"must_change_password": false,
	}).Error
}
