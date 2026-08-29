package service

import (
	"errors"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// OrgService 组织架构业务（校区/学院/教研室）
type Org struct{}

func NewOrg() *Org { return &Org{} }

// ---------- 校区 ----------

func (s *Org) CampusList(db *gorm.DB) ([]model.Campus, error) {
	var list []model.Campus
	err := db.Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

// CampusCollegeCount campus_id -> 启用学院数
func (s *Org) CampusCollegeCount(db *gorm.DB) map[int]int {
	type row struct {
		CampusID int
		Cnt      int64
	}
	var rows []row
	db.Model(&model.College{}).
		Select("campus_id, COUNT(*) AS cnt").
		Where("status = 1 AND campus_id IS NOT NULL").
		Group("campus_id").Scan(&rows)
	m := map[int]int{}
	for _, r := range rows {
		m[r.CampusID] = int(r.Cnt)
	}
	return m
}

func (s *Org) CampusCreate(db *gorm.DB, name string, sort int, status int16) (*model.Campus, error) {
	c := model.Campus{Name: name, SortOrder: sort, Status: status}
	if err := db.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Org) CampusUpdate(db *gorm.DB, id int, name *string, sort *int, status *int16) (*model.Campus, error) {
	var c model.Campus
	if err := db.First(&c, id).Error; err != nil {
		return nil, errors.New("校区不存在")
	}
	updates := map[string]interface{}{}
	if name != nil {
		updates["name"] = *name
	}
	if sort != nil {
		updates["sort_order"] = *sort
	}
	if status != nil {
		updates["status"] = *status
	}
	if len(updates) > 0 {
		if err := db.Model(&c).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return &c, nil
}

func (s *Org) CampusDelete(db *gorm.DB, id int) error {
	var cnt int64
	db.Model(&model.College{}).Where("campus_id = ? AND status = 1", id).Count(&cnt)
	if cnt > 0 {
		return errors.New("该校区下存在启用的学院，禁止删除")
	}
	return db.Delete(&model.Campus{}, id).Error
}

// ---------- 学院 ----------

// CollegeList 列表；scopeIDs 非 nil 时仅返回范围内学院
func (s *Org) CollegeList(db *gorm.DB, keyword, campusID string, status *int16, scopeIDs []int) ([]model.College, error) {
	q := db.Model(&model.College{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", kw, kw)
	}
	if campusID != "" {
		q = q.Where("campus_id = ?", campusID)
	}
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	if scopeIDs != nil {
		if len(scopeIDs) == 0 {
			return []model.College{}, nil
		}
		q = q.Where("id IN ?", scopeIDs)
	}
	var list []model.College
	err := q.Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (s *Org) CollegeCounts(db *gorm.DB) (rooms map[int]int, users map[int]int) {
	type row struct {
		CollegeID int
		Cnt       int64
	}
	var rs []row
	db.Model(&model.ResearchRoom{}).Select("college_id, COUNT(*) AS cnt").
		Where("status = 1").Group("college_id").Scan(&rs)
	rooms = map[int]int{}
	for _, r := range rs {
		rooms[r.CollegeID] = int(r.Cnt)
	}
	var us []row
	db.Table("user").Select("college_id, COUNT(*) AS cnt").
		Where("status = 1 AND college_id IS NOT NULL").Group("college_id").Scan(&us)
	users = map[int]int{}
	for _, r := range us {
		users[r.CollegeID] = int(r.Cnt)
	}
	return
}

func (s *Org) CollegeCreate(db *gorm.DB, code, name string, campusID *int, sort int, status int16) (*model.College, error) {
	var cnt int64
	db.Model(&model.College{}).Where("code = ?", code).Count(&cnt)
	if cnt > 0 {
		return nil, errors.New("学院编码已存在")
	}
	if campusID != nil {
		var c int64
		db.Model(&model.Campus{}).Where("id = ?", *campusID).Count(&c)
		if c == 0 {
			return nil, errors.New("所属校区不存在")
		}
	}
	col := model.College{Code: code, Name: name, CampusID: campusID, SortOrder: int16(sort), Status: status}
	if err := db.Create(&col).Error; err != nil {
		return nil, err
	}
	return &col, nil
}

func (s *Org) CollegeUpdate(db *gorm.DB, id int, code, name *string, campusID *int, sort *int, status *int16) (*model.College, error) {
	var col model.College
	if err := db.First(&col, id).Error; err != nil {
		return nil, errors.New("学院不存在")
	}
	updates := map[string]interface{}{}
	if code != nil && *code != col.Code {
		var cnt int64
		db.Model(&model.College{}).Where("code = ? AND id <> ?", *code, id).Count(&cnt)
		if cnt > 0 {
			return nil, errors.New("学院编码已存在")
		}
		updates["code"] = *code
	}
	if name != nil {
		updates["name"] = *name
	}
	if campusID != nil {
		updates["campus_id"] = *campusID
	}
	if sort != nil {
		updates["sort_order"] = *sort
	}
	if status != nil {
		updates["status"] = *status
	}
	if len(updates) > 0 {
		if err := db.Model(&col).Updates(updates).Error; err != nil {
			return nil, err
		}
		db.First(&col, id) // 回填更新后的值
	}
	return &col, nil
}

func (s *Org) CollegeDelete(db *gorm.DB, id int) error {
	var rooms, users int64
	db.Model(&model.ResearchRoom{}).Where("college_id = ? AND status = 1", id).Count(&rooms)
	db.Table("user").Where("college_id = ? AND status = 1", id).Count(&users)
	if rooms > 0 || users > 0 {
		return errors.New("该学院下存在启用的教研室或用户，禁止删除")
	}
	return db.Delete(&model.College{}, id).Error
}

// ---------- 教研室 ----------

func (s *Org) RoomList(db *gorm.DB, keyword, collegeID string, status *int16, scopeIDs []int) ([]model.ResearchRoom, error) {
	q := db.Model(&model.ResearchRoom{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", kw, kw)
	}
	if collegeID != "" {
		q = q.Where("college_id = ?", collegeID)
	}
	if status != nil {
		q = q.Where("status = ?", *status)
	}
	if scopeIDs != nil {
		if len(scopeIDs) == 0 {
			return []model.ResearchRoom{}, nil
		}
		q = q.Where("college_id IN ?", scopeIDs)
	}
	var list []model.ResearchRoom
	err := q.Order("id DESC").Find(&list).Error // 对齐旧端 order_by=id, order=desc
	return list, err
}

func (s *Org) RoomCreate(db *gorm.DB, code, name string, collegeID int, status int16) (*model.ResearchRoom, error) {
	var cnt int64
	db.Model(&model.ResearchRoom{}).Where("code = ?", code).Count(&cnt)
	if cnt > 0 {
		return nil, errors.New("教研室编码已存在")
	}
	var c int64
	db.Model(&model.College{}).Where("id = ?", collegeID).Count(&c)
	if c == 0 {
		return nil, errors.New("所属学院不存在")
	}
	room := model.ResearchRoom{Code: code, Name: name, CollegeID: collegeID, Status: status}
	if err := db.Create(&room).Error; err != nil {
		return nil, err
	}
	return &room, nil
}

func (s *Org) RoomUpdate(db *gorm.DB, id int, code, name *string, collegeID *int, status *int16) (*model.ResearchRoom, error) {
	var room model.ResearchRoom
	if err := db.First(&room, id).Error; err != nil {
		return nil, errors.New("教研室不存在")
	}
	updates := map[string]interface{}{}
	if code != nil && *code != room.Code {
		var cnt int64
		db.Model(&model.ResearchRoom{}).Where("code = ? AND id <> ?", *code, id).Count(&cnt)
		if cnt > 0 {
			return nil, errors.New("教研室编码已存在")
		}
		updates["code"] = *code
	}
	if name != nil {
		updates["name"] = *name
	}
	if collegeID != nil {
		updates["college_id"] = *collegeID
	}
	if status != nil {
		updates["status"] = *status
	}
	if len(updates) > 0 {
		if err := db.Model(&room).Updates(updates).Error; err != nil {
			return nil, err
		}
		db.First(&room, id) // 回填更新后的值
	}
	return &room, nil
}

func (s *Org) RoomDelete(db *gorm.DB, id int) error {
	var users int64
	db.Table("user").Where("research_room_id = ? AND status = 1", id).Count(&users)
	db.Table("user_research_room").Where("research_room_id = ?", id).Count(&users)
	if users > 0 {
		return errors.New("该教研室存在用户引用，禁止删除")
	}
	return db.Delete(&model.ResearchRoom{}, id).Error
}
