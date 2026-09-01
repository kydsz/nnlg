package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// User 用户管理接口
type User struct {
	db  *gorm.DB
	svc *service.User
}

func NewUser(db *gorm.DB, svc *service.User) *User {
	return &User{db: db, svc: svc}
}

// List 用户分页列表（权限码 user:view）
func (h *User) List(c *gin.Context) {
	page, pageSize := pageOf(c)

	params := service.UserParams{
		Page:           page,
		PageSize:       pageSize,
		Keyword:        c.Query("keyword"),
		Role:           c.Query("role"),
		CollegeID:      c.Query("college_id"),
		ResearchRoomID: c.Query("research_room_id"),
		NoCollege:      c.Query("no_college") == "true" || c.Query("no_college") == "1",
		NoResearchRoom: c.Query("no_research_room") == "true" || c.Query("no_research_room") == "1",
		OrderBy:        c.Query("order_by"),
		OrderDir:       c.Query("order"),
	}
	if v, ok := c.GetQuery("status"); ok {
		if s, err := strconv.Atoi(v); err == nil {
			params.Status = &s
		}
	}

	users, total, err := h.svc.List(h.db, params)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}

	list := []gin.H{}
	for i := range users {
		list = append(list, userListItem(&users[i]))
	}

	response.OK(c, gin.H{
		"list": list, "total": total, "page": page, "page_size": pageSize,
	})
}

// Get 用户详情
func (h *User) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return
	}
	u, err := h.svc.GetByID(h.db, id)
	if err != nil {
		response.Fail(c, http.StatusNotFound, err.Error())
		return
	}
	response.OK(c, userDetailItem(u))
}

// Create 新增用户
func (h *User) Create(c *gin.Context) {
	var p service.CreateUserParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	caller := middleware.CurrentUser(c)
	u, err := h.svc.Create(h.db, caller, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "create", "user", &u.ID, "user",
		map[string]interface{}{"user_no": u.UserNo, "username": u.Username})
	// 旧端 create 的 message 也为 "更新成功"，此处保持一致
	response.OKMsg(c, "更新成功", userDetailItem(u))
}

// Update 编辑用户
func (h *User) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return
	}
	var p service.UpdateUserParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	caller := middleware.CurrentUser(c)
	u, err := h.svc.Update(h.db, caller, id, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &u.ID, "user", nil)
	invalidateUserAuth(c, id)
	if (p.Status != nil && *p.Status == 0) || (p.Password != nil && *p.Password != "") {
		invalidateUserSession(c, id)
	}
	response.OKMsg(c, "更新成功", userDetailItem(u))
}

// Delete 删除用户
func (h *User) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.Delete(h.db, id); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "delete", "user", &id, "user", nil)
	invalidateUserSession(c, id)
	response.OKMsg(c, "删除成功", nil)
}

// UpdateMyResearchRoom 教师自助修改主教研室
func (h *User) UpdateMyResearchRoom(c *gin.Context) {
	var p struct {
		ResearchRoomID *int `json:"research_room_id"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)
	roomName, err := h.svc.UpdateMyResearchRoom(h.db, u, p.ResearchRoomID)
	if err != nil {
		svcErr(c, err)
		return
	}
	service.LogRecord(h.db, &u.ID, u.Username, "update", "user", &u.ID, "user",
		map[string]interface{}{"action": "update_my_research_room", "research_room_id": p.ResearchRoomID})
	invalidateUserAuth(c, u.ID)
	// 旧端 message："教研室设置成功"
	response.OKMsg(c, "教研室设置成功", gin.H{
		"research_room_id":   p.ResearchRoomID,
		"research_room_name": roomName,
	})
}
func (h *User) UpdateStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return
	}
	status, err := strconv.Atoi(c.Query("status"))
	if err != nil || (status != 0 && status != 1) {
		badReq(c, "status 必须为 0(禁用) 或 1(启用)")
		return
	}
	caller := middleware.CurrentUser(c)
	u, err := h.svc.UpdateStatus(h.db, caller, id, status)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "toggle_status", "status": status})
	invalidateUserAuth(c, id)
	if status == 0 {
		invalidateUserSession(c, id)
	}
	response.OKMsg(c, "操作成功", userDetailItem(u))
}

// BatchStatus 批量启用/禁用
func (h *User) BatchStatus(c *gin.Context) {
	var p struct {
		IDs    []int `json:"ids"`
		Status *int  `json:"status"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || len(p.IDs) == 0 || p.Status == nil {
		badReq(c, "请提供用户ID列表和状态")
		return
	}
	if *p.Status != 0 && *p.Status != 1 {
		badReq(c, "status 必须为 0(禁用) 或 1(启用)")
		return
	}
	caller := middleware.CurrentUser(c)
	success, failed, changed := h.svc.BatchStatus(h.db, caller, p.IDs, *p.Status)
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", nil, "user",
		map[string]interface{}{"action": "batch_status", "ids": p.IDs, "status": *p.Status})
	for _, uid := range changed {
		invalidateUserAuth(c, uid)
		if *p.Status == 0 {
			invalidateUserSession(c, uid)
		}
	}
	response.OK(c, gin.H{"success": success, "failed": failed, "total": len(p.IDs)})
}

// AddRole 添加用户角色
func (h *User) AddRole(c *gin.Context) {
	id, role, ok := userIDRoleOf(c)
	if !ok {
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.AddRole(h.db, caller, id, role); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "add_role", "role": role})
	invalidateUserAuth(c, id)
	response.OKMsg(c, "添加成功", gin.H{"user_id": id, "role": role, "role_name": model.RoleName(role)})
}

// RemoveRole 移除用户角色
func (h *User) RemoveRole(c *gin.Context) {
	id, role, ok := userIDRoleOf(c)
	if !ok {
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.RemoveRole(h.db, caller, id, role); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "remove_role", "role": role})
	invalidateUserAuth(c, id)
	response.OKMsg(c, "移除成功", nil)
}

// AddCollege 加入督导学院
func (h *User) AddCollege(c *gin.Context) {
	id, cid, ok := userIDExtOf(c, "college_id")
	if !ok {
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.AddCollege(h.db, caller, id, cid); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "add_college", "college_id": cid})
	invalidateUserAuth(c, id)
	response.OKMsg(c, "添加成功", gin.H{"user_id": id, "college_id": cid})
}

// RemoveCollege 移出督导学院
func (h *User) RemoveCollege(c *gin.Context) {
	id, cid, ok := userIDExtOf(c, "college_id")
	if !ok {
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.RemoveCollege(h.db, caller, id, cid); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "remove_college", "college_id": cid})
	invalidateUserAuth(c, id)
	response.OKMsg(c, "移除成功", nil)
}

// AddRoom 加入教研室
func (h *User) AddRoom(c *gin.Context) {
	id, rid, ok := userIDExtOf(c, "room_id")
	if !ok {
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.AddRoom(h.db, caller, id, rid); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "add_research_room", "research_room_id": rid})
	invalidateUserAuth(c, id)
	response.OKMsg(c, "添加成功", gin.H{"user_id": id, "research_room_id": rid})
}

// RemoveRoom 移出教研室
func (h *User) RemoveRoom(c *gin.Context) {
	id, rid, ok := userIDExtOf(c, "room_id")
	if !ok {
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.RemoveRoom(h.db, caller, id, rid); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "remove_research_room", "research_room_id": rid})
	invalidateUserAuth(c, id)
	response.OKMsg(c, "移除成功", nil)
}

// GetSupervisorScope 查询督导负责范围
func (h *User) GetSupervisorScope(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return
	}
	scope, err := h.svc.SupervisorScope(h.db, id)
	if err != nil {
		response.Fail(c, http.StatusNotFound, err.Error())
		return
	}
	response.OK(c, scope)
}

// UpdateSupervisorScope 覆盖式更新督导负责范围
func (h *User) UpdateSupervisorScope(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return
	}
	// 旧端从 body 取 college_ids / research_room_ids（元素可能是 int 或 list）
	var raw struct {
		CollegeIDs json.RawMessage `json:"college_ids"`
		RoomIDs    json.RawMessage `json:"research_room_ids"`
	}
	if err := c.ShouldBindJSON(&raw); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	collegeIDs, err := intListJSON(raw.CollegeIDs)
	if err != nil {
		serverErr(c, "更新负责范围失败")
		return
	}
	roomIDs, err := intListJSON(raw.RoomIDs)
	if err != nil {
		serverErr(c, "更新负责范围失败")
		return
	}
	caller := middleware.CurrentUser(c)
	if err := h.svc.UpdateSupervisorScope(h.db, caller, id, collegeIDs, roomIDs); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &caller.ID, caller.Username, "update", "user", &id, "user",
		map[string]interface{}{"action": "update_supervisor_scope"})
	u, _ := h.svc.GetByID(h.db, id)
	if u == nil {
		serverErr(c, "更新负责范围失败")
		return
	}
	invalidateUserAuth(c, id)
	response.OKMsg(c, "更新成功", userDetailItem(u))
}

// intListJSON 解析 int / []int / null 形态的 JSON 字段；解析失败返回错误（对齐旧端 500）
func intListJSON(raw json.RawMessage) ([]int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var single int
	if err := json.Unmarshal(raw, &single); err == nil {
		return []int{single}, nil
	}
	var list []int
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	return nil, errors.New("invalid int list")
}

// userIDRoleOf 解析 /:id/roles/:role 路径参数
func userIDRoleOf(c *gin.Context) (int, string, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return 0, "", false
	}
	role := c.Param("role")
	if role == "" {
		badReq(c, "无效的角色编码")
		return 0, "", false
	}
	return id, role, true
}

// userIDExtOf 解析 /:id/{key} 路径参数中的数字 ID
func userIDExtOf(c *gin.Context, key string) (int, int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的用户 ID")
		return 0, 0, false
	}
	extID, err := strconv.Atoi(c.Param(key))
	if err != nil {
		badReq(c, "无效的 ID")
		return 0, 0, false
	}
	return id, extID, true
}

// userListItem 组装用户列表项（字段与旧端 _build_user_list_response 一致）
func userListItem(u *model.User) gin.H {
	return gin.H{
		"id":                             u.ID,
		"user_no":                        u.UserNo,
		"username":                       u.Username,
		"role":                           u.Role,
		"role_name":                      model.RoleName(u.Role),
		"roles":                          dedupeStrings(u.RoleCodes()),
		"user_roles":                     userRolesInfo(u),
		"college_id":                     u.CollegeID,
		"college_name":                   collegeName(u.College),
		"research_room_id":               u.ResearchRoomID,
		"research_room_name":             roomName(u.ResearchRoom),
		"supervisor_college_count":       len(u.UserColleges),
		"supervisor_research_room_count": len(u.UserRooms),
		"colleges":                       userCollegeInfos(u),
		"research_rooms":                 userRoomInfos(u),
		"status":                         u.Status,
		"last_login_time":                u.LastLoginTime,
		"create_time":                    u.CreateTime,
	}
}

// userDetailItem 组装用户详情项（字段与旧端 _build_user_detail_response 一致）
func userDetailItem(u *model.User) gin.H {
	return gin.H{
		"id":                        u.ID,
		"user_no":                   u.UserNo,
		"username":                  u.Username,
		"role":                      u.Role,
		"role_name":                 model.RoleName(u.Role),
		"roles":                     userRolesInfo(u),
		"status":                    u.Status,
		"college_id":                u.CollegeID,
		"research_room_id":          u.ResearchRoomID,
		"supervisor_colleges":       userCollegeInfos(u),
		"supervisor_research_rooms": userRoomInfos(u),
		"last_login_time":           u.LastLoginTime,
		"create_time":               u.CreateTime,
		"update_time":               u.UpdateTime,
	}
}

// userRolesInfo 角色 [{role, role_name, assign_time}]
func userRolesInfo(u *model.User) []model.UserRoleInfo {
	out := []model.UserRoleInfo{}
	for _, ur := range u.UserRoles {
		out = append(out, model.UserRoleInfo{
			Role:       ur.Role,
			RoleName:   model.RoleName(ur.Role),
			AssignTime: ur.AssignTime,
		})
	}
	return out
}

// userCollegeInfos 督导学院 [{college_id, college_name, college_code, join_time}]
func userCollegeInfos(u *model.User) []model.UserCollegeInfo {
	out := model.CollegeInfos(u.UserColleges)
	if out == nil {
		out = []model.UserCollegeInfo{}
	}
	return out
}

// userRoomInfos 督导教研室 [{research_room_id, research_room_name, college_id, college_name, join_time}]
func userRoomInfos(u *model.User) []gin.H {
	out := []gin.H{}
	for _, ur := range u.UserRooms {
		if ur.ResearchRoom == nil {
			continue
		}
		out = append(out, gin.H{
			"research_room_id":   ur.ResearchRoomID,
			"research_room_name": ur.ResearchRoom.Name,
			"college_id":         ur.ResearchRoom.CollegeID,
			"college_name":       collegeName(ur.ResearchRoom.College),
			"join_time":          ur.JoinTime,
		})
	}
	return out
}

// dedupeStrings 去重
func dedupeStrings(codes []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range codes {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}
