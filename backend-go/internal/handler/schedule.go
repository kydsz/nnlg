package handler

import (
	"strconv"
	"time"

	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Schedule 课表接口
type Schedule struct {
	db  *gorm.DB
	svc *service.Schedule
}

func NewSchedule(db *gorm.DB) *Schedule {
	return &Schedule{db: db, svc: service.NewSchedule()}
}

// List 课表分页（响应仅含 list，对齐旧端）
func (h *Schedule) List(c *gin.Context) {
	page, pageSize := pageOf(c)
	f := service.ScheduleFilters{
		TeacherID: qInt(c, "teacher_id"), Semester: c.Query("semester"),
		TeacherName: c.Query("teacher_name"), IsCurrent: true,
	}
	if f.TeacherName == "" {
		f.TeacherName = c.Query("keyword") // 兼容旧版 Go 参数名
	}

	u := middleware.CurrentUser(c)
	// 权限控制：普通老师只能查看本学院教师的课表（对齐旧端）
	if u.Role == "teacher" {
		collegeIDs := h.userCollegeIDs(u.ID)
		if len(collegeIDs) == 0 && u.CollegeID != nil {
			collegeIDs = []int{*u.CollegeID}
		}
		if len(collegeIDs) == 0 {
			response.OK(c, gin.H{"list": []gin.H{}})
			return
		}
		var tids []int
		h.db.Model(&model.User{}).Where("college_id IN ?", collegeIDs).Pluck("id", &tids)
		f.TeacherIDs = tids
		f.TeacherID = nil // 教师角色下忽略 teacher_id 筛选（对齐旧端分支）
	}

	list, _, err := h.svc.List(h.db, f, page, pageSize)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	out := []gin.H{}
	for _, sc := range list {
		out = append(out, gin.H{
			"id": sc.ID, "teacher_id": sc.TeacherID, "teacher_name": sc.TeacherName,
			"semester": sc.Semester, "version": sc.Version, "is_current": sc.IsCurrent,
			"crawl_time": sc.CrawlTime, "change_summary": sc.ChangeSummary,
			"create_time": sc.CreateTime, "update_time": sc.UpdateTime,
		})
	}
	response.OK(c, gin.H{"list": out})
}

// userCollegeIDs 用户关联的全部学院 ID
func (h *Schedule) userCollegeIDs(userID int) []int {
	var ids []int
	h.db.Model(&model.UserCollege{}).Where("user_id = ?", userID).Pluck("college_id", &ids)
	return ids
}

// Detail 课表详情（扁平结构，含明细）
func (h *Schedule) Detail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		invalidParam(c, "path", "schedule_id", c.Param("id"),
			"Input should be a valid integer, unable to parse string as an integer")
		return
	}
	item, err := h.svc.Detail(h.db, id)
	if err != nil {
		response.Fail(c, 404, err.Error())
		return
	}
	response.OK(c, item)
}

// ByTeacher 按教师查询（无数据返回空壳，对齐旧端）
func (h *Schedule) ByTeacher(c *gin.Context) {
	teacherID, err := strconv.Atoi(c.Param("teacher_id"))
	if err != nil {
		invalidParam(c, "path", "teacher_id", c.Param("teacher_id"),
			"Input should be a valid integer, unable to parse string as an integer")
		return
	}
	response.OK(c, h.svc.ByTeacher(h.db, teacherID, c.Query("semester")))
}

// Teachers 教师列表（课表选教师用，仅需 schedule:view）
func (h *Schedule) Teachers(c *gin.Context) {
	page, pageSize := pageOf(c)
	f := service.TeacherFilter{
		Page:           page,
		PageSize:       pageSize,
		Keyword:        c.Query("keyword"),
		CollegeID:      c.Query("college_id"),
		ResearchRoomID: c.Query("research_room_id"),
	}
	users, total, err := h.svc.ListTeachers(h.db, f)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	list := []gin.H{}
	for i := range users {
		u := &users[i]
		list = append(list, gin.H{
			"id":                u.ID,
			"user_no":           u.UserNo,
			"username":          u.Username,
			"college_id":        u.CollegeID,
			"college_name":      collegeName(u.College),
			"research_room_id":  u.ResearchRoomID,
			"research_room_name": roomName(u.ResearchRoom),
		})
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// CurrentSemester 当前学期配置
func (h *Schedule) CurrentSemester(c *gin.Context) {
	var cfg model.SemesterConfig
	if err := h.db.Where("is_current = 1").First(&cfg).Error; err != nil {
		// 无 is_current 配置时，按日期推算学期并尝试取其配置（对齐旧端）
		sem := h.svc.CurrentSemesterByDate().Semester
		h.db.Where("semester = ?", sem).First(&cfg)
	}
	if cfg.Semester == "" {
		response.OK(c, nil)
		return
	}
	response.OK(c, cfg)
}

// MySchedule 当前用户课表
func (h *Schedule) MySchedule(c *gin.Context) {
	u := middleware.CurrentUser(c)
	response.OK(c, h.svc.MySchedule(h.db, u, c.Query("semester")))
}

// Versions 学期版本历史
func (h *Schedule) Versions(c *gin.Context) {
	if c.Query("semester") == "" {
		missingQuery(c, "semester")
		return
	}
	list, err := h.svc.Versions(h.db, c.Query("semester"))
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, list)
}

// TeacherStatus 教师课表同步状态
func (h *Schedule) TeacherStatus(c *gin.Context) {
	if c.Query("semester") == "" {
		missingQuery(c, "semester")
		return
	}
	out, err := h.svc.TeacherStatus(h.db, c.Query("semester"))
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, out)
}

// Semesters 学期列表
func (h *Schedule) Semesters(c *gin.Context) {
	list, err := h.svc.Semesters(h.db)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, list)
}

// Stats 学期统计
func (h *Schedule) Stats(c *gin.Context) {
	out, err := h.svc.SemesterStats(h.db, c.Query("semester"))
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, out)
}

// SemesterConfigs 全部学期配置
func (h *Schedule) SemesterConfigs(c *gin.Context) {
	list, err := h.svc.SemesterConfigList(h.db)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, list)
}

// SemesterConfig 指定学期配置
func (h *Schedule) SemesterConfig(c *gin.Context) {
	cfg, err := h.svc.SemesterConfigBySemester(h.db, c.Param("semester"))
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, cfg)
}

// validWeeks 校验可选周数（1-52），未提供（nil）视为合法
func validWeeks(w *int) bool {
	return w == nil || (*w >= 1 && *w <= 52)
}

// CreateSemesterConfig 新建学期配置（管理员）
func (h *Schedule) CreateSemesterConfig(c *gin.Context) {
	var p struct {
		Semester  string  `json:"semester" binding:"required"`
		StartDate *string `json:"start_date" binding:"required"`
		Weeks     *int    `json:"weeks"`
		IsCurrent bool    `json:"is_current"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if !validWeeks(p.Weeks) {
		badReq(c, "weeks 须在 1-52 之间")
		return
	}
	var start *time.Time
	if p.StartDate != nil && *p.StartDate != "" {
		t, err := time.ParseInLocation("2006-01-02", *p.StartDate, time.Local)
		if err != nil {
			badReq(c, "start_date 格式错误，应为 YYYY-MM-DD")
			return
		}
		start = &t
	}
	if start == nil {
		badReq(c, "start_date 为必填")
		return
	}
	weeks := 0
	if p.Weeks != nil {
		weeks = *p.Weeks
	}
	cfg, err := h.svc.CreateSemesterConfig(h.db, p.Semester, *start, weeks, p.IsCurrent)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	response.OK(c, cfg)
}

// UpdateSemesterConfig 更新学期配置（管理员，不存在则创建，对齐旧端）
func (h *Schedule) UpdateSemesterConfig(c *gin.Context) {
	var p struct {
		StartDate *string `json:"start_date"`
		Weeks     *int    `json:"weeks"`
		IsCurrent *bool   `json:"is_current"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if !validWeeks(p.Weeks) {
		badReq(c, "weeks 须在 1-52 之间")
		return
	}
	var start *time.Time
	if p.StartDate != nil && *p.StartDate != "" {
		t, err := time.ParseInLocation("2006-01-02", *p.StartDate, time.Local)
		if err != nil {
			badReq(c, "start_date 格式错误，应为 YYYY-MM-DD")
			return
		}
		start = &t
	}
	cfg, err := h.svc.UpdateSemesterConfig(h.db, c.Param("semester"), start, p.Weeks, p.IsCurrent)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	response.OK(c, cfg)
}

// DeleteSemesterConfig 删除学期配置（管理员，当前学期不可删除）
func (h *Schedule) DeleteSemesterConfig(c *gin.Context) {
	if err := h.svc.DeleteSemesterConfig(h.db, c.Param("semester")); err != nil {
		badReq(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}
