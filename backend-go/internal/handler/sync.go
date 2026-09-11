package handler

import (
	"fmt"
	"log"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"backend-go/internal/config"
	"backend-go/internal/jwxt"
	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 批量同步进度（内存态）
var (
	batchMu       sync.Mutex
	batchProgress = map[string]map[string]interface{}{}
	batchTTL      = 30 * time.Minute

	// batchSem 后台同步任务并发上限（AC5）：同一时刻最多 1 个批量课表同步在跑，
	// 避免无界 goroutine 打爆爬虫目标与数据库；获得信号量后仍需等待课表写锁互斥。
	batchSem = make(chan struct{}, 1)
)

func initProgress(taskID string, total int, semester, scope string) {
	batchMu.Lock()
	defer batchMu.Unlock()
	batchProgress[taskID] = map[string]interface{}{
		"status": "running", "total": total, "completed": 0, "failed": 0, "percent": 0,
		"failed_ids": []map[string]interface{}{}, "current_teacher": nil,
		"current_teacher_no": nil, "semester": semester, "scope": scope,
		"stats": nil, "error": nil, "started_at": time.Now(), "finished_at": nil,
	}
}

func updateProgress(taskID string, fields map[string]interface{}) {
	batchMu.Lock()
	defer batchMu.Unlock()
	if rec, ok := batchProgress[taskID]; ok {
		for k, v := range fields {
			rec[k] = v
		}
	}
}

func getProgress(taskID string) map[string]interface{} {
	batchMu.Lock()
	defer batchMu.Unlock()
	if rec, ok := batchProgress[taskID]; ok {
		cp := map[string]interface{}{}
		for k, v := range rec {
			cp[k] = v
		}
		return cp
	}
	return nil
}

func cleanupExpiredProgress() {
	batchMu.Lock()
	defer batchMu.Unlock()
	now := time.Now()
	for tid, rec := range batchProgress {
		if fin, ok := rec["finished_at"].(*time.Time); ok && fin != nil && now.Sub(*fin) > batchTTL {
			delete(batchProgress, tid)
		}
	}
}

// 写课表的同步模块：这些入口会写入 course_schedule，通过 doSync 统一加全局课表写锁（AC4）。
var courseWriteModule = map[string]bool{
	"course_schedule": true,
	"sync_all":        true,
	"llsykb":          true,
	"crawl":           true,
}

// Sync 数据同步接口
type Sync struct {
	cfg *config.Config
	db  *gorm.DB
	svc *service.Sync
}

func NewSync(cfg *config.Config, db *gorm.DB) *Sync {
	return &Sync{cfg: cfg, db: db, svc: service.NewSync()}
}

// spider 用请求凭据或全局配置创建已登录爬虫
func (h *Sync) spider(username, password string) (*jwxt.BaseSync, error) {
	if username == "" {
		username = h.cfg.JWXTUser
	}
	if password == "" {
		password = h.cfg.JWXTPassword
	}
	if username == "" || password == "" {
		return nil, errJwxtCredential
	}
	auth := jwxt.NewAuth(h.cfg.JWXTBaseURLXS, h.cfg.JWXTBaseURLGL)
	if err := auth.Login(username, password); err != nil {
		return nil, err
	}
	return &jwxt.BaseSync{Auth: auth}, nil
}

var errJwxtCredential = &jwxtError{"教务系统凭据未配置（JWXT_USERNAME/JWXT_PASSWORD）"}

type jwxtError struct{ msg string }

func (e *jwxtError) Error() string { return e.msg }

// doSync 同步通用流程：锁 -> 记录开始 -> 执行 -> 记录结束
// 写课表的模块额外获取全局课表写锁，确保各入口互斥（AC4）。
func (h *Sync) doSync(c *gin.Context, module string, run func(*jwxt.BaseSync) (map[string]interface{}, error)) {
	if !h.svc.Acquire(module) {
		badReq(c, "同步正在进行中")
		return
	}
	defer h.svc.Release(module)

	if courseWriteModule[module] {
		if !h.svc.AcquireCourseWrite() {
			badReq(c, "课表同步正在进行中，请稍后再试")
			return
		}
		defer h.svc.ReleaseCourseWrite()
	}

	u := middleware.CurrentUser(c)
	logID, err := h.svc.RecordStart(h.db, &u.ID, u.Username, module)
	if err != nil {
		serverErr(c, "记录同步日志失败")
		return
	}
	h.svc.SetProgress(module, map[string]interface{}{"status": "running"})

	spider, err := h.spider("", "")
	if err != nil {
		h.svc.RecordEnd(h.db, logID, "failed", err.Error(), nil)
		h.svc.SetProgress(module, map[string]interface{}{"status": "failed", "message": err.Error()})
		badReq(c, err.Error())
		return
	}

	result, err := run(spider)
	if err != nil {
		h.svc.RecordEnd(h.db, logID, "failed", err.Error(), nil)
		h.svc.SetProgress(module, map[string]interface{}{"status": "failed", "message": err.Error()})
		badReq(c, err.Error())
		return
	}
	h.svc.RecordEnd(h.db, logID, "success", "同步成功", result)
	h.svc.SetProgress(module, map[string]interface{}{"status": "success"})
	response.OKMsg(c, "同步成功", result)
}

// SyncTeachers 从教务系统同步教师
func (h *Sync) SyncTeachers(c *gin.Context) {
	h.doSync(c, "teacher", func(sp *jwxt.BaseSync) (map[string]interface{}, error) {
		return sp.SyncTeachers(h.db, h.cfg.DefaultUserPassword)
	})
}

// SyncUnitsAndTeachers 同步单位与教师
func (h *Sync) SyncUnitsAndTeachers(c *gin.Context) {
	h.doSync(c, "unit", func(sp *jwxt.BaseSync) (map[string]interface{}, error) {
		unitRes, err := sp.SyncUnits(h.db)
		if err != nil {
			return nil, err
		}
		teacherRes, err := sp.SyncTeachers(h.db, h.cfg.DefaultUserPassword)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"units": unitRes, "teachers": teacherRes}, nil
	})
}

// SyncAll 全量同步（单位 + 教师 + 课表）
func (h *Sync) SyncAll(c *gin.Context) {
	var p struct {
		Semester string `json:"semester"`
	}
	_ = c.ShouldBindJSON(&p)
	// 兼容旧端 FastAPI：semester 走 query 参数
	if p.Semester == "" {
		p.Semester = c.Query("semester")
	}
	h.doSync(c, "sync_all", func(sp *jwxt.BaseSync) (map[string]interface{}, error) {
		unitRes, err := sp.SyncUnits(h.db)
		if err != nil {
			return nil, err
		}
		teacherRes, err := sp.SyncTeachers(h.db, h.cfg.DefaultUserPassword)
		if err != nil {
			return nil, err
		}
		courseRes, err := sp.SyncCourses(h.db, jwxt.SyncOptions{Semester: p.Semester})
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"units": unitRes, "teachers": teacherRes, "courses": courseRes}, nil
	})
}

// SyncCourseSchedule 同步课表
// 说明：底层调用 SyncCourses（queryzkb_teacher.jsp 按院系整页 + 姓名匹配），
// 与 /sync/llsykb/batch 结果同源但覆盖/精确度不如 SyncByTeacherNos，无逐人进度。
// 建议前端统一改走按学院批量同步，本接口保留用于兼容旧端。
func (h *Sync) SyncCourseSchedule(c *gin.Context) {
	var p struct {
		Semester string `json:"semester"`
	}
	_ = c.ShouldBindJSON(&p)
	// 兼容旧端 FastAPI：semester 走 query 参数
	if p.Semester == "" {
		p.Semester = c.Query("semester")
	}
	h.doSync(c, "course_schedule", func(sp *jwxt.BaseSync) (map[string]interface{}, error) {
		return sp.SyncCourses(h.db, jwxt.SyncOptions{Semester: p.Semester})
	})
}

// SyncTeachersStatus 教师同步状态
func (h *Sync) SyncTeachersStatus(c *gin.Context) {
	response.OK(c, h.svc.LastStatus(h.db, "teacher"))
}

// CrawlCourseSchedule 爬取课表（支持请求自带教务系统凭据）
// 说明：底层调用 SyncCourses，与 SyncByTeacherNos 结果同源；倾向用批量同步替代，保留用于兼容旧端。
func (h *Sync) CrawlCourseSchedule(c *gin.Context) {
	var p struct {
		Semester    string `json:"semester"`
		CollegeCode string `json:"college_code"`
		TeacherName string `json:"teacher_name"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)
	if !u.HasAnyRole("system_admin", "school_admin") {
		response.Fail(c, 403, "只有管理员可以爬取课程表")
		return
	}
	if !h.svc.Acquire("crawl") {
		badReq(c, "同步正在进行中")
		return
	}
	defer h.svc.Release("crawl")
	if !h.svc.AcquireCourseWrite() {
		badReq(c, "课表同步正在进行中，请稍后再试")
		return
	}
	defer h.svc.ReleaseCourseWrite()

	spider, err := h.spider(p.Username, p.Password)
	if err != nil {
		badReq(c, "教务系统登录失败: "+err.Error())
		return
	}
	result, err := spider.SyncCourses(h.db, jwxt.SyncOptions{
		Semester: p.Semester, Skyx: p.CollegeCode, TeacherName: p.TeacherName,
	})
	if err != nil {
		serverErr(c, err.Error())
		return
	}
	service.LogRecord(h.db, &u.ID, u.Username, "crawl", "course_schedule", nil, "jwxt", result)
	response.OKMsg(c, "爬取成功", result)
}

// ParseHTML 解析 HTML 并保存课表（手动上传场景）
func (h *Sync) ParseHTML(c *gin.Context) {
	var p struct {
		HTMLContent string `json:"html_content"`
		Semester    string `json:"semester"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || strings.TrimSpace(p.HTMLContent) == "" {
		badReq(c, "html_content 为必填")
		return
	}
	u := middleware.CurrentUser(c)
	if !u.HasAnyRole("system_admin", "school_admin") {
		response.Fail(c, 403, "只有管理员可以导入课程表")
		return
	}
	schedules, stats, err := jwxt.ParseScheduleHTML(p.HTMLContent)
	if err != nil {
		badReq(c, "解析失败: "+err.Error())
		return
	}
	semester := p.Semester
	if semester == "" {
		semester = jwxt.CurrentSemesterCode()
	}
	// 手动导入同样写入课表，与其它同步入口互斥（AC4）
	if !h.svc.AcquireCourseWrite() {
		badReq(c, "课表同步正在进行中，请稍后再试")
		return
	}
	defer h.svc.ReleaseCourseWrite()
	saveStats, err := jwxt.SaveSchedules(h.db, schedules, semester, time.Now(), 0)
	if err != nil {
		badReq(c, "保存失败: "+err.Error())
		return
	}
	service.LogRecord(h.db, &u.ID, u.Username, "parse_html", "course_schedule", nil, "jwxt",
		map[string]interface{}{"semester": semester, "stats": stats})
	response.OKMsg(c, "解析并保存成功", gin.H{
		"success": true, "semester": semester,
		"version": saveStats["version"], "stats": saveStats,
	})
}

// currentSemesterCodeStr 当前学期（配置优先，否则推算）
func (h *Sync) currentSemesterCodeStr() string {
	var cfg model.SemesterConfig
	if err := h.db.Where("is_current = 1").First(&cfg).Error; err == nil {
		return cfg.Semester
	}
	return jwxt.CurrentSemesterCode()
}

// SyncLlsykb 按工号同步教师课表（llsykb，同步执行）
func (h *Sync) SyncLlsykb(c *gin.Context) {
	var p struct {
		Xnxq01id   string   `json:"xnxq01id"`
		TeacherIDs []string `json:"teacher_ids"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if p.Xnxq01id == "" {
		p.Xnxq01id = h.currentSemesterCodeStr()
	}
	if len(p.TeacherIDs) == 0 {
		badReq(c, "请至少选择一位教师")
		return
	}

	if !h.svc.Acquire("llsykb") {
		badReq(c, "同步正在进行中")
		return
	}
	defer h.svc.Release("llsykb")
	if !h.svc.AcquireCourseWrite() {
		badReq(c, "课表同步正在进行中，请稍后再试")
		return
	}
	defer h.svc.ReleaseCourseWrite()

	u := middleware.CurrentUser(c)
	logID, err := h.svc.RecordStart(h.db, &u.ID, u.Username, "teacher")
	if err != nil {
		serverErr(c, "记录同步日志失败")
		return
	}

	spider, err := h.spider("", "")
	if err != nil {
		h.svc.RecordEnd(h.db, logID, "failed", err.Error(), nil)
		badReq(c, "教务系统登录失败: "+err.Error())
		return
	}

	result := spider.SyncByTeacherNos(h.db, p.Xnxq01id, p.TeacherIDs, nil)
	h.svc.RecordEnd(h.db, logID, "success", "教师课表同步成功", result)
	service.LogRecord(h.db, &u.ID, u.Username, "sync", "llsykb", nil, "course_schedule", result)
	response.OKMsg(c, "教师课表同步成功", result)
}

// SyncLlsykbBatch 批量同步教师课表（后台任务 + 进度追踪）
// 推荐入口：基于 SyncByTeacherNos 按工号逐人查询，遍历真实用户表（可带 college_id 过滤），
// 可覆盖无课教师、按用户数推进度。课表同步建议统一走本接口。
func (h *Sync) SyncLlsykbBatch(c *gin.Context) {
	cleanupExpiredProgress()

	var p struct {
		Xnxq01id   string   `json:"xnxq01id"`
		CollegeID  *int     `json:"college_id"`
		TeacherNos []string `json:"teacher_nos"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if p.Xnxq01id == "" {
		p.Xnxq01id = h.currentSemesterCodeStr()
	}

	// 显式指定 teacher_nos 时直接用（选择教师同步）；否则按学院过滤在职人员工号（不限角色）
	var teacherNos []string
	scope := "all"
	if len(p.TeacherNos) > 0 {
		teacherNos = p.TeacherNos
		scope = fmt.Sprintf("teachers:%d", len(teacherNos))
	} else {
		q := h.db.Model(&model.User{}).Where("status = 1")
		if p.CollegeID != nil {
			q = q.Where("college_id = ?", *p.CollegeID)
		}
		var users []model.User
		q.Select("id, user_no, username").Find(&users)
		teacherNos = make([]string, 0, len(users))
		for _, u := range users {
			teacherNos = append(teacherNos, u.UserNo)
		}
		if p.CollegeID != nil {
			scope = fmt.Sprintf("college:%d", *p.CollegeID)
		}
	}
	if len(teacherNos) == 0 {
		badReq(c, "未找到符合条件的教师记录")
		return
	}
	taskID := uuid.NewString()[:16]
	initProgress(taskID, len(teacherNos), p.Xnxq01id, scope)

	// 后台执行：信号量限流 + 课表写锁互斥（AC4/AC5），panic 记录日志与堆栈
	go func(taskID, semester string, nos []string) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[sync] batch task %s panic: %v\n%s", taskID, r, debug.Stack())
				now := time.Now()
				updateProgress(taskID, map[string]interface{}{"status": "failed", "error": fmt.Sprint(r), "finished_at": &now})
			}
		}()
		// 并发上限：已有批量任务在跑则本次直接失败（等待无意义且不可告知前端）
		select {
		case batchSem <- struct{}{}:
			defer func() { <-batchSem }()
		default:
			now := time.Now()
			updateProgress(taskID, map[string]interface{}{"status": "failed", "error": "已有批量同步任务进行中", "finished_at": &now})
			return
		}
		if !h.svc.AcquireCourseWrite() {
			now := time.Now()
			updateProgress(taskID, map[string]interface{}{"status": "failed", "error": "课表同步正在进行中，请稍后再试", "finished_at": &now})
			return
		}
		defer h.svc.ReleaseCourseWrite()

		spider, err := h.spider("", "")
		if err != nil {
			now := time.Now()
			updateProgress(taskID, map[string]interface{}{"status": "failed", "error": "教务系统登录失败: " + err.Error(), "finished_at": &now})
			return
		}
		cb := func(no, name string, ok bool, reason string) {
			batchMu.Lock()
			rec := batchProgress[taskID]
			if rec == nil {
				batchMu.Unlock()
				return
			}
			completed := rec["completed"].(int)
			failed := rec["failed"].(int)
			total := rec["total"].(int)
			failedIDs, _ := rec["failed_ids"].([]map[string]interface{})
			batchMu.Unlock()

			fields := map[string]interface{}{
				"current_teacher": name, "current_teacher_no": no,
			}
			if ok && reason == "" {
				// 开始处理，仅更新当前教师
			} else {
				fields["completed"] = completed + 1
				if ok {
					// 完成
				} else {
					fields["failed"] = failed + 1
					fields["failed_ids"] = append(failedIDs, map[string]interface{}{
						"id": no, "name": name, "reason": reason,
					})
				}
				// 进度百分比：已处理数(含成功+失败)/总数
				done := completed + 1
				if total > 0 {
					fields["percent"] = int(float64(done) / float64(total) * 100)
				}
			}
			updateProgress(taskID, fields)
		}
		result := spider.SyncByTeacherNos(h.db, semester, nos, cb)
		now := time.Now()
		updateProgress(taskID, map[string]interface{}{"status": "completed", "percent": 100, "stats": result, "finished_at": &now})
	}(taskID, p.Xnxq01id, teacherNos)

	response.OKMsg(c, "批量同步任务已提交到后台执行", gin.H{
		"task_id": taskID, "total": len(teacherNos), "semester": p.Xnxq01id, "scope": scope,
	})
}

// GetLlsykbProgress 查询批量同步进度
func (h *Sync) GetLlsykbProgress(c *gin.Context) {
	cleanupExpiredProgress()
	rec := getProgress(c.Param("taskId"))
	if rec == nil {
		response.Fail(c, 404, "任务不存在或已过期")
		return
	}
	response.OK(c, rec)
}

var validUserNoRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ScanInvalidUsers 扫描工号格式异常的用户
func (h *Sync) ScanInvalidUsers(c *gin.Context) {
	var users []model.User
	h.db.Preload("College").Find(&users)
	records := []gin.H{}
	suggestDelete := 0
	for _, u := range users {
		if validUserNoRe.MatchString(u.UserNo) {
			continue
		}
		// 同名正常用户
		var sameName []model.User
		h.db.Where("username = ? AND id <> ?", u.Username, u.ID).Find(&sameName)
		valids := []gin.H{}
		for _, s := range sameName {
			if validUserNoRe.MatchString(s.UserNo) {
				var cn interface{}
				if s.College != nil {
					cn = s.College.Name
				}
				valids = append(valids, gin.H{"id": s.ID, "user_no": s.UserNo, "college_name": cn})
			}
		}
		sd := len(valids) > 0
		if sd {
			suggestDelete++
		}
		var cn interface{}
		if u.College != nil {
			cn = u.College.Name
		}
		records = append(records, gin.H{
			"id": u.ID, "user_no": u.UserNo, "username": u.Username,
			"college_id": u.CollegeID, "college_name": cn, "status": u.Status,
			"same_name_valid_users": valids, "suggest_delete": sd,
		})
	}
	response.OKMsg(c, fmt.Sprintf("发现 %d 条工号异常记录", len(records)), gin.H{
		"total": len(records), "suggest_delete_count": suggestDelete, "records": records,
	})
}

// CleanupInvalidUser 删除工号异常用户及其关联
func (h *Sync) CleanupInvalidUser(c *gin.Context) {
	id, err := parseIDParam(c, "userId")
	if err != nil {
		badReq(c, err.Error())
		return
	}
	var u model.User
	if err := h.db.First(&u, id).Error; err != nil {
		response.Fail(c, 404, "用户不存在")
		return
	}
	if validUserNoRe.MatchString(u.UserNo) {
		badReq(c, "该用户工号格式正常，不允许通过此接口删除")
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
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
	if err != nil {
		serverErr(c, "删除失败")
		return
	}
	caller := middleware.CurrentUser(c)
	service.LogRecord(h.db, &caller.ID, caller.Username, "delete", "user", &id, "user",
		map[string]interface{}{"action": "cleanup_invalid", "user_no": u.UserNo})
	response.OKMsg(c, fmt.Sprintf("已删除用户 %s（工号: %s）", u.Username, u.UserNo), nil)
}

// parseIDParam 路径数字参数解析
func parseIDParam(c *gin.Context, key string) (int, error) {
	id, err := strconv.Atoi(c.Param(key))
	if err != nil {
		return 0, fmt.Errorf("无效的 %s", key)
	}
	return id, nil
}

// ---------- 权限辅助 ----------

// requireSyncAdmin 对齐旧端 require_school_admin（= require_college_admin：管理类角色需有所属学院）
func requireSyncAdmin(c *gin.Context) bool {
	u := middleware.CurrentUser(c)
	if u.HasRole("system_admin") {
		return true
	}
	if u.HasAnyRole("school_admin", "college_admin") {
		if u.CollegeID == nil && len(u.UserColleges) == 0 {
			forbidden(c, "您没有管理的学院")
			return false
		}
		return true
	}
	forbidden(c, "权限不足，需要学院管理员权限")
	return false
}

// requireSyncSystemAdmin 对齐旧端 require_system_admin（role:manage）
func requireSyncSystemAdmin(c *gin.Context) bool {
	u := middleware.CurrentUser(c)
	if u.HasRole("system_admin") {
		return true
	}
	forbidden(c, "权限不足")
	return false
}

// ---------- /crawl/*（对齐旧端 crawl.py） ----------

// CrawlTimetable 同步爬取课表并导入（query 参数 semester/username/password）
// 说明：底层调用 SyncCourses（按院系整页），与按学院批量同步（/sync/llsykb/batch）结果同源，
// 建议以批量同步为主入口，本接口保留用于兼容旧端 crawl.py。
func (h *Sync) CrawlTimetable(c *gin.Context) {
	if !requireSyncAdmin(c) {
		return
	}
	spider, err := h.spider(c.Query("username"), c.Query("password"))
	if err != nil {
		badReq(c, "教务系统登录失败")
		return
	}
	result, err := spider.SyncCourses(h.db, jwxt.SyncOptions{Semester: c.Query("semester")})
	if err != nil {
		serverErr(c, "爬取失败: "+err.Error())
		return
	}
	response.OKMsg(c, "爬取完成", result)
}

// CrawlTimetablePreview 预览课表（旧端为 TODO，仅校验登录后返回空预览）
func (h *Sync) CrawlTimetablePreview(c *gin.Context) {
	if !requireSyncAdmin(c) {
		return
	}
	if _, err := h.spider(c.Query("username"), c.Query("password")); err != nil {
		badReq(c, "教务系统登录失败")
		return
	}
	response.OKMsg(c, "预览功能开发中", gin.H{"total": 0, "preview": []interface{}{}})
}

// CrawlTimetableAsync 后台爬取课表
// 说明：同 CrawlTimetable，底层为 SyncCourses，建议由按学院批量同步替代。
func (h *Sync) CrawlTimetableAsync(c *gin.Context) {
	if !requireSyncAdmin(c) {
		return
	}
	semester := c.Query("semester")
	username := c.Query("username")
	password := c.Query("password")
	// 课表写锁互斥 + panic 日志（AC4/AC5）；信号量的获取与释放都必须在后台 goroutine 内，
	// 保证 goroutine 生命周期内一直被限流（而非请求返回即释放）。
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[sync] crawl async panic: %v\n%s", r, debug.Stack())
			}
		}()
		select {
		case batchSem <- struct{}{}:
			defer func() { <-batchSem }()
		default:
			log.Printf("[sync] crawl async 拒绝：已有同步任务进行中")
			return
		}
		if !h.svc.AcquireCourseWrite() {
			return
		}
		defer h.svc.ReleaseCourseWrite()
		spider, err := h.spider(username, password)
		if err != nil {
			return
		}
		_, _ = spider.SyncCourses(h.db, jwxt.SyncOptions{Semester: semester})
	}()
	response.OKMsg(c, "爬取任务已提交到后台执行", nil)
}

// llsykbRecordMap 解析记录输出（对齐旧端 parse_llsykb_html 的 dict 键）
func llsykbRecordMap(r jwxt.LlsykbRecord) map[string]interface{} {
	return map[string]interface{}{
		"weekday": r.Weekday, "section": r.Section,
		"course_name": r.CourseName, "course_type": r.CourseType,
		"credits": r.Credits, "teacher": r.Teacher,
		"classes": r.Classes, "student_count": r.StudentCount,
		"week_pattern": r.WeekPattern, "day_type": r.DayType,
		"section_range": r.SectionRange, "classroom": r.Classroom, "raw": r.Raw,
	}
}

// CrawlLlsykb 用请求自带凭据查询单个教师课表并入库
func (h *Sync) CrawlLlsykb(c *gin.Context) {
	if !requireSyncAdmin(c) {
		return
	}
	var p struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Xnxq01id    string `json:"xnxq01id"`
		TeacherID   string `json:"teacherID"`
		Type        string `json:"type"`
		Zc          string `json:"zc"`
		Yxx         string `json:"yxx"`
		TeacherIDmc string `json:"teacherIDmc"`
		Jg0101mc    string `json:"jg0101mc"`
		Jszc        string `json:"jszc"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || p.Xnxq01id == "" || p.TeacherID == "" {
		badReq(c, "请求参数错误")
		return
	}
	auth := jwxt.NewAuth(h.cfg.JWXTBaseURLXS, h.cfg.JWXTBaseURLGL)
	if err := auth.Login(p.Username, p.Password); err != nil {
		response.Fail(c, 401, "教务系统登录失败")
		return
	}
	sp := &jwxt.BaseSync{Auth: auth}
	htmlContent, err := sp.QueryLlsykb(p.Xnxq01id, p.TeacherID)
	if err != nil || htmlContent == "" {
		serverErr(c, "查询教师课表课表失败")
		return
	}
	records := jwxt.ParseLlsykbHTML(htmlContent)
	out := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		out = append(out, llsykbRecordMap(r))
	}

	// 入库：按教师分组（对齐旧端 save_schedules_from_parser）；写入课表需全局写锁互斥（AC4）
	if len(records) > 0 {
		if !h.svc.AcquireCourseWrite() {
			serverErr(c, "课表同步正在进行中，请稍后再试")
			return
		}
		defer h.svc.ReleaseCourseWrite()
		groups := map[string][]jwxt.CourseInfo{}
		ids := map[string]int{}
		for _, r := range records {
			name := strings.TrimSpace(strings.Split(r.Teacher, ",")[0])
			if name == "" {
				name = "unknown"
			}
			if _, ok := groups[name]; !ok {
				var u model.User
				h.db.Where("username = ?", name).First(&u)
				ids[name] = u.ID
			}
			wp := r.WeekPattern
			if wp != "" && !strings.Contains(wp, "周") {
				wp += "周"
			}
			classes := r.Classes
			if r.StudentCount > 0 && !strings.Contains(classes, fmt.Sprintf("(%d)", r.StudentCount)) {
				classes = fmt.Sprintf("%s(%d)", classes, r.StudentCount)
			}
			var sc *int
			if r.StudentCount > 0 {
				v := r.StudentCount
				sc = &v
			}
			groups[name] = append(groups[name], jwxt.CourseInfo{
				CourseName: r.CourseName, ClassInfo: classes, WeekPattern: wp,
				WeekDay: r.Weekday, Section: r.Section, Classroom: r.Classroom,
				RawData: r.Raw, StudentCount: sc,
			})
		}
		var schedules []jwxt.TeacherSchedule
		for name, courses := range groups {
			schedules = append(schedules, jwxt.TeacherSchedule{TeacherName: name, TeacherID: ids[name], Courses: courses})
		}
		_, _ = jwxt.SaveSchedules(h.db, schedules, p.Xnxq01id, time.Now(), 0)
	}

	c.JSON(200, gin.H{
		"success":        true,
		"xnxq01id":       p.Xnxq01id,
		"message":        fmt.Sprintf("查询成功，解析到 %d 条课程记录", len(records)),
		"html":           htmlContent,
		"content_length": len(htmlContent),
		"records":        out,
		"record_count":   len(records),
	})
}

// SyncLlsykbPreview 预览 llsykb 数据（不入库）
func (h *Sync) SyncLlsykbPreview(c *gin.Context) {
	if !requireSyncSystemAdmin(c) {
		return
	}
	xnxq01id := c.Query("xnxq01id")
	if xnxq01id == "" {
		xnxq01id = h.currentSemesterCodeStr()
	}
	spider, err := h.spider("", "")
	if err != nil {
		badReq(c, "教务系统登录失败")
		return
	}
	teacherIDs := c.QueryArray("teacher_ids")
	if len(teacherIDs) == 0 {
		teacherIDs = []string{h.cfg.JWXTUser}
	}
	allRecords := []map[string]interface{}{}
	for _, tid := range teacherIDs {
		htmlContent, err := spider.QueryLlsykb(xnxq01id, tid)
		if err != nil {
			continue
		}
		for _, r := range jwxt.ParseLlsykbHTML(htmlContent) {
			allRecords = append(allRecords, llsykbRecordMap(r))
		}
	}
	// 按教师分组
	order := []string{}
	counts := map[string]int{}
	for _, r := range allRecords {
		t := fmt.Sprint(r["teacher"])
		name := strings.TrimSpace(strings.Split(t, ",")[0])
		if name == "" {
			name = "unknown"
		}
		if _, ok := counts[name]; !ok {
			order = append(order, name)
		}
		counts[name]++
	}
	teachers := make([]gin.H, 0, len(order))
	for _, name := range order {
		teachers = append(teachers, gin.H{"name": name, "count": counts[name]})
	}
	response.OKMsg(c, fmt.Sprintf("预览到 %d 条课程记录", len(allRecords)), gin.H{
		"semester":      xnxq01id,
		"total_records": len(allRecords),
		"teachers":      teachers,
		"records":       allRecords,
	})
}

// SyncColleges 学院/教研室同步（对齐旧端 POST /colleges/sync-from-jwxt）
func (h *Sync) SyncColleges(c *gin.Context) {
	if !requireSyncSystemAdmin(c) {
		return
	}
	if !h.svc.Acquire("unit") {
		badReq(c, "同步正在进行中")
		return
	}
	defer h.svc.Release("unit")

	spider, err := h.spider("", "")
	if err != nil {
		badReq(c, "教务系统登录失败")
		return
	}
	result, err := spider.SyncUnits(h.db)
	if err != nil {
		badReq(c, "导入失败: "+err.Error())
		return
	}
	u := middleware.CurrentUser(c)
	service.LogRecord(h.db, &u.ID, u.Username, "sync", "unit", nil, "jwxt", result)
	response.OKMsg(c, "单位信息同步成功", gin.H{
		"total":          result["total"],
		"colleges_added": result["colleges_added"],
		"rooms_added":    result["rooms_added"],
		"errors":         result["errors"],
	})
}
