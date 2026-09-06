package service

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// CourseEvalStat 同课评教汇总（同学期 + 同教师 + 同课程名，跨评教任务聚合）
type CourseEvalStat struct {
	SupervisorEvaluated bool // 存在评教人角色为督导的评教记录（含当前用户）
	EvaluatorCount      int  // 按评教人去重的人数
}

// courseEvalGroup 一组"同课"的匹配条件（学期起止由任务 class_time 推算，见 docs/adr/0001）
type courseEvalGroup struct {
	teacherID int
	course    string
	start     time.Time
	end       time.Time
}

// normalizeCourse 课程名归一化：小写、去首尾空格，显式对齐任务创建去重在 MySQL 默认
// 排序规则下的等值语义（ADR-0001）；SQL 条件与内存分组键必须共用本函数，否则同名异写
// 课程会被 SQL 命中、却落不进任何组（重音折叠不复制；TrimSpace 含全部空白而 MySQL
// TRIM 仅空格，课程名场景无实际差异）
func normalizeCourse(course string) string {
	return strings.ToLower(strings.TrimSpace(course))
}

// courseGroupKey 同课组键，参数序与键布局一致：学期|教师|课程
func courseGroupKey(semester string, teacherID int, course string) string {
	return semester + "|" + strconv.Itoa(teacherID) + "|" + normalizeCourse(course)
}

// orderedSemesterConfigs 学期配置按开学日期升序（同一区间重叠时取最早开学者，保证归属确定）
func orderedSemesterConfigs(db *gorm.DB) ([]model.SemesterConfig, error) {
	var cfgs []model.SemesterConfig
	if err := db.Find(&cfgs).Error; err != nil {
		return nil, err
	}
	sort.Slice(cfgs, func(i, j int) bool {
		return cfgs[i].StartDate.ToTime().Before(cfgs[j].StartDate.ToTime())
	})
	return cfgs, nil
}

// semesterConfigOfTask 任务学期归属：class_time 落入某配置的 [start_date, start_date+周数)
// 即为该学期（同一区间重叠时按升序取最早开学者，保证归属确定）；class_time 为空或不在
// 任何配置区间内返回 false
func semesterConfigOfTask(cfgs []model.SemesterConfig, classTime *model.LocalTime) (model.SemesterConfig, bool) {
	if classTime == nil {
		return model.SemesterConfig{}, false
	}
	ct := classTime.ToTime()
	for _, c := range cfgs {
		start := c.StartDate.ToTime()
		if !ct.Before(start) && ct.Before(start.AddDate(0, 0, semesterDurationDays(c.Weeks))) {
			return c, true
		}
	}
	return model.SemesterConfig{}, false
}

// CourseEvalStatsForTasks 批量计算一组任务的同课评教汇总，返回 taskID -> 汇总。
// 有学期归属的任务必在结果中（零评教为零值，即确定的「未评/0 人」）；
// 无学期归属（class_time 为空或不在学期区间内）的任务不在结果中（无法统计）。
func CourseEvalStatsForTasks(db *gorm.DB, tasks []model.EvaluationTask) (map[int]CourseEvalStat, error) {
	res := make(map[int]CourseEvalStat, len(tasks))
	cfgs, err := orderedSemesterConfigs(db)
	if err != nil {
		return res, err
	}
	if len(cfgs) == 0 {
		return res, nil
	}

	// 本页任务按 (教师, 课程, 学期) 分组去重
	groups := map[string]*courseEvalGroup{}
	taskGroup := map[int]string{}
	for i := range tasks {
		t := &tasks[i]
		cfg, ok := semesterConfigOfTask(cfgs, t.ClassTime)
		if !ok {
			continue
		}
		key := courseGroupKey(cfg.Semester, t.TeacherID, t.CourseName)
		if _, ok := groups[key]; !ok {
			start := cfg.StartDate.ToTime()
			groups[key] = &courseEvalGroup{
				teacherID: t.TeacherID, course: t.CourseName,
				start: start, end: start.AddDate(0, 0, semesterDurationDays(cfg.Weeks)),
			}
		}
		taskGroup[t.ID] = key
	}
	if len(groups) == 0 {
		return res, nil
	}

	// 一次查询拉取所有组名下的评教记录（仅取统计所需列），内存分组汇总：
	// 人数按 evaluator_id 去重；督导已评按记录的 evaluator_role 判定
	type recRow struct {
		TeacherID     int              `gorm:"column:teacher_id"`
		CourseName    string           `gorm:"column:course_name"`
		ClassTime     *model.LocalTime `gorm:"column:class_time"`
		EvaluatorID   int              `gorm:"column:evaluator_id"`
		EvaluatorRole string           `gorm:"column:evaluator_role"`
	}
	q := db.Table("evaluation_record r").
		Joins("JOIN evaluation_task t ON t.id = r.task_id AND t.is_deleted = 0").
		Where("r.is_deleted = 0 AND r.evaluator_id IS NOT NULL")
	var cond *gorm.DB
	for _, g := range groups {
		c := db.Where("t.teacher_id = ? AND LOWER(TRIM(t.course_name)) = ? AND t.class_time >= ? AND t.class_time < ?",
			g.teacherID, normalizeCourse(g.course), g.start, g.end)
		if cond == nil {
			cond = c
		} else {
			cond = cond.Or(c)
		}
	}
	var rows []recRow
	if err := q.Where(cond).
		Select("t.teacher_id, t.course_name, t.class_time, r.evaluator_id, r.evaluator_role").
		Scan(&rows).Error; err != nil {
		return res, err
	}

	type agg struct {
		evaluatorIDs map[int]bool
		supervisor   bool
	}
	aggs := map[string]*agg{}
	for _, r := range rows {
		cfg, ok := semesterConfigOfTask(cfgs, r.ClassTime)
		if !ok {
			continue
		}
		key := courseGroupKey(cfg.Semester, r.TeacherID, r.CourseName)
		a := aggs[key]
		if a == nil {
			a = &agg{evaluatorIDs: map[int]bool{}}
			aggs[key] = a
		}
		a.evaluatorIDs[r.EvaluatorID] = true
		if model.IsSupervisorRole(r.EvaluatorRole) {
			a.supervisor = true
		}
	}
	for id, key := range taskGroup {
		// 有学期归属的任务必发汇总：零评教为零值（确定的「未评/0 人」），
		// 缺席仅保留给无学期归属的任务（三态中的「无法统计」）
		stat := CourseEvalStat{}
		if a := aggs[key]; a != nil {
			stat = CourseEvalStat{SupervisorEvaluated: a.supervisor, EvaluatorCount: len(a.evaluatorIDs)}
		}
		res[id] = stat
	}
	return res, nil
}

// semesterConfigOf 按学期代码取配置（找不到返回零值）
func semesterConfigOf(cfgs []model.SemesterConfig, semester string) model.SemesterConfig {
	for _, c := range cfgs {
		if c.Semester == semester {
			return c
		}
	}
	return model.SemesterConfig{}
}
