package service

import (
	"errors"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// PermissionItem 权限码条目
type PermissionItem struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// PermissionGroup 权限码分组
type PermissionGroup struct {
	Group       string           `json:"group"`
	Permissions []PermissionItem `json:"permissions"`
}

// PermissionCatalog 权限码目录（唯一真相源）
var PermissionCatalog = []PermissionGroup{
	{Group: "用户管理", Permissions: []PermissionItem{
		{Code: "user:view", Name: "查看用户"},
		{Code: "user:create", Name: "创建用户"},
		{Code: "user:update", Name: "编辑用户"},
		{Code: "user:delete", Name: "删除用户"},
	}},
	{Group: "组织架构", Permissions: []PermissionItem{
		{Code: "org:view", Name: "查看组织架构"},
		{Code: "campus:manage", Name: "管理校区"},
		{Code: "college:manage", Name: "管理学院"},
		{Code: "research_room:manage", Name: "管理教研室"},
	}},
	{Group: "角色管理", Permissions: []PermissionItem{
		{Code: "role:manage", Name: "管理角色"},
	}},
	{Group: "评教维度", Permissions: []PermissionItem{
		{Code: "dimension:manage", Name: "管理评教维度"},
	}},
	{Group: "评教任务", Permissions: []PermissionItem{
		{Code: "task:view", Name: "查看评教任务"},
		{Code: "task:create", Name: "创建评教任务"},
		{Code: "task:update", Name: "编辑评教任务"},
		{Code: "task:delete", Name: "删除评教任务"},
	}},
	{Group: "评教记录", Permissions: []PermissionItem{
		{Code: "evaluation:view", Name: "查看评教记录"},
		{Code: "evaluation:create", Name: "提交评教"},
		{Code: "evaluation:view_anonymous", Name: "查看匿名评教详情"},
		{Code: "evaluation:view_all", Name: "查看他人评教详情"},
	}},
	{Group: "统计报表", Permissions: []PermissionItem{
		{Code: "stats:view", Name: "查看统计报表"},
	}},
	{Group: "课程表", Permissions: []PermissionItem{
		{Code: "schedule:view", Name: "查看课程表"},
	}},
	{Group: "数据同步", Permissions: []PermissionItem{
		{Code: "sync:execute", Name: "执行数据同步"},
	}},
}

// validPermissionCodes 全部合法权限码集合
func validPermissionCodes() map[string]bool {
	m := map[string]bool{}
	for _, g := range PermissionCatalog {
		for _, p := range g.Permissions {
			m[p.Code] = true
		}
	}
	return m
}

var validDataScopes = map[string]bool{"all": true, "college": true, "self": true}

// RoleService 角色业务
type Role struct{}

func NewRole() *Role { return &Role{} }

// PermissionGroups 返回按分组的权限码目录
func (s *Role) PermissionGroups() []PermissionGroup { return PermissionCatalog }

// RoleWithCount 角色列表项（附带使用人数）
type RoleWithCount struct {
	model.Role
	UserCount int `json:"user_count"`
}

// List 角色列表；onlyEnabled 时仅返回启用的
func (s *Role) List(db *gorm.DB, onlyEnabled bool) ([]RoleWithCount, error) {
	q := db.Model(&model.Role{})
	if onlyEnabled {
		q = q.Where("status = 1")
	}
	var roles []model.Role
	if err := q.Order("level ASC, id ASC").Find(&roles).Error; err != nil {
		return nil, err
	}

	// 统计角色使用人数
	type row struct {
		Role string
		Cnt  int64
	}
	var rows []row
	db.Model(&model.UserRole{}).Select("role, COUNT(*) AS cnt").Group("role").Scan(&rows)
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Role] = int(r.Cnt)
	}

	out := make([]RoleWithCount, 0, len(roles))
	for _, r := range roles {
		out = append(out, RoleWithCount{Role: r, UserCount: counts[r.Code]})
	}
	return out, nil
}

// Get 角色详情（附带使用人数，对齐旧端 get_role_with_user_count）
func (s *Role) Get(db *gorm.DB, id int) (*RoleWithCount, error) {
	var role model.Role
	if err := db.First(&role, id).Error; err != nil {
		return nil, errNotFound("角色不存在")
	}
	var cnt int64
	db.Model(&model.UserRole{}).Where("role = ?", role.Code).Count(&cnt)
	return &RoleWithCount{Role: role, UserCount: int(cnt)}, nil
}

// RoleParams 角色 create/update 参数（指针字段可选）
type RoleParams struct {
	Name        *string   `json:"name"`
	Code        *string   `json:"code"`
	Description *string   `json:"description"`
	Level       *int      `json:"level"`
	Permissions *[]string `json:"permissions"`
	DataScope   *string   `json:"data_scope"`
	Status      *int16    `json:"status"`
}

func (s *Role) validateParams(p RoleParams, isCreate bool) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if p.Name != nil {
		if *p.Name == "" {
			return nil, errors.New("角色名称不能为空")
		}
		updates["name"] = *p.Name
	}
	if p.Code != nil {
		if *p.Code == "" {
			return nil, errors.New("角色编码不能为空")
		}
		updates["code"] = *p.Code
	}
	if p.Description != nil {
		updates["description"] = *p.Description
	}
	if p.Level != nil {
		updates["level"] = *p.Level
	}
	if p.Permissions != nil {
		valid := validPermissionCodes()
		for _, c := range *p.Permissions {
			if !valid[c] {
				return nil, errors.New("非法权限码: " + c)
			}
		}
		updates["permissions"] = *p.Permissions
	}
	if p.DataScope != nil {
		if !validDataScopes[*p.DataScope] {
			return nil, errors.New("data_scope 必须为 all/college/self")
		}
		updates["data_scope"] = *p.DataScope
	}
	if p.Status != nil {
		updates["status"] = *p.Status
	}
	if isCreate {
		for _, k := range []string{"name", "code", "permissions", "data_scope"} {
			if _, ok := updates[k]; !ok {
				return nil, errors.New("缺少必填字段: " + k)
			}
		}
	}
	return updates, nil
}

// Create 新增角色
func (s *Role) Create(db *gorm.DB, p RoleParams) (*RoleWithCount, error) {
	updates, err := s.validateParams(p, true)
	if err != nil {
		return nil, err
	}
	var cnt int64
	db.Model(&model.Role{}).Where("code = ?", updates["code"]).Count(&cnt)
	if cnt > 0 {
		return nil, errors.New("角色编码已存在")
	}
	role := model.Role{
		Name:        updates["name"].(string),
		Code:        updates["code"].(string),
		Description: strVal(updates, "description"),
		Level:       intVal(updates, "level", 100),
		DataScope:   updates["data_scope"].(string),
		Status:      1,
	}
	if v, ok := updates["permissions"].([]string); ok {
		role.Permissions = toRawJSON(v)
	}
	if v, ok := updates["status"].(int16); ok {
		role.Status = v
	}
	if err := db.Create(&role).Error; err != nil {
		return nil, err
	}
	return &RoleWithCount{Role: role, UserCount: 0}, nil
}

// Update 更新角色
func (s *Role) Update(db *gorm.DB, id int, p RoleParams) (*RoleWithCount, error) {
	var role model.Role
	if err := db.First(&role, id).Error; err != nil {
		return nil, errors.New("角色不存在")
	}
	updates, err := s.validateParams(p, false)
	if err != nil {
		return nil, err
	}
	// 内置角色不可禁用
	if role.IsSystem && p.Status != nil && *p.Status == 0 {
		return nil, errors.New("内置角色不可禁用")
	}
	if v, ok := updates["permissions"].([]string); ok {
		raw, _ := marshalJSON(v)
		updates["permissions"] = raw
	} else {
		delete(updates, "permissions") // 避免以 []string 直写 JSON 列
	}
	if len(updates) > 0 {
		if err := db.Model(&role).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return s.Get(db, id)
}

// Delete 删除角色
func (s *Role) Delete(db *gorm.DB, id int) error {
	var role model.Role
	if err := db.First(&role, id).Error; err != nil {
		return errors.New("角色不存在")
	}
	if role.IsSystem {
		return errors.New("内置角色不可删除")
	}
	var cnt int64
	db.Model(&model.UserRole{}).Where("role = ?", role.Code).Count(&cnt)
	if cnt > 0 {
		return errors.New("该角色仍被用户使用，禁止删除")
	}
	return db.Delete(&model.Role{}, id).Error
}

// ---------- 小工具 ----------

func strVal(m map[string]interface{}, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func intVal(m map[string]interface{}, k string, def int) int {
	if v, ok := m[k].(int); ok {
		return v
	}
	return def
}
