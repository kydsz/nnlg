package service

import (
	"errors"
	"strings"
	"time"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserParams 用户列表查询参数
type UserParams struct {
	Page           int
	PageSize       int
	Keyword        string // 匹配工号/姓名
	Role           string
	CollegeID      string // 支持逗号分隔多个学院 ID
	ResearchRoomID string
	Status         *int
	NoCollege      bool
	NoResearchRoom bool
	OrderBy        string // 排序字段（user_no/username/college_name，白名单过滤）
	OrderDir       string // asc/desc，默认 asc（工号从小到大）
}

// sortDir 排序方向白名单：仅 asc/desc，其余默认 asc
func sortDir(dir string) string {
	if strings.ToLower(dir) == "desc" {
		return "DESC"
	}
	return "ASC"
}

// User 用户相关业务逻辑
type User struct{}

func NewUser() *User { return &User{} }

// List 分页查询用户（预加载关联，排序/筛选语义与旧端一致）
func (s *User) List(db *gorm.DB, p UserParams) ([]model.User, int64, error) {
	q := db.Model(&model.User{})
	if p.Keyword != "" {
		kw := "%" + p.Keyword + "%"
		q = q.Where("user_no LIKE ? OR username LIKE ?", kw, kw)
	}
	if p.Role != "" {
		// 旧端：主角色匹配（college_admin/school_admin 互为别名）或 user_role 关联
		roles := []string{p.Role}
		if p.Role == model.RoleCollegeAdmin {
			roles = append(roles, model.RoleSchoolAdmin)
		} else if p.Role == model.RoleSchoolAdmin {
			roles = append(roles, model.RoleCollegeAdmin)
		}
		q = q.Where("`user`.`role` IN ? OR `user`.`id` IN (SELECT user_id FROM user_role WHERE role IN ?)", roles, roles)
	}
	if p.CollegeID != "" {
		ids := splitInts(p.CollegeID)
		// 学院筛选只匹配主学院：user_college 表混存了督导「负责学院」与部分教师的杂散关联，
		// 用它匹配用户会把主学院不在该学院的用户漏出（与 applyTeacherScope / ListTeachers 保持一致）
		q = q.Where("`user`.`college_id` IN ?", ids)
	}
	if p.ResearchRoomID != "" {
		q = q.Where("`user`.`research_room_id` = ? OR `user`.`id` IN (SELECT user_id FROM user_research_room WHERE research_room_id = ?)", p.ResearchRoomID, p.ResearchRoomID)
	}
	if p.Status != nil {
		q = q.Where("status = ?", *p.Status)
	}
	if p.NoCollege {
		q = q.Where("`user`.`college_id` IS NULL AND `user`.`id` NOT IN (SELECT user_id FROM user_college)")
	}
	if p.NoResearchRoom {
		q = q.Where("`user`.`research_room_id` IS NULL AND `user`.`id` NOT IN (SELECT user_id FROM user_research_room)")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []model.User
	offset := (p.Page - 1) * p.PageSize
	// 白名单排序（防注入）：工号/姓名/学院名；学院名需关联学院表，空学院排最后
	switch p.OrderBy {
	case "user_no":
		q = q.Order("`user`.`user_no` " + sortDir(p.OrderDir) + ", `user`.`id` DESC")
	case "username":
		q = q.Order("`user`.`username` " + sortDir(p.OrderDir) + ", `user`.`id` DESC")
	case "college_name":
		q = q.Joins("LEFT JOIN college c ON c.id = `user`.`college_id`").
			Order("c.name IS NULL ASC, c.name " + sortDir(p.OrderDir) + ", `user`.`id` DESC")
	default:
		q = q.Order("id DESC")
	}
	err := q.Session(&gorm.Session{}).
		Preload("UserRoles").
		Preload("UserColleges.College").
		Preload("UserRooms.ResearchRoom.College").
		Preload("College").
		Preload("ResearchRoom").
		Offset(offset).Limit(p.PageSize).
		Find(&users).Error
	return users, total, err
}

// GetByID 按 ID 查询用户（预加载关联）
func (s *User) GetByID(db *gorm.DB, id int) (*model.User, error) {
	var u model.User
	err := db.
		Preload("UserRoles").
		Preload("UserColleges.College").
		Preload("UserRooms.ResearchRoom.College").
		Preload("College").
		Preload("ResearchRoom").
		First(&u, id).Error
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	return &u, nil
}

// assignableRoles 按当前操作者返回可分配的角色编码集合；nil 表示全部
func assignableRoles(caller *model.User) map[string]bool {
	switch {
	case caller.HasRole(model.RoleSystemAdmin):
		return nil // 全部
	case caller.HasRole(model.RoleSchoolAdmin):
		return map[string]bool{
			model.RoleSchoolSupervisor: true, model.RoleCollegeSupervisor: true,
			model.RoleSupervisor: true, model.RoleTeacher: true,
		}
	case caller.HasRole(model.RoleCollegeAdmin):
		return map[string]bool{model.RoleTeacher: true}
	default:
		return map[string]bool{}
	}
}

// checkRoleAssignable 校验角色编码可被 caller 分配
func checkRoleAssignable(db *gorm.DB, caller *model.User, codes []string) error {
	allowed := assignableRoles(caller)
	for _, c := range codes {
		if allowed != nil && !allowed[c] {
			return errors.New("不可分配角色: " + c)
		}
		var cnt int64
		db.Model(&model.Role{}).Where("code = ?", c).Count(&cnt)
		if cnt == 0 {
			return errors.New("角色不存在: " + c)
		}
	}
	return nil
}

// CreateUserParams 新增用户参数
type CreateUserParams struct {
	UserNo             string   `json:"user_no"`
	Username           string   `json:"username"`
	Role               string   `json:"role"`
	Roles              []string `json:"roles"`
	CollegeID          *int     `json:"college_id"`
	ResearchRoomID     *int     `json:"research_room_id"`
	SupervisorColleges []int    `json:"supervisor_college_ids"`
	SupervisorRooms    []int    `json:"supervisor_research_room_ids"`
	Status             int16    `json:"status"`
	Password           string   `json:"password"`
}

// Create 新增用户
func (s *User) Create(db *gorm.DB, caller *model.User, p CreateUserParams) (*model.User, error) {
	if p.UserNo == "" || p.Username == "" {
		return nil, errors.New("工号与姓名不能为空")
	}
	var cnt int64
	db.Model(&model.User{}).Where("user_no = ?", p.UserNo).Count(&cnt)
	if cnt > 0 {
		return nil, errors.New("工号已存在")
	}

	roles := p.Roles
	if len(roles) == 0 && p.Role != "" {
		roles = []string{p.Role}
	}
	if len(roles) == 0 {
		return nil, errors.New("至少分配一个角色")
	}
	if err := checkRoleAssignable(db, caller, roles); err != nil {
		return nil, err
	}

	password := p.Password
	// 旧端：新建用户一律要求首次登录改密
	mustChange := true
	if password == "" {
		password = p.UserNo
	}
	hash, err := pwd.Hash(password)
	if err != nil {
		return nil, err
	}

	// 旧端：主学院/主教研室自动纳入督导范围
	supColleges := p.SupervisorColleges
	if p.CollegeID != nil && !containsInt(supColleges, *p.CollegeID) {
		supColleges = append(supColleges, *p.CollegeID)
	}
	supRooms := p.SupervisorRooms
	if p.ResearchRoomID != nil && !containsInt(supRooms, *p.ResearchRoomID) {
		supRooms = append(supRooms, *p.ResearchRoomID)
	}

	status := p.Status
	if status == 0 {
		status = 1
	}
	u := model.User{
		UserNo: p.UserNo, Username: p.Username, Password: hash,
		Role: roles[0], CollegeID: p.CollegeID, ResearchRoomID: p.ResearchRoomID,
		Status: status, MustChangePassword: mustChange,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		return s.assignRelations(tx, u.ID, roles, supColleges, supRooms)
	})
	if err != nil {
		return nil, err
	}
	return s.GetByID(db, u.ID)
}

// assignRelations 写入角色与督导单位关联
func (s *User) assignRelations(tx *gorm.DB, userID int, roles []string, collegeIDs, roomIDs []int) error {
	seen := map[string]bool{}
	for _, r := range roles {
		if seen[r] {
			continue
		}
		seen[r] = true
		if err := tx.Create(&model.UserRole{UserID: userID, Role: r}).Error; err != nil {
			return err
		}
	}
	for _, cid := range collegeIDs {
		if err := tx.Create(&model.UserCollege{UserID: userID, CollegeID: cid}).Error; err != nil {
			return err
		}
	}
	for _, rid := range roomIDs {
		if err := tx.Create(&model.UserRoom{UserID: userID, ResearchRoomID: rid}).Error; err != nil {
			return err
		}
	}
	return nil
}

// UpdateUserParams 编辑用户参数（指针为 nil 表示不修改）
type UpdateUserParams struct {
	Username           *string   `json:"username"`
	Roles              *[]string `json:"roles"`
	CollegeID          *int      `json:"college_id"`
	ResearchRoomID     *int      `json:"research_room_id"`
	SupervisorColleges *[]int    `json:"supervisor_college_ids"`
	SupervisorRooms    *[]int    `json:"supervisor_research_room_ids"`
	Status             *int16    `json:"status"`
	Password           *string   `json:"password"`
	MustChangePassword *bool     `json:"must_change_password"`
}

// Update 编辑用户
func (s *User) Update(db *gorm.DB, caller *model.User, id int, p UpdateUserParams) (*model.User, error) {
	u, err := s.GetByID(db, id)
	if err != nil {
		return nil, err
	}

	var newRoles []string
	if p.Roles != nil {
		newRoles = *p.Roles
		if len(newRoles) == 0 {
			return nil, errors.New("至少分配一个角色")
		}
		if err := checkRoleAssignable(db, caller, newRoles); err != nil {
			return nil, err
		}
	}

	updates := map[string]interface{}{}
	if p.Username != nil {
		updates["username"] = *p.Username
	}
	if p.CollegeID != nil {
		updates["college_id"] = *p.CollegeID
	}
	if p.ResearchRoomID != nil {
		updates["research_room_id"] = *p.ResearchRoomID
	}
	if p.Status != nil {
		updates["status"] = *p.Status
	}
	if p.MustChangePassword != nil {
		updates["must_change_password"] = *p.MustChangePassword
	}
	if p.Password != nil && *p.Password != "" {
		hash, err := pwd.Hash(*p.Password)
		if err != nil {
			return nil, err
		}
		updates["password"] = hash
	}

	// 最后一个启用 system_admin 保护：不可禁用、不可移除其管理员角色
	if s.isAdminUser(db, u) && u.Status == 1 {
		willDisable := p.Status != nil && *p.Status == 0
		roleRemoved := p.Roles != nil && !containsString(newRoles, model.RoleSystemAdmin)
		if (willDisable || roleRemoved) && s.activeAdminCount(db, u.ID) == 0 {
			return nil, errors.New("系统至少需保留一个启用的系统管理员")
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			// Omit(clause.Associations)：u 预加载了 user_role/user_college 等关联，
			// 不加 Omit 时 gorm 会以 upsert 方式自动保存关联，导致下方 DELETE 后旧行被写回、再 INSERT 撞唯一索引
			if err := tx.Model(&model.User{}).Where("id = ?", u.ID).Omit(clause.Associations).Updates(updates).Error; err != nil {
				return err
			}
		}
		if p.Roles != nil {
			if err := tx.Where("user_id = ?", u.ID).Delete(&model.UserRole{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.User{}).Where("id = ?", u.ID).Omit(clause.Associations).Update("role", newRoles[0]).Error; err != nil {
				return err
			}
		}
		if p.SupervisorColleges != nil {
			if err := tx.Where("user_id = ?", u.ID).Delete(&model.UserCollege{}).Error; err != nil {
				return err
			}
		}
		if p.SupervisorRooms != nil {
			if err := tx.Where("user_id = ?", u.ID).Delete(&model.UserRoom{}).Error; err != nil {
				return err
			}
		}
		var colleges, rooms []int
		if p.SupervisorColleges != nil {
			colleges = *p.SupervisorColleges
		}
		if p.SupervisorRooms != nil {
			rooms = *p.SupervisorRooms
		}
		if p.Roles != nil || p.SupervisorColleges != nil || p.SupervisorRooms != nil {
			return s.assignRelations(tx, u.ID, newRoles, colleges, rooms)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetByID(db, u.ID)
}

// Delete 删除用户及其关联（硬删除）
func (s *User) Delete(db *gorm.DB, id int) error {
	u, err := s.GetByID(db, id)
	if err != nil {
		return err
	}
	if s.isAdminUser(db, u) && u.Status == 1 && s.activeAdminCount(db, u.ID) == 0 {
		return errors.New("系统至少需保留一个启用的系统管理员")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UserCollege{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRoom{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, id).Error
	})
}

// UpdateMyResearchRoom 教师自助修改主教研室（仅主学院下的教研室）
func (s *User) UpdateMyResearchRoom(db *gorm.DB, u *model.User, roomID *int) (*string, error) {
	if roomID == nil {
		if err := db.Model(&model.User{}).Where("id = ?", u.ID).Omit(clause.Associations).Update("research_room_id", nil).Error; err != nil {
			return nil, err
		}
		return nil, nil
	}
	var room model.ResearchRoom
	if err := db.First(&room, *roomID).Error; err != nil {
		return nil, errors.New("教研室不存在")
	}
	if room.CollegeID != 0 && !u.HasCollegeID(room.CollegeID) {
		return nil, errForbidden("您不属于该教研室所属的学院")
	}
	if err := db.Model(&model.User{}).Where("id = ?", u.ID).Omit(clause.Associations).Update("research_room_id", *roomID).Error; err != nil {
		return nil, err
	}
	return &room.Name, nil
}

// UpdateStatus 启用/禁用用户
func (s *User) UpdateStatus(db *gorm.DB, caller *model.User, id, status int) (*model.User, error) {
	u, err := s.GetByID(db, id)
	if err != nil {
		return nil, err
	}
	if id == caller.ID {
		return nil, errors.New("不能禁用自己")
	}
	if s.isAdminUser(db, u) && !caller.HasRole(model.RoleSystemAdmin) {
		return nil, errors.New("无权操作系统管理员")
	}
	if status == 0 && s.isAdminUser(db, u) && u.Status == 1 && s.activeAdminCount(db, u.ID) == 0 {
		return nil, errors.New("不能禁用最后一个活跃的系统管理员")
	}
	if err := db.Model(&model.User{}).Where("id = ?", u.ID).Omit(clause.Associations).Update("status", status).Error; err != nil {
		return nil, err
	}
	return s.GetByID(db, id)
}

// BatchStatus 批量启用/禁用（系统管理员账号一律跳过）
func (s *User) BatchStatus(db *gorm.DB, caller *model.User, ids []int, status int) (success, failed int, changed []int) {
	for _, id := range ids {
		u, err := s.GetByID(db, id)
		if err != nil || id == caller.ID || s.isAdminUser(db, u) {
			failed++
			continue
		}
		if err := db.Model(&model.User{}).Where("id = ?", id).Update("status", status).Error; err != nil {
			failed++
			continue
		}
		success++
		changed = append(changed, id)
	}
	return
}

// AddRole 添加用户角色
func (s *User) AddRole(db *gorm.DB, caller *model.User, userID int, role string) error {
	u, err := s.GetByID(db, userID)
	if err != nil {
		return err
	}
	if err := checkRoleAssignable(db, caller, []string{role}); err != nil {
		return errors.New("无权分配 " + role + " 角色")
	}
	if u.HasRole(role) {
		return nil // 幂等
	}
	return db.Transaction(func(tx *gorm.DB) error {
		now := timeNow()
		if err := tx.Create(&model.UserRole{UserID: userID, Role: role, AssignTime: model.LocalTimePtr(now)}).Error; err != nil {
			return err
		}
		if u.Role == "" {
			return tx.Model(&model.User{}).Where("id = ?", userID).Update("role", role).Error
		}
		return nil
	})
}

// RemoveRole 移除用户角色（至少保留一个）
func (s *User) RemoveRole(db *gorm.DB, caller *model.User, userID int, role string) error {
	if _, err := s.GetByID(db, userID); err != nil {
		return err
	}
	if err := checkRoleAssignable(db, caller, []string{role}); err != nil {
		return errors.New("无权移除 " + role + " 角色")
	}
	var u model.User
	if err := db.Preload("UserRoles").First(&u, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if len(u.RoleCodes()) <= 1 {
		return errors.New("用户至少需要一个角色")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("user_id = ? AND role = ?", userID, role).Delete(&model.UserRole{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("用户没有该角色")
		}
		// 兼容字段：若主角色被移除，切换为剩余第一个角色
		if u.Role == role {
			rest := []string{}
			for _, c := range u.RoleCodes() {
				if c != role {
					rest = append(rest, c)
				}
			}
			if len(rest) > 0 {
				return tx.Model(&model.User{}).Where("id = ?", userID).Update("role", rest[0]).Error
			}
		}
		return nil
	})
}

// AddCollege 加入督导学院
func (s *User) AddCollege(db *gorm.DB, caller *model.User, userID, collegeID int) error {
	if _, err := s.GetByID(db, userID); err != nil {
		return err
	}
	if !caller.HasRole(model.RoleSystemAdmin) && !caller.HasCollegeID(collegeID) {
		return errors.New("无权管理该学院")
	}
	var college model.College
	if err := db.First(&college, collegeID).Error; err != nil {
		return errors.New("学院不存在")
	}
	var cnt int64
	db.Model(&model.UserCollege{}).Where("user_id = ? AND college_id = ?", userID, collegeID).Count(&cnt)
	if cnt > 0 {
		return nil // 幂等
	}
	now := timeNow()
	return db.Create(&model.UserCollege{UserID: userID, CollegeID: collegeID, JoinTime: model.LocalTimePtr(now)}).Error
}

// RemoveCollege 移出督导学院
func (s *User) RemoveCollege(db *gorm.DB, caller *model.User, userID, collegeID int) error {
	if _, err := s.GetByID(db, userID); err != nil {
		return err
	}
	if !caller.HasRole(model.RoleSystemAdmin) && !caller.HasCollegeID(collegeID) {
		return errors.New("无权管理该学院")
	}
	res := db.Where("user_id = ? AND college_id = ?", userID, collegeID).Delete(&model.UserCollege{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("用户不在该学院中")
	}
	return nil
}

// AddRoom 加入教研室
func (s *User) AddRoom(db *gorm.DB, caller *model.User, userID, roomID int) error {
	if _, err := s.GetByID(db, userID); err != nil {
		return err
	}
	var room model.ResearchRoom
	if err := db.First(&room, roomID).Error; err != nil {
		return errors.New("教研室不存在")
	}
	var cnt int64
	db.Model(&model.UserRoom{}).Where("user_id = ? AND research_room_id = ?", userID, roomID).Count(&cnt)
	if cnt > 0 {
		return nil // 幂等
	}
	now := timeNow()
	return db.Create(&model.UserRoom{UserID: userID, ResearchRoomID: roomID, JoinTime: model.LocalTimePtr(now)}).Error
}

// RemoveRoom 移出教研室
func (s *User) RemoveRoom(db *gorm.DB, caller *model.User, userID, roomID int) error {
	if _, err := s.GetByID(db, userID); err != nil {
		return err
	}
	res := db.Where("user_id = ? AND research_room_id = ?", userID, roomID).Delete(&model.UserRoom{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("用户不在该教研室中")
	}
	return nil
}

// SupervisorScope 督导负责范围查询
func (s *User) SupervisorScope(db *gorm.DB, userID int) (*ScopeResult, error) {
	u, err := s.GetByID(db, userID)
	if err != nil {
		return nil, err
	}
	colleges := model.CollegeInfos(u.UserColleges)
	if colleges == nil {
		colleges = []model.UserCollegeInfo{}
	}
	rooms := roomInfos(u.UserRooms)
	if rooms == nil {
		rooms = []model.UserResearchRoomInfo{}
	}
	return &ScopeResult{
		CollegeIDs: collegeIDsOf(u.UserColleges),
		RoomIDs:    roomIDsOf(u.UserRooms),
		Colleges:   colleges,
		Rooms:      rooms,
	}, nil
}

// UpdateSupervisorScope 覆盖式更新督导负责范围（仅系统管理员）
func (s *User) UpdateSupervisorScope(db *gorm.DB, caller *model.User, userID int, collegeIDs, roomIDs []int) error {
	if !caller.HasRole(model.RoleSystemAdmin) {
		return errors.New("仅系统管理员可修改督导负责范围")
	}
	if _, err := s.GetByID(db, userID); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserCollege{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserRoom{}).Error; err != nil {
			return err
		}
		now := timeNow()
		for _, cid := range collegeIDs {
			if err := tx.Create(&model.UserCollege{UserID: userID, CollegeID: cid, JoinTime: model.LocalTimePtr(now)}).Error; err != nil {
				return err
			}
		}
		for _, rid := range roomIDs {
			if err := tx.Create(&model.UserRoom{UserID: userID, ResearchRoomID: rid, JoinTime: model.LocalTimePtr(now)}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ScopeResult 督导负责范围
type ScopeResult struct {
	CollegeIDs []int                        `json:"supervisor_college_ids"`
	RoomIDs    []int                        `json:"supervisor_research_room_ids"`
	Colleges   []model.UserCollegeInfo      `json:"colleges"`
	Rooms      []model.UserResearchRoomInfo `json:"research_rooms"`
}

func collegeIDsOf(ucs []model.UserCollege) []int {
	out := make([]int, 0, len(ucs))
	for _, uc := range ucs {
		out = append(out, uc.CollegeID)
	}
	return out
}

func roomIDsOf(urs []model.UserRoom) []int {
	out := make([]int, 0, len(urs))
	for _, ur := range urs {
		out = append(out, ur.ResearchRoomID)
	}
	return out
}

func roomInfos(urs []model.UserRoom) []model.UserResearchRoomInfo {
	out := []model.UserResearchRoomInfo{}
	for _, ur := range urs {
		if ur.ResearchRoom == nil {
			continue
		}
		roomID := ur.ResearchRoomID
		collegeID := ur.ResearchRoom.CollegeID
		out = append(out, model.UserResearchRoomInfo{
			RoomID:    &roomID,
			RoomName:  ur.ResearchRoom.Name,
			CollegeID: &collegeID,
			JoinTime:  ur.JoinTime,
		})
	}
	return out
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// timeNow 当前时间指针
func timeNow() time.Time { return time.Now() }

// isAdminUser 是否持有 system_admin 角色
func (s *User) isAdminUser(db *gorm.DB, u *model.User) bool {
	if u.Role == model.RoleSystemAdmin {
		return true
	}
	var cnt int64
	db.Model(&model.UserRole{}).Where("user_id = ? AND role = ?", u.ID, model.RoleSystemAdmin).Count(&cnt)
	return cnt > 0
}

// activeAdminCount 除 excludeID 外启用的 system_admin 数量
func (s *User) activeAdminCount(db *gorm.DB, excludeID int) int64 {
	var cnt int64
	db.Table("user").Where(
		"status = 1 AND id <> ? AND (role = ? OR id IN (SELECT user_id FROM user_role WHERE role = ?))",
		excludeID, model.RoleSystemAdmin, model.RoleSystemAdmin,
	).Count(&cnt)
	return cnt
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// splitInts 解析逗号分隔的 ID 列表
func splitInts(s string) []int {
	var ids []int
	cur := 0
	has := false
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if has {
				ids = append(ids, cur)
			}
			cur = 0
			has = false
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			cur = cur*10 + int(s[i]-'0')
			has = true
		}
	}
	return ids
}
