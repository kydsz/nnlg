package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"backend-go/internal/cache"
	"backend-go/internal/model"

	"gorm.io/gorm"
)

// Task 评教任务业务
type Task struct{}

func NewTask() *Task { return &Task{} }

// TaskFilters 列表筛选
type TaskFilters struct {
	Keyword           string
	Status            *int16
	TeacherID         *int
	CollegeIDs        []int // 显式查询参数（逗号分隔）
	HasSupervisorEval *bool
	CreateBy          *int
	CreateByNot       *int
	Start, End        *time.Time // 按上课时间（class_time）筛选学期区间
	OrderBy, OrderDir string     // 排序字段白名单（id/class_time）与方向（asc/desc）
	Page, PageSize    int
}

// applyCollegeFilter 按学院集合过滤任务的被评教师
func applyCollegeFilter(q *gorm.DB, ids []int) *gorm.DB {
	if ids == nil {
		return q
	}
	if len(ids) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", ids)
}

// relatedToMeCond 「与我相关」的评教任务：我创建的、我被评的、我评过的。
//
// 未获 task:view_all 权限者（当前典型为普通教师）的可见范围收敛条件，
// 三处占位符均为当前用户 ID。评教任务不是「广播」数据，而是每位评教人自己的
// 待评课表：看不见他人任务，也就看不见督导是否把某位教师的课排进了待评计划。
// 注：任务表在本查询中不带别名，子查询按表名 evaluation_task 关联。
const relatedToMeCond = "(create_by = ? OR teacher_id = ? OR EXISTS (" +
	"SELECT 1 FROM evaluation_record er " +
	"WHERE er.task_id = evaluation_task.id AND er.evaluator_id = ? AND er.is_deleted = 0))"

func (s *Task) buildQuery(db *gorm.DB, f TaskFilters, caller *model.User) (*gorm.DB, error) {
	q := db.Model(&model.EvaluationTask{}).Where("is_deleted = 0")

	if f.Keyword != "" {
		// 同时匹配课程名与被评教师姓名（teacher_name 为建任务时冗余的快照字段）
		kw := "%" + f.Keyword + "%"
		q = q.Where("course_name LIKE ? OR teacher_name LIKE ?", kw, kw)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.Start != nil {
		q = q.Where("class_time >= ?", *f.Start)
	}
	if f.End != nil {
		// end 为结束日期（含当天），上界取次日零点
		q = q.Where("class_time < ?", f.End.AddDate(0, 0, 1))
	}
	if f.TeacherID != nil {
		q = q.Where("teacher_id = ?", *f.TeacherID)
	}
	if f.HasSupervisorEval != nil {
		q = q.Where("has_supervisor_eval = ?", *f.HasSupervisorEval)
	}
	if f.CreateBy != nil {
		q = q.Where("create_by = ?", *f.CreateBy)
	}
	if f.CreateByNot != nil {
		q = q.Where("create_by <> ? OR create_by IS NULL", *f.CreateByNot)
	}

	// 数据范围过滤
	scope := AccessibleCollegeIDs(caller)
	ids := f.CollegeIDs
	if scope != nil {
		if ids == nil {
			ids = scope
		} else {
			set := map[int]bool{}
			for _, id := range scope {
				set[id] = true
			}
			var merged []int
			for _, id := range ids {
				if set[id] {
					merged = append(merged, id)
				}
			}
			ids = merged
		}
	}
	q = applyCollegeFilter(q, ids)

	// 与我相关的任务收敛：未获 task:view_all 权限者只能看到自己创建的、自己被评的、自己评过的任务。
	// 该条件在服务端强制生效，前端筛选参数（create_by / create_by_not）无法绕过——去掉参数也拿不到无关任务。
	if !CanViewAllTasks(db, caller) {
		q = q.Where(relatedToMeCond, caller.ID, caller.ID, caller.ID)
	}
	return q, nil
}

// List 分页查询任务
func (s *Task) List(db *gorm.DB, caller *model.User, f TaskFilters) ([]model.EvaluationTask, int64, error) {
	q, err := s.buildQuery(db, f, caller)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []model.EvaluationTask
	// 排序白名单：仅允许 id / class_time（class_time 为空的排最后），其余回落 id DESC
	switch f.OrderBy {
	case "id":
		q = q.Order("id " + sortDir(f.OrderDir))
	case "class_time":
		q = q.Order("class_time IS NULL ASC, class_time " + sortDir(f.OrderDir) + ", id DESC")
	default:
		q = q.Order("id DESC")
	}
	err = q.Session(&gorm.Session{}).
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Find(&tasks).Error
	return tasks, total, err
}

// Get 加载单个未删除任务
func (s *Task) Get(db *gorm.DB, id int) (*model.EvaluationTask, error) {
	var t model.EvaluationTask
	if err := db.Where("id = ? AND is_deleted = 0", id).First(&t).Error; err != nil {
		return nil, errors.New("评教任务不存在")
	}
	return &t, nil
}

// LoadTeacherUser 加载被评教师（含学院）
func LoadTeacherUser(db *gorm.DB, id int) (*model.User, error) {
	u, err := cache.LoadUser(context.Background(), db, id)
	if err != nil {
		return nil, errors.New("被评教师不存在")
	}
	return u, nil
}

// TeacherCollegeMap 批量读取「被评教师 ID → 所属学院」。
//
// 学院口径全系统唯一：被评教师的**主学院**（user.college_id），与评教任务的可见范围
// （AccessibleCollegeIDs/applyCollegeFilter）和统计报表一致。评教记录列表、评教任务
// 列表与任务导出走这里，避免这几条链路各写一份 JOIN 造成口径漂移。
// （统计明细与单任务详情仍有各自的预加载写法，未并入。）
//
// 教师不存在、未绑定学院、或学院行缺失时该键缺省，调用方按「无学院」处理。
// 注意：model.User 含指针字段，必须用 Find 而非 Scan（Scan + Select 组合会报 unsupported data type）。
func TeacherCollegeMap(db *gorm.DB, teacherIDs []int) map[int]model.College {
	out := map[int]model.College{}
	if len(teacherIDs) == 0 {
		return out
	}
	var users []model.User
	db.Select("id, college_id").Preload("College").Where("id IN ?", teacherIDs).Find(&users)
	for _, u := range users {
		if u.College != nil {
			out[u.ID] = *u.College
		}
	}
	return out
}

// TeacherCollegeDisplay 取某被评教师的学院展示信息：ID 指针与学院名，两者在教师无学院时均为 nil。
// 返回 interface{} 是刻意的——JSON 序列化需要 null 而不是空串/0，前端据 null 渲染 "-"。
func TeacherCollegeDisplay(colleges map[int]model.College, teacherID int) (*int, interface{}) {
	college, ok := colleges[teacherID]
	if !ok {
		return nil, nil
	}
	id := college.ID
	return &id, college.Name
}

// checkTaskTargetScope 校验 caller 对目标教师所在学院的操作权
func checkTaskTargetScope(caller *model.User, teacher *model.User) error {
	switch {
	case IsAdminRole(caller):
		if !CollegeInScope(caller, teacher.CollegeID) {
			return errors.New("您只能操作本学院教师的任务")
		}
	case IsSupervisor(caller):
		if !CollegeInScope(caller, teacher.CollegeID) {
			return errors.New("您只能操作自己负责学院的教师任务")
		}
	default: // 教师
		if teacher.CollegeID == nil || caller.CollegeID == nil || *teacher.CollegeID != *caller.CollegeID {
			return errors.New("您只能为同学院教师创建任务")
		}
		if teacher.Role != model.RoleTeacher && !teacher.HasRole(model.RoleTeacher) {
			return errors.New("只能为教师角色创建评教任务")
		}
	}
	return nil
}

// CreateTaskParams 创建参数
type CreateTaskParams struct {
	TeacherID   int              `json:"teacher_id"`
	TeacherName string           `json:"teacher_name"`
	CourseName  string           `json:"course_name"`
	ClassTime   *model.LocalTime `json:"class_time"`
	Classroom   *string          `json:"classroom"`
	StartTime   *model.LocalTime `json:"start_time"`
	EndTime     *model.LocalTime `json:"end_time"`
	// 课表信息快照（加入待评时固化：上课时间/教室/班级/应到人数/周次）
	ScheduleSnapshot *ScheduleSnapshot `json:"schedule"`
}

// ScheduleSnapshot 加入待评任务时固化的课表信息（与提交页展示一致）
type ScheduleSnapshot struct {
	ClassTimeText *string `json:"class_time_text"`
	Classroom     *string `json:"classroom"`
	ClassInfo     *string `json:"class_info"`
	StudentCount  *int    `json:"student_count"`
	WeekPattern   *string `json:"week_pattern"`
}

// ErrDuplicateTask 任务重复：同教师 + 同课程 + 同上课时间，且未删除未取消。
// 预检查与唯一索引兜底（迁移 013）共用同一语义，便于批量场景按重复跳过。
var ErrDuplicateTask = errors.New("该教师已存在相同课程的任务")

// taskDedupeKey 批次内去重键（口径与预检查一致：教师+课程+上课时间）
func taskDedupeKey(p CreateTaskParams) string {
	ct := ""
	if p.ClassTime != nil {
		ct = p.ClassTime.ToTime().Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("%d|%s|%s", p.TeacherID, p.CourseName, ct)
}

// buildTask 创建前校验并构造任务实体（不落库）。
// 批量创建据此先整体校验、再统一在事务内落库。
func (s *Task) buildTask(db *gorm.DB, caller *model.User, p CreateTaskParams) (*model.EvaluationTask, error) {
	if p.TeacherID == 0 || p.CourseName == "" {
		return nil, errors.New("teacher_id 与 course_name 为必填")
	}
	teacher, err := LoadTeacherUser(db, p.TeacherID)
	if err != nil {
		return nil, err
	}
	if teacher.Status != 1 {
		return nil, errors.New("被评教师已停用")
	}
	if err := checkTaskTargetScope(caller, teacher); err != nil {
		return nil, err
	}

	// 去重预检查：同教师+同课程+同上课时间，未删除且未取消。
	// class_time 参与比较：不同时间（同一天不同节次、不同周）的课程不算重复。
	// 预检查只为「友好提示 + 批量跳过」，并发安全由唯一索引 uk_task_dedupe_active 兜底。
	var dup model.EvaluationTask
	q := db.Where("teacher_id = ? AND course_name = ? AND is_deleted = 0 AND status <> ?",
		p.TeacherID, p.CourseName, model.TaskStatusCancelled)
	if p.ClassTime != nil {
		q = q.Where("class_time = ?", p.ClassTime)
	} else {
		q = q.Where("class_time IS NULL")
	}
	if err := q.First(&dup).Error; err == nil {
		return nil, fmt.Errorf("该教师已存在相同课程的任务（任务ID=%d）", dup.ID)
	}

	name := p.TeacherName
	if name == "" {
		name = teacher.Username
	}
	t := model.EvaluationTask{
		TeacherID: p.TeacherID, TeacherName: name, CourseName: p.CourseName,
		ClassTime: p.ClassTime, Classroom: p.Classroom,
		Status: model.TaskStatusPending, StartTime: p.StartTime, EndTime: p.EndTime,
		CreateBy: IPtr(caller.ID),
	}
	// 加入待评时固化的课表信息快照（JSON 落库）
	if p.ScheduleSnapshot != nil {
		raw, err := json.Marshal(p.ScheduleSnapshot)
		if err == nil {
			t.ScheduleSnapshot = raw
		}
	}
	return &t, nil
}

// Create 创建任务（含权限与去重校验）
func (s *Task) Create(db *gorm.DB, caller *model.User, p CreateTaskParams) (*model.EvaluationTask, error) {
	t, err := s.buildTask(db, caller, p)
	if err != nil {
		return nil, err
	}
	if err := db.Create(t).Error; err != nil {
		// 并发下两个请求可同时通过预检查，由唯一索引兜底；
		// 命中时返回与预检查同类的重复语义，而非暴露原始 1062 报错。
		if isDupKeyErr(err) {
			return nil, ErrDuplicateTask
		}
		return nil, err
	}
	return t, nil
}

// BatchCreateResult 批量创建结果
type BatchCreateResult struct {
	Created      []model.EvaluationTask   `json:"-"`
	CreatedTasks []map[string]interface{} `json:"created_tasks"`
	Skipped      []map[string]interface{} `json:"skipped"`
}

// BatchCreate 批量创建。
//
// 分两阶段，兼顾「按项跳过」的既有语义与「失败不留半批数据」：
//  1. 校验阶段（事务外）：逐项做权限/参数/重复预检查，不合格项计入 skipped 并说明原因，
//     不阻断本批其它项；批内重复（同一请求里出现两条相同任务）在此直接跳过。
//  2. 落库阶段（单事务）：校验通过的项一次性提交，任一项落库失败（DB 故障等）则整体回滚，
//     避免「接口报错但库里已有部分任务」的脏状态；
//     *并发* 才暴露的唯一索引冲突仍按重复跳过（另一请求已创建），不拖垮整批。
func (s *Task) BatchCreate(db *gorm.DB, caller *model.User, items []CreateTaskParams) (*BatchCreateResult, error) {
	res := &BatchCreateResult{CreatedTasks: []map[string]interface{}{}, Skipped: []map[string]interface{}{}}
	skip := func(p CreateTaskParams, reason string) {
		res.Skipped = append(res.Skipped, map[string]interface{}{
			"teacher_id": p.TeacherID, "course_name": p.CourseName, "reason": reason,
		})
	}

	pending := make([]*model.EvaluationTask, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, p := range items {
		if key := taskDedupeKey(p); seen[key] {
			skip(p, "本次提交中已存在相同课程的任务")
			continue
		}
		t, err := s.buildTask(db, caller, p)
		if err != nil {
			skip(p, err.Error())
			continue
		}
		seen[taskDedupeKey(p)] = true
		pending = append(pending, t)
	}
	if len(pending) == 0 {
		return res, nil
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, t := range pending {
			if err := tx.Create(t).Error; err != nil {
				if isDupKeyErr(err) {
					skip(CreateTaskParams{TeacherID: t.TeacherID, CourseName: t.CourseName}, ErrDuplicateTask.Error())
					continue
				}
				return err // 触发回滚：本批已插入的任务全部撤销
			}
			res.Created = append(res.Created, *t)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// UpdateTaskParams 编辑参数
type UpdateTaskParams struct {
	CourseName *string          `json:"course_name"`
	ClassTime  *model.LocalTime `json:"class_time"`
	Classroom  *string          `json:"classroom"`
	StartTime  *model.LocalTime `json:"start_time"`
	EndTime    *model.LocalTime `json:"end_time"`
	Status     *int16           `json:"status"`
	// 课表信息快照（编辑时随教室/时间一起可更新）
	ScheduleSnapshot *ScheduleSnapshot `json:"schedule"`
}

// Update 编辑任务（状态机见需求文档 13.2）
func (s *Task) Update(db *gorm.DB, caller *model.User, id int, p UpdateTaskParams) (*model.EvaluationTask, error) {
	t, err := s.Get(db, id)
	if err != nil {
		return nil, err
	}
	teacher, err := LoadTeacherUser(db, t.TeacherID)
	if err != nil {
		return nil, err
	}

	// 编辑权限
	switch {
	case IsAdminRole(caller):
		if !CollegeInScope(caller, teacher.CollegeID) {
			return nil, errors.New("您只能操作本学院教师的任务")
		}
	case IsSupervisor(caller):
		if !CollegeInScope(caller, teacher.CollegeID) {
			return nil, errors.New("您只能操作自己负责学院的教师任务")
		}
	default:
		if t.CreateBy == nil || *t.CreateBy != caller.ID {
			return nil, errors.New("您只能编辑自己创建的任务")
		}
		if t.EvaluationCount > 0 {
			return nil, errors.New("已有评教提交的任务不可编辑")
		}
	}

	updates := map[string]interface{}{}
	hasOther := false
	if p.CourseName != nil {
		updates["course_name"] = *p.CourseName
		hasOther = true
	}
	if p.ClassTime != nil {
		updates["class_time"] = *p.ClassTime
		hasOther = true
	}
	if p.Classroom != nil {
		updates["classroom"] = *p.Classroom
		hasOther = true
	}
	if p.StartTime != nil {
		updates["start_time"] = *p.StartTime
		hasOther = true
	}
	if p.EndTime != nil {
		updates["end_time"] = *p.EndTime
		hasOther = true
	}
	if p.ScheduleSnapshot != nil {
		if raw, err := json.Marshal(p.ScheduleSnapshot); err == nil {
			updates["schedule_snapshot"] = raw
			hasOther = true
		}
	}

	// 状态流转
	if p.Status != nil && *p.Status != t.Status {
		newStatus := *p.Status
		switch {
		case t.Status == model.TaskStatusPending && newStatus == model.TaskStatusCancelled:
			updates["status"] = newStatus // 取消
		case t.Status == model.TaskStatusCancelled && newStatus == model.TaskStatusPending:
			updates["status"] = newStatus // 恢复
		case t.Status == model.TaskStatusEvaluated && newStatus == model.TaskStatusCancelled:
			return nil, errors.New("已评任务不可取消")
		case t.Status == model.TaskStatusEvaluated && newStatus == model.TaskStatusPending:
			return nil, errors.New("已评任务不可退回待评")
		case newStatus == model.TaskStatusEvaluated:
			return nil, errors.New("已评状态由评教提交自动变更")
		default:
			return nil, errors.New("非法的状态变更")
		}
	}

	if t.Status == model.TaskStatusCancelled {
		// 已取消任务仅允许"恢复"，不允许改其它字段
		if _, restoring := updates["status"]; !restoring || hasOther {
			return nil, errors.New("已取消任务仅可恢复，不可编辑其它字段")
		}
	}

	if len(updates) > 0 {
		if err := db.Model(t).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return s.Get(db, id)
}

// Delete 软删除（任务及名下评教记录一并软删，数据保留、可恢复）
// 有 task:delete 权限可删任意；仅有 task:delete_own 权限仅能删除自己创建的任务
func (s *Task) Delete(db *gorm.DB, caller *model.User, id int) error {
	t, err := s.Get(db, id)
	if err != nil {
		return err
	}
	if CanDeleteTask(db, caller) {
		// 放行，允许删除任意任务
	} else if CanDeleteOwnTask(db, caller) {
		if t.CreateBy == nil || *t.CreateBy != caller.ID {
			return errors.New("只能删除自己创建的评教任务")
		}
	} else {
		return errors.New("无权删除评教任务")
	}
	// 同步软删该任务名下评教记录，避免任务删除后记录仍残留于记录列表/统计
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.EvaluationRecord{}).
			Where("task_id = ? AND is_deleted = 0", id).
			Update("is_deleted", true).Error; err != nil {
			return err
		}
		return tx.Model(t).Update("is_deleted", true).Error
	})
}
