package handler

import (
	"strconv"

	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RoleHandler 角色管理接口
type RoleHandler struct {
	db  *gorm.DB
	svc *service.Role
}

func NewRoleHandler(db *gorm.DB, svc *service.Role) *RoleHandler {
	return &RoleHandler{db: db, svc: svc}
}

// PermissionGroups 权限码目录
func (h *RoleHandler) PermissionGroups(c *gin.Context) {
	response.OK(c, h.svc.PermissionGroups())
}

// canManageRole 是否可管理角色（role:manage；系统管理员恒有全部权限）
func canManageRole(c *gin.Context, db *gorm.DB) bool {
	u := middleware.CurrentUser(c)
	if u.HasRole(model.RoleSystemAdmin) {
		return true
	}
	for _, p := range u.Permissions(db) {
		if p == "role:manage" {
			return true
		}
	}
	return false
}

// roleViewItem 角色响应视图。
// 无 role:manage 的调用者（如学院管理员）不返回 permissions/data_scope/level 等角色配置——
// 不可管理的角色配置也不作为可读数据返回，只保留展示所需的名称/编码等字段；
// assignable 标记该角色是否可由调用者分配给用户（与写侧 checkRoleAssignable 同一口径），
// 前端「新增/编辑用户」的角色下拉据此收敛，不展示不可分配的角色。
func roleViewItem(c *gin.Context, r service.RoleWithCount, manage bool) gin.H {
	allowed := service.AssignableRoles(middleware.CurrentUser(c))
	item := gin.H{
		"id":          r.ID,
		"name":        r.Name,
		"code":        r.Code,
		"description": r.Description,
		"status":      r.Status,
		"is_system":   r.IsSystem,
		"user_count":  r.UserCount,
		"assignable":  allowed == nil || allowed[r.Code],
	}
	if manage {
		item["level"] = r.Level
		item["permissions"] = r.Permissions
		item["data_scope"] = r.DataScope
	}
	return item
}

// List 角色列表
func (h *RoleHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)
	onlyEnabled := !u.HasRole("system_admin")
	list, err := h.svc.List(h.db, onlyEnabled)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	manage := canManageRole(c, h.db)
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, roleViewItem(c, list[i], manage))
	}
	_, size := pageOfDefault(c, 100)
	response.OK(c, pageData(c, out, int64(len(out)), size))
}

// Get 角色详情
func (h *RoleHandler) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	role, err := h.svc.Get(h.db, id)
	if err != nil {
		response.Fail(c, 404, "角色不存在")
		return
	}
	response.OK(c, roleViewItem(c, *role, canManageRole(c, h.db)))
}

// Create 新增角色
func (h *RoleHandler) Create(c *gin.Context) {
	var p service.RoleParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	role, err := h.svc.Create(h.db, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "create", &role.ID)
	response.OKMsg(c, "创建成功", role)
}

// Update 编辑角色
func (h *RoleHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p service.RoleParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	role, err := h.svc.Update(h.db, id, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "update", &role.ID)
	// 角色权限/状态变更后：失效持有该角色的用户快照，旧权限在快照 TTL 内收敛
	h.invalidateRoleUsers(c, role.Code)
	response.OKMsg(c, "更新成功", role)
}

// invalidateRoleUsers 失效持有某角色的用户认证快照（user_role 关联 + legacy role 字段）。
func (h *RoleHandler) invalidateRoleUsers(c *gin.Context, code string) {
	var ids []int
	if err := h.db.Model(&model.UserRole{}).Where("role = ?", code).Pluck("user_id", &ids).Error; err != nil {
		return
	}
	var legacy []int
	if err := h.db.Model(&model.User{}).Where("role = ?", code).Pluck("id", &legacy).Error; err == nil {
		ids = append(ids, legacy...)
	}
	for _, uid := range ids {
		invalidateUserAuth(c, uid)
	}
}

// Delete 删除角色
func (h *RoleHandler) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.Delete(h.db, id); err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "delete", &id)
	response.OKMsg(c, "删除成功", nil)
}

func (h *RoleHandler) log(c *gin.Context, opType string, id *int) {
	u := middleware.CurrentUser(c)
	service.LogRecord(h.db, &u.ID, u.Username, opType, "role", id, "role", nil)
}
