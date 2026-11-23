package service

import (
	"errors"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// TeachingCollegeItem 授课学院管理列表项
type TeachingCollegeItem struct {
	UserID             int              `gorm:"column:user_id" json:"user_id"`
	UserNo             string           `gorm:"column:user_no" json:"user_no"`
	Username           string           `gorm:"column:username" json:"username"`
	TeacherCollegeID   *int             `gorm:"column:teacher_college_id" json:"teacher_college_id"`
	TeacherCollegeName string           `gorm:"column:teacher_college_name" json:"teacher_college_name"`
	CollegeID          int              `gorm:"column:college_id" json:"college_id"`
	CollegeName        string           `gorm:"column:college_name" json:"college_name"`
	Source             string           `gorm:"column:source" json:"source"`
	JoinTime           *model.LocalTime `gorm:"column:join_time" json:"join_time"`
}

// TeachingCollegeListParams 授课学院管理列表筛选
type TeachingCollegeListParams struct {
	Page      int
	PageSize  int
	Keyword   string
	CollegeID *int
}

// teachingCollegeScope 授课学院管理范围：system_admin 全校（nil），
// 学院管理员仅本人主学院；其他角色无权。
func teachingCollegeScope(caller *model.User) ([]int, error) {
	if caller == nil || !IsAdminRole(caller) {
		return nil, errors.New("无权管理授课学院")
	}
	return UserManageCollegeIDs(caller), nil
}

// ListTeachingColleges 授课学院关联列表（按调用者范围收敛）
func (s *User) ListTeachingColleges(db *gorm.DB, caller *model.User, p TeachingCollegeListParams) ([]TeachingCollegeItem, int64, error) {
	scope, err := teachingCollegeScope(caller)
	if err != nil {
		return nil, 0, err
	}
	if scope != nil && len(scope) == 0 {
		return []TeachingCollegeItem{}, 0, nil
	}
	base := db.Table("teacher_teaching_college AS ttc").
		Joins("JOIN `user` u ON u.id = ttc.user_id").
		Joins("LEFT JOIN `college` c ON c.id = ttc.college_id").
		Joins("LEFT JOIN `college` mc ON mc.id = u.college_id")
	if scope != nil {
		base = base.Where("ttc.college_id IN ?", scope)
	}
	if p.CollegeID != nil {
		base = base.Where("ttc.college_id = ?", *p.CollegeID)
	}
	if p.Keyword != "" {
		kw := "%" + p.Keyword + "%"
		base = base.Where("(u.user_no LIKE ? OR u.username LIKE ?)", kw, kw)
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 20
	}
	var list []TeachingCollegeItem
	err = base.Session(&gorm.Session{}).
		Select("ttc.user_id, u.user_no, u.username, u.college_id AS teacher_college_id, mc.name AS teacher_college_name, " +
			"ttc.college_id, c.name AS college_name, ttc.source, ttc.join_time").
		Order("ttc.college_id ASC, u.username ASC").
		Offset((p.Page - 1) * p.PageSize).Limit(p.PageSize).
		Scan(&list).Error
	if err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// TeachingCollegeCandidates 候选教师：跨学院搜索启用中的教师（供学院管理员添加外院授课教师）。
// 仅返回最小字段，避免扩大用户管理数据范围。
func (s *User) TeachingCollegeCandidates(db *gorm.DB, caller *model.User, keyword string, limit int) ([]model.User, error) {
	if _, err := teachingCollegeScope(caller); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	q := db.Model(&model.User{}).Where("status = 1 AND " + teacherCondAlias("user"))
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("(user_no LIKE ? OR username LIKE ?)", kw, kw)
	}
	var users []model.User
	if err := q.Preload("College").Order("username ASC, id ASC").Limit(limit).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// AddTeachingCollege 添加授课学院关系。
// 目标教师可以不在调用者的用户管理范围内（外聘教师人事学院在别处），
// 但写入的学院必须在调用者可管理范围内；重复添加按幂等成功处理。
func (s *User) AddTeachingCollege(db *gorm.DB, caller *model.User, userID, collegeID int) error {
	scope, err := teachingCollegeScope(caller)
	if err != nil {
		return err
	}
	if scope != nil && !containsInt(scope, collegeID) {
		return errors.New("只能维护本学院的授课教师")
	}
	var u model.User
	if err := db.Preload("UserRoles").First(&u, userID).Error; err != nil {
		return errors.New("教师不存在")
	}
	if u.Status != 1 {
		return errors.New("教师已停用")
	}
	if u.Role != model.RoleTeacher && !u.HasRole(model.RoleTeacher) {
		return errors.New("只能为教师角色维护授课学院")
	}
	var college model.College
	if err := db.First(&college, collegeID).Error; err != nil {
		return errors.New("学院不存在")
	}
	var existing model.TeacherTeachingCollege
	err = db.Where("user_id = ? AND college_id = ?", userID, collegeID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.TeacherTeachingCollege{
			UserID: userID, CollegeID: collegeID, Source: "manual",
			JoinTime: model.LocalTimePtr(time.Now()),
		}).Error
	}
	if err != nil {
		return err
	}
	return db.Model(&existing).Updates(map[string]interface{}{
		"source": "manual", "join_time": model.LocalTimePtr(time.Now()),
	}).Error
}

// RemoveTeachingCollege 删除授课学院关系（范围校验同添加）
func (s *User) RemoveTeachingCollege(db *gorm.DB, caller *model.User, userID, collegeID int) error {
	scope, err := teachingCollegeScope(caller)
	if err != nil {
		return err
	}
	if scope != nil && !containsInt(scope, collegeID) {
		return errors.New("只能维护本学院的授课教师")
	}
	res := db.Where("user_id = ? AND college_id = ?", userID, collegeID).Delete(&model.TeacherTeachingCollege{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("授课学院关系不存在")
	}
	return nil
}
