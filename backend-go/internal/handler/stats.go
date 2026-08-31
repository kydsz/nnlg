package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/middleware"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Stats 统计接口
type Stats struct {
	db  *gorm.DB
	svc *service.Stats
}

func NewStats(db *gorm.DB) *Stats {
	return &Stats{db: db, svc: service.NewStats()}
}

// CurrentSemester 当前学期
func (h *Stats) CurrentSemester(c *gin.Context) {
	info, err := service.NewSchedule().CurrentSemester(h.db)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}
	response.OK(c, info)
}

// Overview 首页大盘
func (h *Stats) Overview(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var err error
	var start, end *time.Time
	if start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if end, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if start != nil && end != nil && start.After(*end) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	semester := c.Query("semester")
	cachedJSON(c, "overview", func() (interface{}, error) {
		return h.svc.Overview(h.db, u, semester, start, end)
	})
}

// Teachers 教师评教统计（分页，默认每页 10 对齐旧端）
func (h *Stats) Teachers(c *gin.Context) {
	page, pageSize := pageOfDefault(c, 10)
	f := service.TeacherStatsFilters{
		CollegeIDs:     splitIntsHandler(c.Query("college_ids")),
		Keyword:        c.Query("keyword"),
		EvaluatorRoles: splitRoles(c.Query("evaluator_roles")),
		Page:           page, PageSize: pageSize,
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	cachedJSON(c, "teachers", func() (interface{}, error) {
		list, total, err := h.svc.TeacherStats(h.db, u, f)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}, nil
	})
}

// College 学院评教统计
func (h *Stats) College(c *gin.Context) {
	u := middleware.CurrentUser(c)
	collegeID := qInt(c, "college_id")
	semester := c.Query("semester")
	cachedJSON(c, "college", func() (interface{}, error) {
		return h.svc.CollegeStats(h.db, u, collegeID, semester)
	})
}

// Colleges 学院评教统计列表
func (h *Stats) Colleges(c *gin.Context) {
	page, pageSize := pageOfDefault(c, 10) // 对齐旧端默认 page_size=10
	f := service.CollegeStatsFilters{
		CollegeIDs: splitIntsHandler(c.Query("college_ids")),
		Semester:   c.Query("semester"),
		Page:       page, PageSize: pageSize,
	}
	if v := c.Query("evaluator_roles"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				f.EvaluatorRoles = append(f.EvaluatorRoles, s)
			}
		}
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	cachedJSON(c, "colleges", func() (interface{}, error) {
		list, total, semester, err := h.svc.CollegeStatsList(h.db, u, f)
		if err != nil {
			return nil, err
		}
		// 已评/未评教师名单仅导出接口返回（对齐旧端）
		for _, item := range list {
			delete(item, "evaluated_teacher_names")
			delete(item, "unevaluated_teacher_names")
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize, "semester": semester}, nil
	})
}

// Campus 校区评教统计
func (h *Stats) Campus(c *gin.Context) {
	var err error
	var start, end *time.Time
	if start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if end, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if start != nil && end != nil && start.After(*end) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	campusID := qInt(c, "campus_id")
	cachedJSON(c, "campus", func() (interface{}, error) {
		return h.svc.CampusStats(h.db, u, campusID, start, end)
	})
}

// Campuses 校区评教统计列表（按校区逐行聚合）
func (h *Stats) Campuses(c *gin.Context) {
	var err error
	var start, end *time.Time
	if start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if end, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if start != nil && end != nil && start.After(*end) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	cachedJSON(c, "campuses", func() (interface{}, error) {
		list, total, err := h.svc.CampusStatsList(h.db, u, start, end)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total}, nil
	})
}

// CollegeTeachers 指定学院教师评教详情
func (h *Stats) CollegeTeachers(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("college_id"))
	if err != nil {
		badReq(c, "无效的学院 ID")
		return
	}
	u := middleware.CurrentUser(c)
	out, err := h.svc.CollegeTeacherDetails(h.db, u, id)
	if err != nil {
		response.Fail(c, http.StatusForbidden, err.Error())
		return
	}
	response.OK(c, out)
}

// Supervisors 督导评教统计列表
func (h *Stats) Supervisors(c *gin.Context) {
	page, pageSize := pageOfDefault(c, 10)
	f := service.PersonStatsFilters{
		CollegeIDs: splitIntsHandler(c.Query("college_ids")),
		Keyword:    c.Query("keyword"),
		Page:       page, PageSize: pageSize,
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	cachedJSON(c, "supervisors", func() (interface{}, error) {
		list, total, err := h.svc.SupervisorStats(h.db, u, f)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}, nil
	})
}

// Evaluators 评教人统计列表
func (h *Stats) Evaluators(c *gin.Context) {
	page, pageSize := pageOfDefault(c, 10)
	f := service.PersonStatsFilters{
		CollegeIDs: splitIntsHandler(c.Query("college_ids")),
		Keyword:    c.Query("keyword"),
		Page:       page, PageSize: pageSize,
	}
	var roles []string
	if v := c.Query("evaluator_roles"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				roles = append(roles, s)
			}
		}
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	rolesCopy := roles
	cachedJSON(c, "evaluators", func() (interface{}, error) {
		list, total, err := h.svc.EvaluatorStats(h.db, u, f, rolesCopy)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}, nil
	})
}

// UnteachedTeachers 未被听课教师
func (h *Stats) UnteachedTeachers(c *gin.Context) {
	page, pageSize := pageOf(c)
	var err error
	var start, end *time.Time
	if start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if end, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if start != nil && end != nil && start.After(*end) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	collegeID := qInt(c, "college_id")
	cachedJSON(c, "unteached", func() (interface{}, error) {
		list, total, err := h.svc.UnteachedTeachers(h.db, u, collegeID, start, end, page, pageSize)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}, nil
	})
}

// EvaluationRecords 评教记录合并统计
func (h *Stats) EvaluationRecords(c *gin.Context) {
	page, pageSize := pageOfDefault(c, 10) // 对齐旧端默认 page_size=10
	f := service.RecordStatsFilters{
		CollegeIDs: splitIntsHandler(c.Query("college_ids")),
		TeacherID:  qInt(c, "teacher_id"), EvaluatorID: qInt(c, "evaluator_id"),
		Keyword: c.Query("keyword"), Page: page, PageSize: pageSize,
	}
	if v := c.Query("evaluator_roles"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				f.EvaluatorRoles = append(f.EvaluatorRoles, s)
			}
		}
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	cachedJSON(c, "eval_records", func() (interface{}, error) {
		list, total, err := h.svc.EvaluationRecordsStats(h.db, u, f)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}, nil
	})
}

// TeacherSummary 教师评教汇总
func (h *Stats) TeacherSummary(c *gin.Context) {
	page, pageSize := pageOfDefault(c, 10)
	f := service.SummaryFilters{
		CollegeIDs:      splitIntsHandler(c.Query("college_ids")),
		CampusID:        qInt(c, "campus_id"),
		ResearchRoomIDs: splitIntsHandler(c.Query("research_room_ids")),
		Keyword:         c.Query("keyword"),
		SortBy:          c.Query("sort_by"), SortOrder: c.DefaultQuery("sort_order", "desc"),
		Page: page, PageSize: pageSize,
	}
	if v := c.Query("evaluator_roles"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				f.EvaluatorRoles = append(f.EvaluatorRoles, s)
			}
		}
	}
	if v := c.Query("has_courses"); v == "true" || v == "1" {
		b := true
		f.HasCourses = &b
	} else if v == "false" || v == "0" {
		b := false
		f.HasCourses = &b
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	cachedJSON(c, "teacher_summary", func() (interface{}, error) {
		list, total, err := h.svc.TeacherEvaluationSummary(h.db, u, f)
		if err != nil {
			return nil, err
		}
		return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}, nil
	})
}

// parseDatePtr YYYY-MM-DD 日期解析
func parseDatePtr(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
