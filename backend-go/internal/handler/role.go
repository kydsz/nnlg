package handler

import (
	"strconv"

	"backend-go/internal/middleware"
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

// List 角色列表
func (h *RoleHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)
	onlyEnabled := !u.HasRole("system_admin")
	list, err := h.svc.List(h.db, onlyEnabled)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	_, size := pageOfDefault(c, 100)
	response.OK(c, pageData(c, list, int64(len(list)), size))
}

// Get 角色详情
func (h *RoleHandler) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	role, err := h.svc.Get(h.db, id)
	if err != nil {
		response.Fail(c, 404, "角色不存在")
		return
	}
	response.OK(c, role)
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
	response.OKMsg(c, "更新成功", role)
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
