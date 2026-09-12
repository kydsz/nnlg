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

// Org 组织架构接口（校区/学院/教研室）
type Org struct {
	db  *gorm.DB
	svc *service.Org
}

func NewOrg(db *gorm.DB, svc *service.Org) *Org {
	return &Org{db: db, svc: svc}
}

// ---------- 校区 ----------

func (h *Org) CampusList(c *gin.Context) {
	list, err := h.svc.CampusList(h.db)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	counts := h.svc.CampusCollegeCount(h.db)
	out := []gin.H{}
	for _, cp := range list {
		out = append(out, gin.H{
			"id": cp.ID, "name": cp.Name, "sort_order": cp.SortOrder,
			"status": cp.Status, "college_count": counts[cp.ID], "create_time": FTime(cp.CreateTime),
		})
	}
	_, size := pageOfDefault(c, 100)
	response.OK(c, pageData(c, out, int64(len(out)), size))
}

type campusReq struct {
	Name      string `json:"name"`
	SortOrder *int   `json:"sort_order"`
	Status    *int16 `json:"status"`
}

func (h *Org) CampusCreate(c *gin.Context) {
	var p campusReq
	if err := c.ShouldBindJSON(&p); err != nil || p.Name == "" {
		badReq(c, "校区名称不能为空")
		return
	}
	cp, err := h.svc.CampusCreate(h.db, p.Name, deref(p.SortOrder), 1)
	if err != nil {
		serverErr(c, "创建失败")
		return
	}
	h.log(c, "create", "campus", &cp.ID)
	invalidateAllStats(c)
	response.OKMsg(c, "创建成功", cp)
}

func (h *Org) CampusUpdate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p campusReq
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	cp, err := h.svc.CampusUpdate(h.db, id, &p.Name, p.SortOrder, p.Status)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "update", "campus", &cp.ID)
	invalidateAllStats(c)
	response.OKMsg(c, "更新成功", cp)
}

func (h *Org) CampusDelete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.CampusDelete(h.db, id); err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "delete", "campus", &id)
	invalidateAllStats(c)
	response.OKMsg(c, "删除成功", nil)
}

// ---------- 学院 ----------

func (h *Org) CollegeList(c *gin.Context) {
	u := middleware.CurrentUser(c)
	// 学院管理员仅可见自己范围；其余角色全量
	var scope []int
	if u.HasAnyRole(model.RoleCollegeAdmin, model.RoleSchoolAdmin, model.RoleCollegeSupervisor) {
		scope = service.AccessibleCollegeIDs(u)
	}
	status := qInt16(c, "status")
	list, err := h.svc.CollegeList(h.db, c.Query("keyword"), c.Query("campus_id"), status, scope)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	rooms, users := h.svc.CollegeCounts(h.db)
	campusNames := h.campusNameMap()
	out := []gin.H{}
	for _, col := range list {
		out = append(out, gin.H{
			"id": col.ID, "code": col.Code, "name": col.Name,
			"campus_id": col.CampusID, "campus_name": campusName(col.CampusID, campusNames),
			"sort_order": col.SortOrder, "status": col.Status,
			"research_room_count": rooms[col.ID], "user_count": users[col.ID],
			"create_time": FTime(col.CreateTime),
		})
	}
	_, size := pageOfDefault(c, 100)
	response.OK(c, pageData(c, out, int64(len(out)), size))
}

type collegeReq struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	CampusID  *int   `json:"campus_id"`
	SortOrder *int   `json:"sort_order"`
	Status    *int16 `json:"status"`
}

func (h *Org) CollegeCreate(c *gin.Context) {
	var p collegeReq
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	col, err := h.svc.CollegeCreate(h.db, p.Code, p.Name, p.CampusID, deref(p.SortOrder), 1)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "create", "college", &col.ID)
	invalidateAllStats(c)
	campusNames := h.campusNameMap()
	response.OKMsg(c, "创建成功", gin.H{
		"id": col.ID, "code": col.Code, "name": col.Name,
		"campus_id": col.CampusID, "campus_name": campusName(col.CampusID, campusNames),
		"sort_order": col.SortOrder, "status": col.Status,
		"create_time": col.CreateTime,
	})
}

func (h *Org) CollegeUpdate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p collegeReq
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	// 对齐旧端 exclude_unset 语义：未提供的字段不更新
	var codePtr, namePtr *string
	if p.Code != "" {
		codePtr = &p.Code
	}
	if p.Name != "" {
		namePtr = &p.Name
	}
	col, err := h.svc.CollegeUpdate(h.db, id, codePtr, namePtr, p.CampusID, p.SortOrder, p.Status)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "update", "college", &col.ID)
	invalidateAllStats(c)
	campusNames := h.campusNameMap()
	response.OKMsg(c, "更新成功", gin.H{
		"id": col.ID, "code": col.Code, "name": col.Name,
		"campus_id": col.CampusID, "campus_name": campusName(col.CampusID, campusNames),
		"sort_order": col.SortOrder, "status": col.Status,
		"update_time": col.UpdateTime,
	})
}

func (h *Org) CollegeDelete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.CollegeDelete(h.db, id); err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "delete", "college", &id)
	invalidateAllStats(c)
	response.OKMsg(c, "删除成功", nil)
}

// ---------- 教研室 ----------

func (h *Org) RoomList(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var scope []int
	if u.HasAnyRole(model.RoleCollegeAdmin, model.RoleSchoolAdmin, model.RoleCollegeSupervisor) {
		scope = service.AccessibleCollegeIDs(u)
	}
	status := qInt16(c, "status")
	list, err := h.svc.RoomList(h.db, c.Query("keyword"), c.Query("college_id"), status, scope)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	// 用户数
	type row struct {
		RoomID int
		Cnt    int64
	}
	var rows []row
	h.db.Table("user").Select("research_room_id AS room_id, COUNT(*) AS cnt").
		Where("status = 1 AND research_room_id IS NOT NULL").Group("research_room_id").Scan(&rows)
	userCounts := map[int]int{}
	for _, r := range rows {
		userCounts[r.RoomID] = int(r.Cnt)
	}
	// 学院/校区名称
	collegeNames := h.collegeNameMap()
	campusNames := h.campusNameMap()
	collegeCampus := h.collegeCampusMap()

	out := []gin.H{}
	for _, r := range list {
		campusID := collegeCampus[r.CollegeID]
		// 字段对齐旧端列表（无 code）
		out = append(out, gin.H{
			"id": r.ID, "name": r.Name,
			"college_id": r.CollegeID, "college_name": collegeNames[r.CollegeID],
			"campus_id": campusID, "campus_name": campusName(campusID, campusNames),
			"status": r.Status, "user_count": userCounts[r.ID], "create_time": FTime(r.CreateTime),
		})
	}
	_, size := pageOfDefault(c, 100)
	response.OK(c, pageData(c, out, int64(len(out)), size))
}

type roomReq struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	CollegeID *int   `json:"college_id"`
	Status    *int16 `json:"status"`
}

func (h *Org) RoomCreate(c *gin.Context) {
	var p roomReq
	if err := c.ShouldBindJSON(&p); err != nil || p.CollegeID == nil {
		badReq(c, "请求参数错误")
		return
	}
	room, err := h.svc.RoomCreate(h.db, p.Code, p.Name, *p.CollegeID, 1)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "create", "research_room", &room.ID)
	collegeNames := h.collegeNameMap()
	// 响应字段对齐旧端（无 code，含 college_name）
	response.OKMsg(c, "创建成功", gin.H{
		"id": room.ID, "name": room.Name,
		"college_id": room.CollegeID, "college_name": collegeNames[room.CollegeID],
		"status": room.Status, "create_time": room.CreateTime,
	})
}

func (h *Org) RoomUpdate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p roomReq
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	// 对齐旧端 exclude_unset 语义：未提供的字段不更新
	var codePtr, namePtr *string
	if p.Code != "" {
		codePtr = &p.Code
	}
	if p.Name != "" {
		namePtr = &p.Name
	}
	room, err := h.svc.RoomUpdate(h.db, id, codePtr, namePtr, p.CollegeID, p.Status)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "update", "research_room", &room.ID)
	collegeNames := h.collegeNameMap()
	response.OKMsg(c, "更新成功", gin.H{
		"id": room.ID, "name": room.Name,
		"college_id": room.CollegeID, "college_name": collegeNames[room.CollegeID],
		"status": room.Status, "update_time": room.UpdateTime,
	})
}

func (h *Org) RoomDelete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.RoomDelete(h.db, id); err != nil {
		badReq(c, err.Error())
		return
	}
	h.log(c, "delete", "research_room", &id)
	response.OKMsg(c, "删除成功", nil)
}

// CampusGet 校区详情
func (h *Org) CampusGet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cp model.Campus
	if err := h.db.First(&cp, id).Error; err != nil {
		response.Fail(c, 404, "校区不存在")
		return
	}
	counts := h.svc.CampusCollegeCount(h.db)
	response.OK(c, gin.H{
		"id": cp.ID, "name": cp.Name, "sort_order": cp.SortOrder,
		"status": cp.Status, "college_count": counts[cp.ID],
		"create_time": FTime(cp.CreateTime), "update_time": FTime(cp.UpdateTime),
	})
}

// CollegeGet 学院详情（学院管理员/校级管理员仅可查看自己范围）
func (h *Org) CollegeGet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var col model.College
	if err := h.db.First(&col, id).Error; err != nil {
		response.Fail(c, 404, "学院不存在")
		return
	}
	u := middleware.CurrentUser(c)
	if u.HasAnyRole(model.RoleCollegeAdmin, model.RoleSchoolAdmin) && !u.HasRole(model.RoleSystemAdmin) {
		if !u.HasCollegeID(id) {
			forbidden(c, "无权查看该学院")
			return
		}
	}
	campusNames := h.campusNameMap()
	rooms, users := h.svc.CollegeCounts(h.db)
	response.OK(c, gin.H{
		"id": col.ID, "code": col.Code, "name": col.Name,
		"campus_id": col.CampusID, "campus_name": campusName(col.CampusID, campusNames),
		"sort_order": col.SortOrder, "status": col.Status,
		"research_room_count": rooms[col.ID], "user_count": users[col.ID],
		"create_time": FTime(col.CreateTime), "update_time": FTime(col.UpdateTime),
	})
}

// RoomGet 教研室详情
func (h *Org) RoomGet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var r model.ResearchRoom
	if err := h.db.First(&r, id).Error; err != nil {
		response.Fail(c, 404, "教研室不存在")
		return
	}
	var userCnt int64
	h.db.Table("user").Where("status = 1 AND research_room_id = ?", id).Count(&userCnt)
	collegeNames := h.collegeNameMap()
	campusNames := h.campusNameMap()
	collegeCampus := h.collegeCampusMap()
	campusID := collegeCampus[r.CollegeID]
	// 对齐旧端：详情无 code 字段
	response.OK(c, gin.H{
		"id": r.ID, "name": r.Name,
		"college_id": r.CollegeID, "college_name": collegeNames[r.CollegeID],
		"campus_id": campusID, "campus_name": campusName(campusID, campusNames),
		"status": r.Status, "user_count": userCnt,
		"create_time": FTime(r.CreateTime), "update_time": FTime(r.UpdateTime),
	})
}

// ---------- 名称映射辅助 ----------

func (h *Org) campusNameMap() map[int]string {
	var list []model.Campus
	h.db.Find(&list)
	m := map[int]string{}
	for _, cp := range list {
		m[cp.ID] = cp.Name
	}
	return m
}

func (h *Org) collegeNameMap() map[int]string {
	var list []model.College
	h.db.Find(&list)
	m := map[int]string{}
	for _, col := range list {
		m[col.ID] = col.Name
	}
	return m
}

func (h *Org) collegeCampusMap() map[int]*int {
	var list []model.College
	h.db.Find(&list)
	m := map[int]*int{}
	for _, col := range list {
		m[col.ID] = col.CampusID
	}
	return m
}

func campusName(id *int, m map[int]string) interface{} {
	if id == nil {
		return nil
	}
	if n, ok := m[*id]; ok {
		return n
	}
	return nil
}

func (h *Org) log(c *gin.Context, opType, module string, id *int) {
	u := middleware.CurrentUser(c)
	service.LogRecord(h.db, &u.ID, u.Username, opType, module, id, module, nil)
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
