package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// Stats 统计业务
type Stats struct{}

func NewStats() *Stats { return &Stats{} }

// TeacherStatItem 教师统计输出（对齐旧端字段）
type TeacherStatItem struct {
	TeacherID        int     `json:"teacher_id"`
	TeacherName      string  `json:"teacher_name"`
	UserNo           string  `json:"user_no"`
	CollegeName      *string `json:"college_name"`
	TotalTasks       int64   `json:"total_tasks"`
	EvaluatedTasks   int64   `json:"evaluated_tasks"`
	PendingTasks     int64   `json:"pending_tasks"`
	TotalEvaluations int     `json:"total_evaluations"`
	AverageScore     float64 `json:"average_score"`
	EvaluationRate   float64 `json:"evaluation_rate"`
}

// TeacherStatsFilters 教师统计筛选
type TeacherStatsFilters struct {
	CollegeIDs     []int
	Keyword        string
	EvaluatorRoles []string
	Start, End     *time.Time
	Page, PageSize int
}

// TeacherStats 教师评教统计（分页，仅含实际有评教记录的教师，对齐旧端）
func (s *Stats) TeacherStats(db *gorm.DB, viewer *model.User, f TeacherStatsFilters) ([]TeacherStatItem, int64, error) {
	// 未指定日期时，默认按当前学期汇总
	if f.Start == nil && f.End == nil {
		f.Start, f.End = currentSemesterRange(db)
	}
	// 有评教记录的教师 ID（按角色/日期筛选记录集）
	rq := db.Table("evaluation_record r").Joins("JOIN evaluation_task t ON t.id = r.task_id").Where("r.is_deleted = 0")
	if len(f.EvaluatorRoles) > 0 {
		rq = rq.Where("r.evaluator_role IN ?", f.EvaluatorRoles)
	}
	if f.Start != nil {
		rq = rq.Where("r.submit_time >= ?", *f.Start)
	}
	if f.End != nil {
		rq = rq.Where("r.submit_time < ?", *f.End)
	}
	var teacherIDs []int
	rq.Distinct().Pluck("t.teacher_id", &teacherIDs)
	if len(teacherIDs) == 0 {
		return []TeacherStatItem{}, 0, nil
	}

	uq := db.Model(&model.User{}).Where("status = 1 AND id IN ?", teacherIDs)
	// 数据范围：教师只见自己；其余按学院范围
	if viewer.HasRole(model.RoleTeacher) && !IsAdminRole(viewer) && !IsSupervisor(viewer) {
		uq = uq.Where("id = ?", viewer.ID)
	} else {
		scope := AccessibleCollegeIDs(viewer)
		if scope != nil {
			if len(scope) == 0 {
				return []TeacherStatItem{}, 0, nil
			}
			uq = uq.Where("college_id IN ?", scope)
		}
	}
	if len(f.CollegeIDs) > 0 {
		uq = uq.Where("college_id IN ?", f.CollegeIDs)
	}
	if f.Keyword != "" {
		uq = uq.Where("username LIKE ?", "%"+f.Keyword+"%")
	}

	var total int64
	if err := uq.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var teachers []model.User
	if err := uq.Preload("College").Order("id ASC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&teachers).Error; err != nil {
		return nil, 0, err
	}

	scoreCodes := scoreDimCodeSet(db)
	out := make([]TeacherStatItem, 0, len(teachers))
	for _, t := range teachers {
		var tasks []model.EvaluationTask
		db.Where("teacher_id = ?", t.ID).Find(&tasks)
		taskIDs := make([]int, 0, len(tasks))
		for _, tk := range tasks {
			taskIDs = append(taskIDs, tk.ID)
		}

		var records []model.EvaluationRecord
		if len(taskIDs) > 0 {
			recq := db.Where("task_id IN ? AND is_deleted = 0", taskIDs)
			if len(f.EvaluatorRoles) > 0 {
				recq = recq.Where("evaluator_role IN ?", f.EvaluatorRoles)
			}
			if f.Start != nil {
				recq = recq.Where("submit_time >= ?", *f.Start)
			}
			if f.End != nil {
				recq = recq.Where("submit_time < ?", *f.End)
			}
			recq.Find(&records)
		}
		if len(records) == 0 {
			continue // 旧端：无符合记录的教师跳过
		}

		evaluated, pending := int64(0), int64(0)
		for _, tk := range tasks {
			switch tk.Status {
			case model.TaskStatusEvaluated:
				evaluated++
			case model.TaskStatusPending:
				pending++
			}
		}
		rate := 0.0
		if len(tasks) > 0 {
			rate = round2(float64(evaluated) / float64(len(tasks)) * 100)
		}
		var collegeName *string
		if t.College != nil {
			collegeName = &t.College.Name
		}
		out = append(out, TeacherStatItem{
			TeacherID: t.ID, TeacherName: t.Username, UserNo: t.UserNo,
			CollegeName: collegeName,
			TotalTasks:  int64(len(tasks)), EvaluatedTasks: evaluated, PendingTasks: pending,
			TotalEvaluations: len(records), AverageScore: recordsAvgScore(records, scoreCodes),
			EvaluationRate: rate,
		})
	}
	return out, total, nil
}

// Overview 首页大盘（返回结构与旧端一致：users/organization/tasks/evaluations 嵌套）
// semester 为空时取当前学期；tasks/evaluations 按学期 class_time 范围统计（对齐 CollegeStats 口径）
func (s *Stats) Overview(db *gorm.DB, viewer *model.User, semester string) (map[string]interface{}, error) {
	if semester == "" {
		semester, _ = currentSemesterOf(db)
	}
	start, end := semesterRangeOf(db, semester)

	// 数据范围
	scope := AccessibleCollegeIDs(viewer)
	limitTeacher := func(q *gorm.DB) *gorm.DB {
		if scope != nil {
			if len(scope) == 0 {
				return q.Where("1 = 0")
			}
			return q.Where("college_id IN ?", scope)
		}
		return q
	}

	// 用户统计（含多角色）
	var totalUsers, totalTeachers, totalSupervisors int64
	teacherCond := "(user.role = 'teacher' OR EXISTS (SELECT 1 FROM user_role ur WHERE ur.user_id = user.id AND ur.role = 'teacher'))"
	supervisorCond := "(user.role IN ('supervisor','school_supervisor','college_supervisor') OR EXISTS (SELECT 1 FROM user_role ur WHERE ur.user_id = user.id AND ur.role IN ('supervisor','school_supervisor','college_supervisor')))"
	limitTeacher(db.Table("user").Where("status = 1")).Count(&totalUsers)
	limitTeacher(db.Table("user").Where("status = 1 AND " + teacherCond)).Count(&totalTeachers)
	limitTeacher(db.Table("user").Where("status = 1 AND " + supervisorCond)).Count(&totalSupervisors)

	// 组织架构
	var totalCampuses, totalColleges int64
	db.Table("campus").Where("status = 1").Count(&totalCampuses)
	collegeQ := db.Table("college").Where("status = 1")
	if scope != nil {
		if len(scope) == 0 {
			collegeQ = collegeQ.Where("1 = 0")
		} else {
			collegeQ = collegeQ.Where("id IN ?", scope)
		}
	}
	collegeQ.Count(&totalColleges)

	// 任务统计（按学期 class_time 范围）
	taskQ := db.Table("evaluation_task").Where("is_deleted = 0")
	if scope != nil {
		if len(scope) == 0 {
			taskQ = taskQ.Where("1 = 0")
		} else {
			taskQ = taskQ.Where("teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", scope)
		}
	}
	if start != nil {
		taskQ = taskQ.Where("class_time >= ?", *start)
	}
	if end != nil {
		taskQ = taskQ.Where("class_time < ?", *end)
	}
	var totalTasks, evaluatedTasks, pendingTasks int64
	taskQ.Session(&gorm.Session{}).Count(&totalTasks)
	taskQ.Session(&gorm.Session{}).Where("status = 2").Count(&evaluatedTasks)
	taskQ.Session(&gorm.Session{}).Where("status = 1").Count(&pendingTasks)

	// 评教记录（按学期归属任务统计）
	var totalEvaluations int64
	evalQ := db.Table("evaluation_record r").
		Joins("JOIN evaluation_task t ON t.id = r.task_id").
		Where("r.is_deleted = 0")
	if scope != nil {
		if len(scope) == 0 {
			evalQ = evalQ.Where("1 = 0")
		} else {
			evalQ = evalQ.Where("t.teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", scope)
		}
	}
	if start != nil {
		evalQ = evalQ.Where("t.class_time >= ?", *start)
	}
	if end != nil {
		evalQ = evalQ.Where("t.class_time < ?", *end)
	}
	evalQ.Count(&totalEvaluations)

	evalRate := 0.0
	if totalTasks > 0 {
		evalRate = round2(float64(evaluatedTasks) / float64(totalTasks) * 100)
	}

	return map[string]interface{}{
		"semester": semester,
		"users": map[string]interface{}{
			"total": totalUsers, "teachers": totalTeachers, "supervisors": totalSupervisors,
		},
		"organization": map[string]interface{}{
			"campuses": totalCampuses, "colleges": totalColleges,
		},
		"tasks": map[string]interface{}{
			"total": totalTasks, "evaluated": evaluatedTasks,
			"pending": pendingTasks, "evaluation_rate": evalRate,
		},
		"evaluations": map[string]interface{}{
			"total": totalEvaluations,
		},
	}, nil
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// ---------- 统计通用辅助 ----------

// currentSemesterOf 当前学期（配置优先，否则按日期推算）
func currentSemesterOf(db *gorm.DB) (string, *time.Time) {
	var cfg model.SemesterConfig
	if err := db.Where("is_current = 1").First(&cfg).Error; err == nil {
		st := cfg.StartDate.ToTime()
		return cfg.Semester, &st
	}
	now := time.Now()
	y := now.Year()
	switch {
	case now.Month() >= 8:
		return fmt.Sprintf("%d-%d-1", y, y+1), timePtr(time.Date(y, 9, 1, 0, 0, 0, 0, time.Local))
	case now.Month() >= 2:
		return fmt.Sprintf("%d-%d-2", y-1, y), timePtr(time.Date(y, 2, 1, 0, 0, 0, 0, time.Local))
	default:
		return fmt.Sprintf("%d-%d-1", y-1, y), timePtr(time.Date(y-1, 9, 1, 0, 0, 0, 0, time.Local))
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// semesterRange 学期代码 -> 起止日期
func semesterRange(semester string) (*time.Time, *time.Time) {
	var sy, ey, term int
	if _, err := fmt.Sscanf(semester, "%d-%d-%d", &sy, &ey, &term); err != nil {
		return nil, nil
	}
	if term == 1 {
		return timePtr(time.Date(sy, 9, 1, 0, 0, 0, 0, time.Local)),
			timePtr(time.Date(ey, 2, 1, 0, 0, 0, 0, time.Local))
	}
	return timePtr(time.Date(ey, 2, 1, 0, 0, 0, 0, time.Local)),
		timePtr(time.Date(ey, 9, 1, 0, 0, 0, 0, time.Local))
}

// semesterRangeOf 学期代码 -> 起止日期（优先取 semester_config 开学日期，未配置回退默认推断）
func semesterRangeOf(db *gorm.DB, semester string) (*time.Time, *time.Time) {
	if semester == "" {
		semester, _ = currentSemesterOf(db)
	}
	var cfg model.SemesterConfig
	if err := db.Where("semester = ?", semester).First(&cfg).Error; err == nil {
		start := cfg.StartDate.ToTime()
		return timePtr(start), timePtr(start.AddDate(0, 0, semesterDurationDays(cfg.Weeks)))
	}
	return semesterRange(semester)
}

// currentSemesterRange 当前学期默认区间（后端兜底：未传日期时按当前学期汇总）
func currentSemesterRange(db *gorm.DB) (*time.Time, *time.Time) {
	semester, _ := currentSemesterOf(db)
	return semesterRangeOf(db, semester)
}

// teachersWithCourses 学期内有课（有实际课程明细）的教师集合
func teachersWithCourses(db *gorm.DB, teacherIDs []int, semester string) map[int]bool {
	set := map[int]bool{}
	if len(teacherIDs) == 0 {
		return set
	}
	type row struct {
		TeacherID int
	}
	var rows []row
	db.Table("course_schedule cs").
		Joins("JOIN course_schedule_detail d ON d.schedule_id = cs.id").
		Where("cs.teacher_id IN ? AND cs.semester = ? AND cs.is_current = 1", teacherIDs, semester).
		Distinct().Select("cs.teacher_id").Scan(&rows)
	for _, r := range rows {
		set[r.TeacherID] = true
	}
	return set
}

// scoreDimCodeSet 启用的 score 维度编码集合
func scoreDimCodeSet(db *gorm.DB) map[string]bool {
	var codes []string
	db.Model(&model.EvaluationDimension{}).Where("status = 1 AND field_type = ?", model.FieldScore).
		Pluck("code", &codes)
	m := map[string]bool{}
	for _, c := range codes {
		m[c] = true
	}
	return m
}

// recordsAvgScore 记录平均分：逐条按 score 维度求和，再对记录求均值（对齐旧端 _calc_average_score，无数据返回 0）
func recordsAvgScore(recs []model.EvaluationRecord, scoreCodes map[string]bool) float64 {
	if len(recs) == 0 {
		return 0
	}
	var sum float64
	var cnt int
	for i := range recs {
		var values map[string]interface{}
		if len(recs[i].DimensionValues) == 0 {
			continue
		}
		if err := json.Unmarshal(recs[i].DimensionValues, &values); err != nil {
			continue
		}
		total := 0.0
		for code, v := range values {
			if scoreCodes[code] {
				if f, ok := toFloat(v); ok {
					total += f
				}
			}
		}
		if total > 0 {
			sum += total
			cnt++
		}
	}
	if cnt == 0 {
		return 0
	}
	return round2(sum / float64(cnt))
}

// teacherCondAlias 教师条件（主角色或多角色关联），alias 为 user 表别名
func teacherCondAlias(alias string) string {
	return "(" + alias + ".role = 'teacher' OR EXISTS (SELECT 1 FROM user_role ur WHERE ur.user_id = " + alias + ".id AND ur.role = 'teacher'))"
}

// collegeScopeFor 统计用的学院范围：教师仅主学院；nil 表示全校
func collegeScopeFor(viewer *model.User) []int {
	if viewer.HasRole(model.RoleTeacher) && !IsAdminRole(viewer) && !IsSupervisor(viewer) {
		if viewer.CollegeID == nil {
			return []int{}
		}
		return []int{*viewer.CollegeID}
	}
	return AccessibleCollegeIDs(viewer)
}

// taskStatsInput 任务统计条件
type taskStatsInput struct {
	TeacherIDs     []int
	ClassStart     *time.Time
	ClassEnd       *time.Time
	EvaluatorRoles []string
}

// taskStatsFor 任务与评教统计（返回任务数、已评、待评、记录数、任务ID）
func taskStatsFor(db *gorm.DB, in taskStatsInput) (total, evaluated, pending, evaluations int64, taskIDs []int) {
	q := db.Model(&model.EvaluationTask{}).Where("is_deleted = 0")
	if len(in.TeacherIDs) > 0 {
		q = q.Where("teacher_id IN ?", in.TeacherIDs)
	} else {
		return 0, 0, 0, 0, nil
	}
	if in.ClassStart != nil {
		q = q.Where("class_time >= ?", *in.ClassStart)
	}
	if in.ClassEnd != nil {
		q = q.Where("class_time < ?", *in.ClassEnd)
	}
	q.Session(&gorm.Session{}).Count(&total)
	q.Session(&gorm.Session{}).Where("status = 2").Count(&evaluated)
	q.Session(&gorm.Session{}).Where("status = 1").Count(&pending)
	q.Session(&gorm.Session{}).Pluck("id", &taskIDs)
	if len(taskIDs) > 0 {
		rq := db.Model(&model.EvaluationRecord{}).Where("task_id IN ? AND is_deleted = 0", taskIDs)
		if len(in.EvaluatorRoles) > 0 {
			rq = rq.Where("evaluator_role IN ?", in.EvaluatorRoles)
		}
		rq.Count(&evaluations)
	}
	return
}

// distinctTeachersWithTasks 有任务的去重教师数
func distinctTeachersWithTasks(db *gorm.DB, teacherIDs []int, start, end *time.Time) int64 {
	var cnt int64
	if len(teacherIDs) == 0 {
		return 0
	}
	q := db.Model(&model.EvaluationTask{}).Where("is_deleted = 0 AND teacher_id IN ?", teacherIDs)
	if start != nil {
		q = q.Where("class_time >= ?", *start)
	}
	if end != nil {
		q = q.Where("class_time < ?", *end)
	}
	q.Distinct().Count(&cnt)
	return cnt
}

// collegeTeacherIDs 学院下的在岗教师
func collegeTeacherIDs(db *gorm.DB, collegeIDs []int) ([]model.User, []int) {
	if len(collegeIDs) == 0 {
		return nil, nil
	}
	var users []model.User
	db.Where("status = 1 AND college_id IN ? AND "+teacherCondAlias("user"), collegeIDs).Find(&users)
	ids := make([]int, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return users, ids
}

// ---------- /stats/college ----------

// CollegeStats 学院评教统计
func (s *Stats) CollegeStats(db *gorm.DB, viewer *model.User, collegeID *int, semester string) (map[string]interface{}, error) {
	scope := collegeScopeFor(viewer)
	if scope != nil {
		if len(scope) == 0 {
			return nil, errors.New("您没有所属学院")
		}
		if collegeID != nil {
			allowed := false
			for _, id := range scope {
				if id == *collegeID {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, errors.New("无权访问该学院数据")
			}
		} else {
			collegeID = &scope[0]
		}
	}

	if semester == "" {
		semester, _ = currentSemesterOf(db)
	}
	start, end := semesterRangeOf(db, semester)

	collegeName := "全部学院"
	var users []model.User
	q := db.Where("status = 1 AND " + teacherCondAlias("user"))
	if collegeID != nil {
		var college model.College
		if err := db.First(&college, *collegeID).Error; err != nil {
			return nil, errors.New("学院不存在")
		}
		collegeName = college.Name
		q = q.Where("college_id = ?", *collegeID)
	} else if scope != nil {
		q = q.Where("college_id IN ?", scope)
	}
	q.Find(&users)
	teacherIDs := make([]int, 0, len(users))
	for _, u := range users {
		teacherIDs = append(teacherIDs, u.ID)
	}

	withCourses := teachersWithCourses(db, teacherIDs, semester)
	courseIDs := make([]int, 0, len(withCourses))
	for id := range withCourses {
		courseIDs = append(courseIDs, id)
	}

	total, evaluated, pending, evaluations, _ := taskStatsFor(db, taskStatsInput{
		TeacherIDs: courseIDs, ClassStart: start, ClassEnd: end,
	})
	withTasks := distinctTeachersWithTasks(db, courseIDs, start, end)

	coverage := 0.0
	if len(courseIDs) > 0 {
		coverage = round2(float64(withTasks) / float64(len(courseIDs)) * 100)
	}
	evalRate := 0.0
	if total > 0 {
		evalRate = round2(float64(evaluated) / float64(total) * 100)
	}
	return map[string]interface{}{
		"college_id": collegeID, "college_name": collegeName,
		"teacher_count": len(courseIDs),
		"total_tasks":   total, "evaluated_tasks": evaluated, "pending_tasks": pending,
		"total_evaluations": evaluations, "coverage_rate": coverage,
		"teachers_with_tasks": withTasks, "evaluation_rate": evalRate,
		"semester": semester,
	}, nil
}

// ---------- /stats/colleges ----------

// CollegeStatsFilters 学院统计列表筛选
type CollegeStatsFilters struct {
	CollegeIDs     []int
	EvaluatorRoles []string
	Semester       string
	Start, End     *time.Time
	Page, PageSize int
}

// CollegeStatsList 学院评教统计列表（分页，含教师明细）
func (s *Stats) CollegeStatsList(db *gorm.DB, viewer *model.User, f CollegeStatsFilters) ([]map[string]interface{}, int64, string, error) {
	scope := collegeScopeFor(viewer)

	cq := db.Model(&model.College{}).Where("status = 1")
	if scope != nil {
		if len(scope) == 0 {
			return []map[string]interface{}{}, 0, f.Semester, nil
		}
		cq = cq.Where("id IN ?", scope)
	}
	if len(f.CollegeIDs) > 0 {
		cq = cq.Where("id IN ?", f.CollegeIDs)
	}
	var total int64
	cq.Session(&gorm.Session{}).Count(&total)
	var colleges []model.College
	cq.Order("id ASC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&colleges)

	// 学期/时间范围
	semester := f.Semester
	var start, end *time.Time
	if f.Start != nil || f.End != nil {
		start, end = f.Start, f.End
		if semester == "" {
			if f.Start != nil {
				semester, _ = currentSemesterOf(db)
			} else {
				semester, _ = currentSemesterOf(db)
			}
		}
	} else {
		if semester == "" {
			semester, _ = currentSemesterOf(db)
		}
		start, end = semesterRangeOf(db, semester)
	}

	scoreCodes := scoreDimCodeSet(db)
	list := make([]map[string]interface{}, 0, len(colleges))
	for _, college := range colleges {
		users, ids := collegeTeacherIDs(db, []int{college.ID})
		withCourses := teachersWithCourses(db, ids, semester)
		courseIDs := make([]int, 0, len(withCourses))
		for id := range withCourses {
			courseIDs = append(courseIDs, id)
		}
		totalT, evaluatedT, pendingT, evals, taskIDs := taskStatsFor(db, taskStatsInput{
			TeacherIDs: courseIDs, ClassStart: start, ClassEnd: end, EvaluatorRoles: f.EvaluatorRoles,
		})
		withTasks := distinctTeachersWithTasks(db, courseIDs, start, end)

		// 已评/未评教师名单（旧端仅在导出接口附带；列表输出时由 handler 剔除）
		evaluatedTIDs := map[int]bool{}
		if len(taskIDs) > 0 {
			var tids []int
			db.Model(&model.EvaluationTask{}).Where("id IN ?", taskIDs).Distinct().Pluck("teacher_id", &tids)
			for _, id := range tids {
				evaluatedTIDs[id] = true
			}
		}
		evaluatedNames := make([]string, 0, len(users))
		unevaluatedNames := make([]string, 0, len(users))
		for _, t := range users {
			if evaluatedTIDs[t.ID] {
				evaluatedNames = append(evaluatedNames, t.Username)
			} else if withCourses[t.ID] {
				unevaluatedNames = append(unevaluatedNames, t.Username)
			}
		}

		var recs []model.EvaluationRecord
		if len(taskIDs) > 0 {
			db.Where("task_id IN ? AND is_deleted = 0", taskIDs).Find(&recs)
		}
		avg := recordsAvgScore(recs, scoreCodes)

		coverage := 0.0
		if len(courseIDs) > 0 {
			coverage = round2(float64(withTasks) / float64(len(courseIDs)) * 100)
		}
		evalRate := 0.0
		if totalT > 0 {
			evalRate = round2(float64(evaluatedT) / float64(totalT) * 100)
		}

		// 教师明细（全部教师，标注是否有课）
		details := make([]map[string]interface{}, 0, len(users))
		for _, t := range users {
			var tTaskIDs []int
			tq := db.Model(&model.EvaluationTask{}).Where("is_deleted = 0 AND teacher_id = ?", t.ID)
			if start != nil {
				tq = tq.Where("class_time >= ?", *start)
			}
			if end != nil {
				tq = tq.Where("class_time < ?", *end)
			}
			tq.Pluck("id", &tTaskIDs)
			var tRecs []model.EvaluationRecord
			if len(tTaskIDs) > 0 {
				trq := db.Where("task_id IN ? AND is_deleted = 0", tTaskIDs)
				if len(f.EvaluatorRoles) > 0 {
					trq = trq.Where("evaluator_role IN ?", f.EvaluatorRoles)
				}
				trq.Find(&tRecs)
			}
			tAvg := recordsAvgScore(tRecs, scoreCodes)
			details = append(details, map[string]interface{}{
				"teacher_id": t.ID, "teacher_name": t.Username, "user_no": t.UserNo,
				"total_evaluations": len(tRecs), "average_score": tAvg,
				"has_evaluation": len(tRecs) > 0, "has_courses": withCourses[t.ID],
			})
		}

		list = append(list, map[string]interface{}{
			"college_id": college.ID, "college_name": college.Name,
			"total_teacher_count": len(users), "teacher_count": len(courseIDs),
			"total_tasks": totalT, "evaluated_tasks": evaluatedT, "pending_tasks": pendingT,
			"total_evaluations": evals, "average_score": avg,
			"coverage_rate": coverage, "teachers_with_tasks": withTasks,
			"evaluation_rate": evalRate, "teacher_details": details, "semester": semester,
			"evaluated_teacher_names":   strings.Join(evaluatedNames, ","),
			"unevaluated_teacher_names": strings.Join(unevaluatedNames, ","),
		})
	}
	return list, total, semester, nil
}

// ---------- /stats/campus ----------

// CampusStats 校区评教统计
func (s *Stats) CampusStats(db *gorm.DB, viewer *model.User, campusID *int) (map[string]interface{}, error) {
	scope := collegeScopeFor(viewer)

	cq := db.Model(&model.College{}).Where("status = 1")
	campusName := "全部校区"
	if campusID != nil {
		var campus model.Campus
		if err := db.First(&campus, *campusID).Error; err != nil {
			return nil, errors.New("校区不存在")
		}
		campusName = campus.Name
		cq = cq.Where("campus_id = ?", *campusID)
	}
	if scope != nil {
		if len(scope) == 0 {
			cq = cq.Where("1 = 0")
		} else {
			// 教师角色限定其学院所在校区
			if viewer.HasRole(model.RoleTeacher) && !IsAdminRole(viewer) && !IsSupervisor(viewer) && viewer.CollegeID != nil {
				var college model.College
				if err := db.First(&college, *viewer.CollegeID).Error; err == nil && college.CampusID != nil {
					campusID = college.CampusID
					var campus model.Campus
					if err := db.First(&campus, *college.CampusID).Error; err == nil {
						campusName = campus.Name
					}
					cq = db.Model(&model.College{}).Where("status = 1 AND id = ?", *viewer.CollegeID)
				} else {
					cq = cq.Where("1 = 0")
				}
			} else {
				cq = cq.Where("id IN ?", scope)
			}
		}
	}
	var colleges []model.College
	cq.Find(&colleges)
	cids := make([]int, 0, len(colleges))
	for _, c := range colleges {
		cids = append(cids, c.ID)
	}

	_, teacherIDs := collegeTeacherIDs(db, cids)
	total, evaluated, pending, evaluations, _ := taskStatsFor(db, taskStatsInput{TeacherIDs: teacherIDs})
	evalRate := 0.0
	if total > 0 {
		evalRate = round2(float64(evaluated) / float64(total) * 100)
	}
	return map[string]interface{}{
		"campus_id": campusID, "campus_name": campusName,
		"college_count": len(colleges), "teacher_count": len(teacherIDs),
		"total_tasks": total, "evaluated_tasks": evaluated, "pending_tasks": pending,
		"total_evaluations": evaluations, "evaluation_rate": evalRate,
	}, nil
}

// ---------- /stats/college-teachers/{college_id} ----------

// CollegeTeacherDetails 指定学院教师评教详情
func (s *Stats) CollegeTeacherDetails(db *gorm.DB, viewer *model.User, collegeID int) (map[string]interface{}, error) {
	scope := collegeScopeFor(viewer)
	if scope != nil {
		allowed := false
		for _, id := range scope {
			if id == collegeID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("无权访问该学院数据")
		}
	}
	var college model.College
	if err := db.Where("status = 1").First(&college, collegeID).Error; err != nil {
		return nil, errors.New("学院不存在")
	}

	users, ids := collegeTeacherIDs(db, []int{collegeID})
	semester, _ := currentSemesterOf(db)
	withCourses := teachersWithCourses(db, ids, semester)
	scoreCodes := scoreDimCodeSet(db)

	details := make([]map[string]interface{}, 0, len(users))
	for _, t := range users {
		var taskIDs []int
		db.Model(&model.EvaluationTask{}).Where("is_deleted = 0 AND teacher_id = ?", t.ID).Pluck("id", &taskIDs)
		var received []model.EvaluationRecord
		if len(taskIDs) > 0 {
			db.Where("task_id IN ? AND is_deleted = 0", taskIDs).Find(&received)
		}
		var givenCnt int64
		db.Model(&model.EvaluationRecord{}).Where("evaluator_id = ? AND submit_time IS NOT NULL AND is_deleted = 0", t.ID).Count(&givenCnt)
		avg := recordsAvgScore(received, scoreCodes)
		details = append(details, map[string]interface{}{
			"teacher_id": t.ID, "teacher_name": t.Username, "user_no": t.UserNo,
			"be_evaluated_count": len(received), "evaluate_count": givenCnt,
			"average_score": avg, "has_evaluation": len(received) > 0,
			"has_courses": withCourses[t.ID],
		})
	}
	return map[string]interface{}{
		"college_id": collegeID, "college_name": college.Name,
		"teacher_count": len(users), "teacher_details": details,
	}, nil
}

// ---------- /stats/supervisors ----------

// PersonStatsFilters 督导/评教人统计筛选
type PersonStatsFilters struct {
	CollegeIDs     []int
	Keyword        string
	Start, End     *time.Time
	Page, PageSize int
}

// supervisorCond 督导角色条件
const supervisorRolesIn = "('supervisor','school_supervisor','college_supervisor')"

// SupervisorStats 督导评教统计列表（分页）
func (s *Stats) SupervisorStats(db *gorm.DB, viewer *model.User, f PersonStatsFilters) ([]map[string]interface{}, int64, error) {
	// 未指定日期时，默认按当前学期汇总
	if f.Start == nil && f.End == nil {
		f.Start, f.End = currentSemesterRange(db)
	}
	scope := collegeScopeFor(viewer)

	q := db.Model(&model.User{}).Where("user.status = 1 AND (user.role IN " + supervisorRolesIn +
		" OR EXISTS (SELECT 1 FROM user_role ur WHERE ur.user_id = user.id AND ur.role IN " + supervisorRolesIn + "))")
	if scope != nil {
		if len(scope) == 0 {
			return []map[string]interface{}{}, 0, nil
		}
		q = q.Where("(user.college_id IN ? OR EXISTS (SELECT 1 FROM user_college uc WHERE uc.user_id = user.id AND uc.college_id IN ?))",
			scope, scope)
	}
	if len(f.CollegeIDs) > 0 {
		q = q.Where("(user.college_id IN ? OR EXISTS (SELECT 1 FROM user_college uc WHERE uc.user_id = user.id AND uc.college_id IN ?))",
			f.CollegeIDs, f.CollegeIDs)
	}
	if f.Keyword != "" {
		q = q.Where("user.username LIKE ?", "%"+f.Keyword+"%")
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var supervisors []model.User
	err := q.Session(&gorm.Session{}).
		Preload("College").
		Order("user.id ASC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&supervisors).Error
	if err != nil {
		return nil, 0, err
	}

	scoreCodes := scoreDimCodeSet(db)
	out := make([]map[string]interface{}, 0, len(supervisors))
	for i := range supervisors {
		sp := &supervisors[i]
		recq := db.Model(&model.EvaluationRecord{}).
			Joins("JOIN evaluation_task t ON t.id = evaluation_record.task_id").
			Where("evaluator_id = ? AND evaluation_record.submit_time IS NOT NULL AND evaluation_record.is_deleted = 0", sp.ID)
		if f.Start != nil {
			recq = recq.Where("evaluation_record.submit_time >= ?", *f.Start)
		}
		if f.End != nil {
			recq = recq.Where("evaluation_record.submit_time < ?", *f.End)
		}
		var recs []model.EvaluationRecord
		recq.Find(&recs)

		teachers := map[int]bool{}
		var last *model.LocalTime
		for _, r := range recs {
			var tid int
			if err := db.Model(&model.EvaluationTask{}).Select("teacher_id").Where("id = ?", r.TaskID).Scan(&tid).Error; err == nil {
				teachers[tid] = true
			}
			if r.SubmitTime != nil && (last == nil || r.SubmitTime.ToTime().After(last.ToTime())) {
				last = r.SubmitTime
			}
		}
		var collegeName interface{}
		if sp.College != nil {
			collegeName = sp.College.Name
		}
		var lastStr interface{}
		if last != nil {
			lastStr = last.ToTime().Format("2006-01-02 15:04:05")
		}
		out = append(out, map[string]interface{}{
			"supervisor_id": sp.ID, "supervisor_name": sp.Username, "user_no": sp.UserNo,
			"college_name": collegeName, "total_evaluations": len(recs),
			"average_score":           recordsAvgScore(recs, scoreCodes),
			"evaluated_teacher_count": len(teachers), "last_evaluation_time": lastStr,
		})
	}
	return out, total, nil
}

// ---------- /stats/evaluators ----------

// EvaluatorStats 评教人统计列表（分页）
func (s *Stats) EvaluatorStats(db *gorm.DB, viewer *model.User, f PersonStatsFilters, evaluatorRoles []string) ([]map[string]interface{}, int64, error) {
	// 未指定日期时，默认按当前学期汇总
	if f.Start == nil && f.End == nil {
		f.Start, f.End = currentSemesterRange(db)
	}
	scope := collegeScopeFor(viewer)

	// 有提交记录的评教人 ID（按范围过滤）
	idq := db.Table("evaluation_record AS r").
		Joins("JOIN evaluation_task t ON t.id = r.task_id").
		Where("r.submit_time IS NOT NULL AND r.is_deleted = 0")
	if scope != nil {
		if len(scope) == 0 {
			return []map[string]interface{}{}, 0, nil
		}
		idq = idq.Where("t.teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", scope)
	}
	if f.Start != nil {
		idq = idq.Where("r.submit_time >= ?", *f.Start)
	}
	if f.End != nil {
		idq = idq.Where("r.submit_time < ?", *f.End)
	}
	if len(evaluatorRoles) > 0 {
		idq = idq.Where("r.evaluator_role IN ?", evaluatorRoles)
	}
	var evaluatorIDs []int
	idq.Distinct().Pluck("r.evaluator_id", &evaluatorIDs)
	if len(evaluatorIDs) == 0 {
		return []map[string]interface{}{}, 0, nil
	}

	uq := db.Where("status = 1 AND id IN ?", evaluatorIDs)
	if len(f.CollegeIDs) > 0 {
		uq = uq.Where("college_id IN ?", f.CollegeIDs)
	}
	if f.Keyword != "" {
		uq = uq.Where("username LIKE ?", "%"+f.Keyword+"%")
	}
	var total int64
	uq.Session(&gorm.Session{}).Count(&total)
	var evaluators []model.User
	uq.Preload("College").Order("id ASC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&evaluators)

	scoreCodes := scoreDimCodeSet(db)
	out := make([]map[string]interface{}, 0, len(evaluators))
	for i := range evaluators {
		ev := &evaluators[i]
		recq := db.Model(&model.EvaluationRecord{}).
			Joins("JOIN evaluation_task t ON t.id = evaluation_record.task_id").
			Where("evaluator_id = ? AND evaluation_record.submit_time IS NOT NULL AND evaluation_record.is_deleted = 0", ev.ID)
		if scope != nil {
			recq = recq.Where("t.teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", scope)
		}
		if f.Start != nil {
			recq = recq.Where("evaluation_record.submit_time >= ?", *f.Start)
		}
		if f.End != nil {
			recq = recq.Where("evaluation_record.submit_time < ?", *f.End)
		}
		if len(evaluatorRoles) > 0 {
			recq = recq.Where("evaluation_record.evaluator_role IN ?", evaluatorRoles)
		}
		var recs []model.EvaluationRecord
		recq.Order("evaluation_record.submit_time DESC").Find(&recs)
		if len(recs) == 0 {
			continue
		}
		teachers := map[int]bool{}
		for _, r := range recs {
			var tid int
			if err := db.Model(&model.EvaluationTask{}).Select("teacher_id").Where("id = ?", r.TaskID).Scan(&tid).Error; err == nil {
				teachers[tid] = true
			}
		}
		var collegeName interface{}
		if ev.College != nil {
			collegeName = ev.College.Name
		}
		out = append(out, map[string]interface{}{
			"evaluator_id": ev.ID, "evaluator_name": ev.Username, "user_no": ev.UserNo,
			"evaluator_role": model.RoleName(recs[0].EvaluatorRole),
			"college_name":   collegeName, "total_evaluations": len(recs),
			"average_score":           recordsAvgScore(recs, scoreCodes),
			"evaluated_teacher_count": len(teachers),
		})
	}
	return out, total, nil
}

// ---------- /stats/unteached-teachers ----------

// UnteachedTeachers 未被听课教师列表
func (s *Stats) UnteachedTeachers(db *gorm.DB, viewer *model.User, collegeID *int, page, pageSize int) ([]map[string]interface{}, int64, error) {
	scope := collegeScopeFor(viewer)
	target := collegeID
	if target != nil && scope != nil {
		allowed := false
		for _, id := range scope {
			if id == *target {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, 0, errors.New("无权访问该学院数据")
		}
	} else if target == nil && scope != nil {
		if len(scope) == 0 {
			return nil, 0, errors.New("您没有管理的学院")
		}
		if len(scope) > 1 {
			return nil, 0, errors.New("您有多个学院权限，请指定 college_id 参数")
		}
		target = &scope[0]
	}

	q := db.Model(&model.User{}).Where("status = 1 AND " + teacherCondAlias("user") +
		" AND NOT EXISTS (SELECT 1 FROM evaluation_task t WHERE t.teacher_id = user.id)")
	if target != nil {
		q = q.Where("college_id = ?", *target)
	} else if scope != nil {
		q = q.Where("college_id IN ?", scope)
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	err := q.Preload("College").Preload("ResearchRoom").
		Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	list := make([]map[string]interface{}, 0, len(users))
	for _, t := range users {
		var collegeName, roomName interface{}
		if t.College != nil {
			collegeName = t.College.Name
		}
		if t.ResearchRoom != nil {
			roomName = t.ResearchRoom.Name
		}
		var createTime interface{}
		if t.CreateTime != nil {
			createTime = t.CreateTime.ToTime().Format("2006-01-02 15:04:05")
		}
		list = append(list, map[string]interface{}{
			"id": t.ID, "user_no": t.UserNo, "username": t.Username,
			"college_id": t.CollegeID, "college_name": collegeName,
			"research_room_id": t.ResearchRoomID, "research_room_name": roomName,
			"status": t.Status, "create_time": createTime,
		})
	}
	return list, total, nil
}

// ftimePtr 时间指针格式化
func ftimePtr(t *model.LocalTime) interface{} {
	if t == nil {
		return nil
	}
	return t.ToTime().Format("2006-01-02 15:04:05")
}

// ---------- /stats/evaluation-records ----------

// RecordStatsFilters 评教记录统计筛选
type RecordStatsFilters struct {
	CollegeIDs     []int
	TeacherID      *int
	EvaluatorID    *int
	Keyword        string
	EvaluatorRoles []string
	Start, End     *time.Time
	Page, PageSize int
}

// studentCountFor 课表中学生人数（当前学期、模糊匹配课程名）
func studentCountFor(db *gorm.DB, teacherID int, courseName string) (int, bool) {
	if teacherID == 0 || courseName == "" {
		return 0, false
	}
	var sem model.SemesterConfig
	if err := db.Where("is_current = 1").First(&sem).Error; err != nil {
		return 0, false
	}
	var schedule model.CourseSchedule
	if err := db.Where("teacher_id = ? AND semester = ? AND is_current = 1", teacherID, sem.Semester).
		First(&schedule).Error; err != nil {
		return 0, false
	}
	var details []model.CourseScheduleDetail
	db.Where("schedule_id = ?", schedule.ID).Find(&details)
	var matched *model.CourseScheduleDetail
	for i := range details {
		cn := details[i].CourseName
		if cn == courseName || strings.Contains(cn, courseName) || strings.Contains(courseName, cn) {
			matched = &details[i]
			break
		}
	}
	if matched == nil && len(details) > 0 {
		matched = &details[0]
	}
	if matched == nil {
		return 0, false
	}
	return ParseStudentCount(matched.ClassInfo)
}

// EvaluationRecordsStats 评教记录合并统计（分页）
func (s *Stats) EvaluationRecordsStats(db *gorm.DB, viewer *model.User, f RecordStatsFilters) ([]map[string]interface{}, int64, error) {
	// 未指定日期时，默认按当前学期汇总
	if f.Start == nil && f.End == nil {
		f.Start, f.End = currentSemesterRange(db)
	}
	scope := collegeScopeFor(viewer)
	isPureTeacher := viewer.HasRole(model.RoleTeacher) && !IsAdminRole(viewer) && !IsSupervisor(viewer)

	q := db.Table("evaluation_record AS r").
		Joins("JOIN evaluation_task t ON t.id = r.task_id").
		Joins("JOIN `user` u ON u.id = t.teacher_id").
		Where("r.submit_time IS NOT NULL AND r.is_deleted = 0")
	if isPureTeacher {
		q = q.Where("(t.teacher_id = ? OR r.evaluator_id = ?)", viewer.ID, viewer.ID)
	} else if CanViewOthersEvaluation(db, viewer) {
		// 系统管理员/被分配"查看他人评教详情"权限的角色：按学院数据范围
		if scope != nil {
			if len(scope) == 0 {
				return []map[string]interface{}{}, 0, nil
			}
			q = q.Where("u.college_id IN ?", scope)
		}
	} else {
		// 未被分配查看权限（含督导）：仅自己评的 + 评给自己的
		q = q.Where("(t.teacher_id = ? OR r.evaluator_id = ?)", viewer.ID, viewer.ID)
	}
	if len(f.CollegeIDs) > 0 {
		q = q.Where("u.college_id IN ?", f.CollegeIDs)
	}
	if f.TeacherID != nil {
		q = q.Where("t.teacher_id = ?", *f.TeacherID)
	}
	if f.EvaluatorID != nil {
		q = q.Where("r.evaluator_id = ?", *f.EvaluatorID)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		q = q.Where("u.username LIKE ? OR t.course_name LIKE ?", kw, kw)
	}
	if len(f.EvaluatorRoles) > 0 {
		q = q.Where("r.evaluator_role IN ?", f.EvaluatorRoles)
	}
	if f.Start != nil {
		q = q.Where("r.submit_time >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("r.submit_time < ?", *f.End)
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var recs []model.EvaluationRecord
	if err := q.Session(&gorm.Session{}).
		Select("r.*").
		Order("r.submit_time DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Scan(&recs).Error; err != nil {
		return nil, 0, err
	}

	// 批量装配关联数据
	taskIDs := map[int]bool{}
	for _, r := range recs {
		taskIDs[r.TaskID] = true
	}
	var tasks []model.EvaluationTask
	db.Where("id IN ?", keysOf(taskIDs)).Find(&tasks)
	taskMap := map[int]model.EvaluationTask{}
	teacherIDs := map[int]bool{}
	for _, t := range tasks {
		taskMap[t.ID] = t
		teacherIDs[t.TeacherID] = true
	}
	var users []model.User
	db.Preload("College").Where("id IN ?", keysOf(teacherIDs)).Find(&users)
	userMap := map[int]model.User{}
	campusIDs := map[int]bool{}
	for _, u := range users {
		userMap[u.ID] = u
		if u.College != nil && u.College.CampusID != nil {
			campusIDs[*u.College.CampusID] = true
		}
	}
	campusNames := map[int]string{}
	if len(campusIDs) > 0 {
		var campuses []model.Campus
		db.Where("id IN ?", keysOf(campusIDs)).Find(&campuses)
		for _, cp := range campuses {
			campusNames[cp.ID] = cp.Name
		}
	}

	// score 维度满分映射
	var dims []model.EvaluationDimension
	db.Where("status = 1 AND field_type = ?", model.FieldScore).Find(&dims)
	maxScoreOf := map[string]float64{}
	for _, d := range dims {
		var cfg struct {
			MaxScore float64 `json:"max_score"`
		}
		_ = json.Unmarshal(d.FieldConfig, &cfg)
		if cfg.MaxScore == 0 {
			cfg.MaxScore = 100
		}
		maxScoreOf[d.Code] = cfg.MaxScore
	}

	list := make([]map[string]interface{}, 0, len(recs))
	viewAll := CanViewOthersEvaluation(db, viewer)
	for i := range recs {
		r := &recs[i]
		task := taskMap[r.TaskID]
		teacher := userMap[task.TeacherID]

		// 评教人姓名脱敏
		canSee := viewAll
		if !canSee && IsSupervisor(viewer) {
			canSee = r.EvaluatorID != nil && *r.EvaluatorID == viewer.ID
		}
		if !canSee && viewer.HasRole(model.RoleTeacher) {
			canSee = !r.IsAnonymous
		}
		if !canSee && r.EvaluatorID != nil && *r.EvaluatorID == viewer.ID {
			canSee = true
		}
		evaluatorName := interface{}(nil)
		if canSee {
			evaluatorName = r.EvaluatorName
		} else if r.IsAnonymous {
			evaluatorName = "匿名"
		}

		// 总分与满分
		var values map[string]interface{}
		if len(r.DimensionValues) > 0 {
			_ = json.Unmarshal(r.DimensionValues, &values)
		}
		totalScore, maxTotal := 0.0, 0.0
		for code, v := range values {
			if max, ok := maxScoreOf[code]; ok {
				if fv, isNum := toFloat(v); isNum {
					totalScore += fv
					maxTotal += max
				}
			}
		}

		collegeName := interface{}(nil)
		campusName := ""
		if teacher.College != nil {
			collegeName = teacher.College.Name
			if teacher.College.CampusID != nil {
				campusName = campusNames[*teacher.College.CampusID]
			}
		}

		studentCount, hasStudents := studentCountFor(db, task.TeacherID, task.CourseName)
		attendanceRate := values["attendance_rate"]
		attendanceCount := interface{}("")
		if hasStudents {
			if rate, ok := toFloat(attendanceRate); ok {
				attendanceCount = int(float64(studentCount)*rate/100 + 0.5)
			}
		}

		classTime := interface{}(nil)
		if task.ClassTime != nil {
			classTime = task.ClassTime.ToTime().Format("2006-01-02 15:04:05")
		}
		listenContent := ""
		if v, ok := values["listening_content"]; ok && v != nil {
			listenContent = fmt.Sprint(v)
		}
		list = append(list, map[string]interface{}{
			"record_id": r.ID, "teacher_name": teacher.Username, "teacher_user_no": teacher.UserNo,
			"course_name": task.CourseName, "class_time": classTime, "classroom": task.Classroom,
			"college_name": collegeName, "campus_name": campusName,
			"student_count": func() interface{} {
				if hasStudents {
					return studentCount
				}
				return ""
			}(), "attendance_count": attendanceCount,
			"evaluator_name":    evaluatorName,
			"evaluator_role":    model.RoleName(r.EvaluatorRole),
			"listening_content": listenContent, "attendance_rate": attendanceRate,
			"total_score": totalScore, "max_total_score": maxTotal,
			"submit_time": ftimePtr(r.SubmitTime),
		})
	}
	return list, total, nil
}

// ---------- /stats/teacher-evaluation-summary ----------

// SummaryFilters 教师评教汇总筛选
type SummaryFilters struct {
	CollegeIDs      []int
	CampusID        *int
	ResearchRoomIDs []int
	Keyword         string
	EvaluatorRoles  []string
	Start, End      *time.Time
	HasCourses      *bool
	SortBy          string
	SortOrder       string
	Page, PageSize  int
}

// TeacherEvaluationSummary 教师评教汇总（分页 + 排序）
func (s *Stats) TeacherEvaluationSummary(db *gorm.DB, viewer *model.User, f SummaryFilters) ([]map[string]interface{}, int64, error) {
	// 未指定日期时，默认按当前学期汇总
	if f.Start == nil && f.End == nil {
		f.Start, f.End = currentSemesterRange(db)
	}
	scope := collegeScopeFor(viewer)
	semester, _ := currentSemesterOf(db)

	uq := db.Model(&model.User{}).Where("status = 1")
	if viewer.HasRole(model.RoleTeacher) && !IsAdminRole(viewer) && !IsSupervisor(viewer) {
		uq = uq.Where("id = ?", viewer.ID)
	} else if scope != nil {
		if len(scope) == 0 {
			return []map[string]interface{}{}, 0, nil
		}
		uq = uq.Where("college_id IN ?", scope)
	}
	if len(f.CollegeIDs) > 0 {
		uq = uq.Where("college_id IN ?", f.CollegeIDs)
	}
	if f.CampusID != nil {
		uq = uq.Where("college_id IN (SELECT id FROM college WHERE campus_id = ? AND status = 1)", *f.CampusID)
	}
	if len(f.ResearchRoomIDs) > 0 {
		uq = uq.Where("research_room_id IN ?", f.ResearchRoomIDs)
	}
	if f.Keyword != "" {
		uq = uq.Where("username LIKE ?", "%"+f.Keyword+"%")
	}
	if len(f.EvaluatorRoles) > 0 {
		uq = uq.Where("(role IN ? OR id IN (SELECT user_id FROM user_role WHERE role IN ?))",
			f.EvaluatorRoles, f.EvaluatorRoles)
	}

	// 是否有课筛选
	var allCandidates []model.User
	uq.Session(&gorm.Session{}).Preload("College").Find(&allCandidates)
	idsOf := func(us []model.User) []int {
		out := make([]int, 0, len(us))
		for _, u := range us {
			out = append(out, u.ID)
		}
		return out
	}
	if f.HasCourses != nil {
		with := teachersWithCourses(db, idsOf(allCandidates), semester)
		var filtered []model.User
		for _, u := range allCandidates {
			if with[u.ID] == *f.HasCourses {
				filtered = append(filtered, u)
			}
		}
		allCandidates = filtered
	}
	total := int64(len(allCandidates))

	start := (f.Page - 1) * f.PageSize
	if start > len(allCandidates) {
		start = len(allCandidates)
	}
	end := start + f.PageSize
	if end > len(allCandidates) {
		end = len(allCandidates)
	}
	users := allCandidates[start:end]
	userIDs := idsOf(users)
	withCourses := teachersWithCourses(db, userIDs, semester)
	scoreCodes := scoreDimCodeSet(db)

	// 角色映射
	rolesMap := map[int][]string{}
	if len(userIDs) > 0 {
		var urs []model.UserRole
		db.Where("user_id IN ?", userIDs).Find(&urs)
		for _, ur := range urs {
			rolesMap[ur.UserID] = append(rolesMap[ur.UserID], ur.Role)
		}
	}

	applyTime := func(q *gorm.DB) *gorm.DB {
		if f.Start != nil {
			q = q.Where("submit_time >= ?", *f.Start)
		}
		if f.End != nil {
			q = q.Where("submit_time < ?", *f.End)
		}
		return q
	}

	// 批量取评出的记录
	givenBy := map[int][]model.EvaluationRecord{}
	if len(userIDs) > 0 {
		gq := applyTime(db.Where("evaluator_id IN ? AND submit_time IS NOT NULL AND is_deleted = 0", userIDs))
		if len(f.EvaluatorRoles) > 0 {
			gq = gq.Where("evaluator_role IN ?", f.EvaluatorRoles)
		}
		var given []model.EvaluationRecord
		gq.Find(&given)
		for _, r := range given {
			if r.EvaluatorID != nil {
				givenBy[*r.EvaluatorID] = append(givenBy[*r.EvaluatorID], r)
			}
		}
	}

	// 批量取任务与被评记录
	tasksBy := map[int][]model.EvaluationTask{}
	var allTaskIDs []int
	if len(userIDs) > 0 {
		var tasks []model.EvaluationTask
		db.Where("teacher_id IN ? AND is_deleted = 0", userIDs).Find(&tasks)
		for _, t := range tasks {
			tasksBy[t.TeacherID] = append(tasksBy[t.TeacherID], t)
			allTaskIDs = append(allTaskIDs, t.ID)
		}
	}
	receivedBy := map[int][]model.EvaluationRecord{}
	if len(allTaskIDs) > 0 {
		rq := applyTime(db.Where("task_id IN ? AND submit_time IS NOT NULL AND is_deleted = 0", allTaskIDs))
		if len(f.EvaluatorRoles) > 0 {
			rq = rq.Where("evaluator_role IN ?", f.EvaluatorRoles)
		}
		var recs []model.EvaluationRecord
		rq.Find(&recs)
		taskTeacher := map[int]int{}
		for _, t := range tasksBy {
			for _, tt := range t {
				taskTeacher[tt.ID] = tt.TeacherID
			}
		}
		for _, r := range recs {
			if tid, ok := taskTeacher[r.TaskID]; ok {
				receivedBy[tid] = append(receivedBy[tid], r)
			}
		}
	}

	// 任务的 teacher_id 映射（用于 given 统计）
	givenTaskTeacher := map[int]int{}
	{
		gTaskIDs := map[int]bool{}
		for _, recs := range givenBy {
			for _, r := range recs {
				gTaskIDs[r.TaskID] = true
			}
		}
		if len(gTaskIDs) > 0 {
			var tasks []model.EvaluationTask
			db.Select("id, teacher_id").Where("id IN ?", keysOf(gTaskIDs)).Scan(&tasks)
			for _, t := range tasks {
				givenTaskTeacher[t.ID] = t.TeacherID
			}
		}
	}

	result := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		roles := rolesMap[u.ID]
		if len(roles) == 0 && u.Role != "" {
			roles = []string{u.Role}
		}
		roleNames := make([]string, 0, len(roles))
		for _, r := range roles {
			roleNames = append(roleNames, model.RoleName(r))
		}

		given := givenBy[u.ID]
		givenTeachers := map[int]bool{}
		var givenLatest *model.LocalTime
		for _, r := range given {
			if tid, ok := givenTaskTeacher[r.TaskID]; ok {
				givenTeachers[tid] = true
			}
			if r.SubmitTime != nil && (givenLatest == nil || r.SubmitTime.ToTime().After(givenLatest.ToTime())) {
				givenLatest = r.SubmitTime
			}
		}

		uTasks := tasksBy[u.ID]
		received := receivedBy[u.ID]
		receivedEvals := map[int]bool{}
		var receivedLatest *model.LocalTime
		for _, r := range received {
			if r.EvaluatorID != nil {
				receivedEvals[*r.EvaluatorID] = true
			}
			if r.SubmitTime != nil && (receivedLatest == nil || r.SubmitTime.ToTime().After(receivedLatest.ToTime())) {
				receivedLatest = r.SubmitTime
			}
		}

		evaluated, pending := 0, 0
		for _, t := range uTasks {
			switch t.Status {
			case model.TaskStatusEvaluated:
				evaluated++
			case model.TaskStatusPending:
				pending++
			}
		}
		evalRate := 0.0
		if len(uTasks) > 0 {
			evalRate = round2(float64(evaluated) / float64(len(uTasks)) * 100)
		}

		var collegeName interface{}
		if u.College != nil {
			collegeName = u.College.Name
		}
		var givenLatestI, receivedLatestI interface{}
		if givenLatest != nil {
			givenLatestI = givenLatest // LocalTime 序列化为 isoformat（对齐旧端 datetime 输出）
		}
		if receivedLatest != nil {
			receivedLatestI = receivedLatest
		}
		result = append(result, map[string]interface{}{
			"teacher_id": u.ID, "teacher_name": u.Username, "user_no": u.UserNo,
			"college_name": collegeName, "role": u.Role, "role_name": model.RoleName(u.Role),
			"roles": roles, "role_names": roleNames, "has_courses": withCourses[u.ID],
			"given_count": len(given), "given_avg_score": recordsAvgScore(given, scoreCodes),
			"given_teacher_count": len(givenTeachers), "given_latest_time": givenLatestI,
			"received_count": len(received), "received_avg_score": recordsAvgScore(received, scoreCodes),
			"received_evaluator_count": len(receivedEvals), "received_latest_time": receivedLatestI,
			"total_tasks": len(uTasks), "evaluated_tasks": evaluated, "pending_tasks": pending,
			"evaluation_rate": evalRate,
		})
	}

	// 排序（数值优先，空值排后）
	if f.SortBy != "" {
		desc := f.SortOrder != "asc"
		sort.SliceStable(result, func(i, j int) bool {
			vi, vj := result[i][f.SortBy], result[j][f.SortBy]
			fi, oki := numOf(vi)
			fj, okj := numOf(vj)
			switch {
			case !oki && !okj:
				return false
			case !oki:
				return false
			case !okj:
				return true
			}
			if desc {
				return fi > fj
			}
			return fi < fj
		})
	}
	return result, total, nil
}

// numOf 接口数据转数值
func numOf(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case *float64:
		if n == nil {
			return 0, false
		}
		return *n, true
	}
	return 0, false
}
