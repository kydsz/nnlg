package service

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// Schedule 课表与学期业务
type Schedule struct{}

func NewSchedule() *Schedule { return &Schedule{} }

// SemesterInfo 当前学期信息
type SemesterInfo struct {
	Semester  string `json:"semester"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

const defaultSemesterWeeks = 20

// semesterDurationDays 学期总天数：周数可配置，未配置（<=0）按默认 20 周
func semesterDurationDays(weeks int) int {
	if weeks <= 0 {
		weeks = defaultSemesterWeeks
	}
	return weeks * 7
}

// CurrentSemesterByDate 按日期推算当前学期（对齐旧端 _get_current_semester，不查库）
func (s *Schedule) CurrentSemesterByDate() SemesterInfo {
	now := time.Now()
	y, m := now.Year(), int(now.Month())
	var sem string
	var start time.Time
	switch {
	case m >= 8: // 8 月及以后为第一学期
		sem = fmt.Sprintf("%d-%d-1", y, y+1)
		start = time.Date(y, time.September, 1, 0, 0, 0, 0, now.Location())
	case m >= 2: // 2-7 月为第二学期
		sem = fmt.Sprintf("%d-%d-2", y-1, y)
		start = time.Date(y, time.February, 1, 0, 0, 0, 0, now.Location())
	default: // 1 月延续上一秋学期
		sem = fmt.Sprintf("%d-%d-1", y-1, y)
		start = time.Date(y-1, time.September, 1, 0, 0, 0, 0, now.Location())
	}
	return SemesterInfo{
		Semester:  sem,
		StartDate: start.Format("2006-01-02"),
		EndDate:   start.AddDate(0, 0, semesterDurationDays(0)).Format("2006-01-02"),
	}
}

// CurrentSemester 当前学期：优先取配置表 is_current，否则按日期推算
func (s *Schedule) CurrentSemester(db *gorm.DB) (*SemesterInfo, error) {
	var cfg model.SemesterConfig
	err := db.Where("is_current = 1").First(&cfg).Error
	if err == nil {
		start := cfg.StartDate.ToTime()
		return &SemesterInfo{
			Semester:  cfg.Semester,
			StartDate: start.Format("2006-01-02"),
			EndDate:   start.AddDate(0, 0, semesterDurationDays(cfg.Weeks)).Format("2006-01-02"),
		}, nil
	}
	info := s.CurrentSemesterByDate()
	return &info, nil
}

// ScheduleFilters 课表查询
type ScheduleFilters struct {
	TeacherID   *int
	TeacherIDs  []int // 限定的教师范围（教师角色按学院过滤）
	Semester    string
	TeacherName string // 教师姓名模糊
	IsCurrent   bool   // 仅当前版本（旧端列表固定 true）
}

// List 课表主表分页
func (s *Schedule) List(db *gorm.DB, f ScheduleFilters, page, pageSize int) ([]model.CourseSchedule, int64, error) {
	q := db.Model(&model.CourseSchedule{})
	if f.TeacherID != nil {
		q = q.Where("teacher_id = ?", *f.TeacherID)
	}
	if len(f.TeacherIDs) > 0 {
		q = q.Where("teacher_id IN ?", f.TeacherIDs)
	}
	if f.Semester != "" {
		q = q.Where("semester = ?", f.Semester)
	}
	if f.TeacherName != "" {
		q = q.Where("teacher_name LIKE ?", "%"+f.TeacherName+"%")
	}
	if f.IsCurrent {
		q = q.Where("is_current = ?", true)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.CourseSchedule
	// 旧端无显式排序，MySQL 实际按主键顺序返回
	err := q.Session(&gorm.Session{}).
		Order("id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// studentCountRe 对齐旧端 re.findall(r'\((\d+)\)')，仅匹配半角括号
var studentCountRe = regexp.MustCompile(`\((\d+)\)`)

// ParseStudentCount 从 class_info 提取学生人数：取最后一个括号内数字
// 如 "2404计算机班,2403计算机(125)" -> 125
func ParseStudentCount(classInfo string) (int, bool) {
	ms := studentCountRe.FindAllStringSubmatch(classInfo, -1)
	if len(ms) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(ms[len(ms)-1][1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Detail 课表详情（扁平结构，含明细与学生人数解析，对齐旧端 _enrich_schedule_with_student_count）
func (s *Schedule) Detail(db *gorm.DB, id int) (map[string]interface{}, error) {
	var sc model.CourseSchedule
	if err := db.Preload("Details").First(&sc, id).Error; err != nil {
		return nil, errors.New("课程表不存在")
	}
	return scheduleDetailPayload(&sc), nil
}

// scheduleDetailPayload 课表 + 明细的扁平响应体；schedule.student_count 取明细中的最大值
func scheduleDetailPayload(sc *model.CourseSchedule) map[string]interface{} {
	details := []map[string]interface{}{}
	var maxCount *int
	for _, d := range sc.Details {
		item := map[string]interface{}{
			"id": d.ID, "schedule_id": d.ScheduleID,
			"course_name": d.CourseName, "class_info": d.ClassInfo,
			"week_pattern": d.WeekPattern, "week_day": d.WeekDay,
			"section": d.Section, "classroom": d.Classroom,
			"raw_data": d.RawData, "create_time": d.CreateTime, "update_time": d.UpdateTime,
		}
		if n, ok := ParseStudentCount(d.ClassInfo); ok {
			cnt := n
			item["student_count"] = &cnt
			if maxCount == nil || cnt > *maxCount {
				maxCount = &cnt
			}
		} else {
			item["student_count"] = nil
		}
		details = append(details, item)
	}
	return map[string]interface{}{
		"id": sc.ID, "teacher_id": sc.TeacherID, "teacher_name": sc.TeacherName,
		"semester": sc.Semester, "version": sc.Version, "is_current": sc.IsCurrent,
		"crawl_time": sc.CrawlTime, "change_summary": sc.ChangeSummary,
		"create_time": sc.CreateTime, "update_time": sc.UpdateTime,
		"student_count": maxCount, "details": details,
	}
}

// emptyScheduleShell 无课表数据的空壳响应（对齐旧端）
func emptyScheduleShell(teacherID int, teacherName, semester string) map[string]interface{} {
	now := model.LocalTimePtr(time.Now())
	return map[string]interface{}{
		"id": 0, "teacher_id": teacherID, "teacher_name": teacherName,
		"semester": semester, "version": 0, "student_count": nil,
		"is_current": false, "crawl_time": now, "change_summary": nil,
		"create_time": now, "update_time": now, "details": []map[string]interface{}{},
	}
}

// ByTeacher 按教师查询当前版本课表；无数据返回空壳（200）
func (s *Schedule) ByTeacher(db *gorm.DB, teacherID int, semester string) map[string]interface{} {
	if semester == "" {
		semester = s.CurrentSemesterByDate().Semester
	}
	var sc model.CourseSchedule
	err := db.Preload("Details").
		Where("teacher_id = ? AND semester = ? AND is_current = 1", teacherID, semester).
		Order("version DESC").First(&sc).Error
	if err != nil {
		return emptyScheduleShell(teacherID, "", semester)
	}
	return scheduleDetailPayload(&sc)
}

// MySchedule 当前登录用户的课表（无数据返回空壳）
func (s *Schedule) MySchedule(db *gorm.DB, viewer *model.User, semester string) map[string]interface{} {
	if semester == "" {
		semester = s.CurrentSemesterByDate().Semester
	}
	var sc model.CourseSchedule
	err := db.Preload("Details").
		Where("teacher_id = ? AND semester = ? AND is_current = 1", viewer.ID, semester).
		Order("version DESC").First(&sc).Error
	if err != nil {
		return emptyScheduleShell(viewer.ID, viewer.Username, semester)
	}
	return scheduleDetailPayload(&sc)
}

// Versions 指定学期版本历史（版本号倒序，对齐旧端）
func (s *Schedule) Versions(db *gorm.DB, semester string) ([]model.CourseScheduleVersion, error) {
	var list []model.CourseScheduleVersion
	err := db.Where("semester = ?", semester).Order("version DESC").Find(&list).Error
	return list, err
}

// TeacherStatus 教师课表同步状态（semester -> teacher_id -> 状态）
func (s *Schedule) TeacherStatus(db *gorm.DB, semester string) (map[int]map[string]interface{}, error) {
	type row struct {
		TeacherID   int
		Version     int
		CrawlTime   *time.Time
		CourseCount int64
	}
	var rows []row
	err := db.Table("course_schedule cs").
		Select("cs.teacher_id, cs.version, cs.crawl_time, COUNT(d.id) AS course_count").
		Joins("LEFT JOIN course_schedule_detail d ON d.schedule_id = cs.id").
		Where("cs.semester = ? AND cs.is_current = 1 AND cs.teacher_id IS NOT NULL", semester).
		Group("cs.id, cs.teacher_id, cs.version, cs.crawl_time").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[int]map[string]interface{}{}
	for _, r := range rows {
		var crawl interface{}
		if r.CrawlTime != nil {
			crawl = r.CrawlTime.Format("2006-01-02T15:04:05")
		}
		out[r.TeacherID] = map[string]interface{}{
			"version": r.Version, "crawl_time": crawl,
			"has_data": r.CourseCount > 0, "synced": true,
		}
	}
	return out, nil
}

// Semesters 学期列表（有明细的学期 + 已配置学期；无数据给默认）
func (s *Schedule) Semesters(db *gorm.DB) ([]string, error) {
	set := map[string]bool{}
	var sems []string
	db.Table("course_schedule cs").
		Joins("WHERE EXISTS (SELECT 1 FROM course_schedule_detail d WHERE d.schedule_id = cs.id)").
		Distinct().Pluck("cs.semester", &sems)
	for _, s := range sems {
		if s != "" {
			set[s] = true
		}
	}
	var cfgs []model.SemesterConfig
	db.Find(&cfgs)
	for _, c := range cfgs {
		if c.Semester != "" {
			set[c.Semester] = true
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	if len(out) == 0 {
		now := time.Now()
		var y1, y2 int
		if now.Month() >= 8 {
			y1, y2 = now.Year(), now.Year()+1
		} else if now.Month() >= 2 {
			y1, y2 = now.Year()-1, now.Year()
		} else {
			y1, y2 = now.Year()-1, now.Year()
		}
		out = []string{fmt.Sprintf("%d-%d-2", y1, y2), fmt.Sprintf("%d-%d-1", y1, y2)}
	}
	sort.Strings(out)
	// 倒序
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// SemesterStats 学期统计
func (s *Schedule) SemesterStats(db *gorm.DB, semester string) (map[string]interface{}, error) {
	if semester == "" {
		semester = s.CurrentSemesterByDate().Semester
	}
	var teacherCount int64
	db.Model(&model.CourseSchedule{}).
		Where("semester = ? AND is_current = 1", semester).Count(&teacherCount)

	var courseCount int64
	db.Model(&model.CourseScheduleDetail{}).
		Where("schedule_id IN (SELECT id FROM course_schedule WHERE semester = ? AND is_current = 1)", semester).
		Count(&courseCount)

	var latest model.CourseScheduleVersion
	err := db.Where("semester = ?", semester).Order("version DESC").First(&latest).Error
	var curVersion int
	var lastCrawl interface{}
	if err == nil {
		curVersion = latest.Version
		if latest.CrawlTime != nil {
			lastCrawl = latest.CrawlTime.ToTime().Format("2006-01-02T15:04:05")
		}
	}
	return map[string]interface{}{
		"semester": semester, "teacher_count": teacherCount, "course_count": courseCount,
		"current_version": curVersion, "last_crawl_time": lastCrawl,
	}, nil
}

// SemesterConfigList 全部学期配置（按学期倒序，对齐旧端）
func (s *Schedule) SemesterConfigList(db *gorm.DB) ([]model.SemesterConfig, error) {
	var list []model.SemesterConfig
	err := db.Order("semester DESC").Find(&list).Error
	return list, err
}

// SemesterConfigBySemester 指定学期配置
func (s *Schedule) SemesterConfigBySemester(db *gorm.DB, semester string) (*model.SemesterConfig, error) {
	var cfg model.SemesterConfig
	if err := db.Where("semester = ?", semester).First(&cfg).Error; err != nil {
		return nil, nil
	}
	return &cfg, nil
}

// CreateSemesterConfig 新建学期配置（is_current 互斥）
func (s *Schedule) CreateSemesterConfig(db *gorm.DB, semester string, startDate time.Time, weeks int, isCurrent bool) (*model.SemesterConfig, error) {
	existing, _ := s.SemesterConfigBySemester(db, semester)
	if existing != nil {
		return nil, errors.New("该学期配置已存在")
	}
	return s.upsertSemesterConfig(db, nil, semester, &startDate, &weeks, &isCurrent)
}

// UpdateSemesterConfig 更新学期配置（不存在则创建，对齐旧端 update_config 的 upsert 行为）
func (s *Schedule) UpdateSemesterConfig(db *gorm.DB, semester string, startDate *time.Time, weeks *int, isCurrent *bool) (*model.SemesterConfig, error) {
	existing, _ := s.SemesterConfigBySemester(db, semester)
	return s.upsertSemesterConfig(db, existing, semester, startDate, weeks, isCurrent)
}

// DeleteSemesterConfig 删除学期配置（当前学期不可删除）
func (s *Schedule) DeleteSemesterConfig(db *gorm.DB, semester string) error {
	var cfg model.SemesterConfig
	if err := db.Where("semester = ?", semester).First(&cfg).Error; err != nil {
		return errors.New("学期配置不存在")
	}
	if cfg.IsCurrent {
		return errors.New("当前学期配置不可删除")
	}
	return db.Delete(&model.SemesterConfig{}, cfg.ID).Error
}

func (s *Schedule) upsertSemesterConfig(db *gorm.DB, existing *model.SemesterConfig, semester string, startDate *time.Time, weeks *int, isCurrent *bool) (*model.SemesterConfig, error) {
	var cfg model.SemesterConfig
	err := db.Transaction(func(tx *gorm.DB) error {
		if existing == nil {
			w := defaultSemesterWeeks
			if weeks != nil {
				w = *weeks
			}
			cfg = model.SemesterConfig{Semester: semester, StartDate: model.LocalDate(*startDate), Weeks: w, IsCurrent: false}
			if isCurrent != nil {
				cfg.IsCurrent = *isCurrent
			}
			if cfg.IsCurrent {
				if err := tx.Model(&model.SemesterConfig{}).Where("is_current = 1").Update("is_current", false).Error; err != nil {
					return err
				}
			}
			return tx.Create(&cfg).Error
		}
		cfg = *existing
		updates := map[string]interface{}{}
		if startDate != nil {
			updates["start_date"] = *startDate
			cfg.StartDate = model.LocalDate(*startDate)
		}
		if weeks != nil {
			updates["weeks"] = *weeks
			cfg.Weeks = *weeks
		}
		if isCurrent != nil && *isCurrent {
			if err := tx.Model(&model.SemesterConfig{}).Where("is_current = 1").Update("is_current", false).Error; err != nil {
				return err
			}
			updates["is_current"] = true
			cfg.IsCurrent = true
		} else if isCurrent != nil {
			updates["is_current"] = *isCurrent
			cfg.IsCurrent = *isCurrent
		}
		if len(updates) > 0 {
			return tx.Model(&model.SemesterConfig{}).Where("id = ?", existing.ID).Updates(updates).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
