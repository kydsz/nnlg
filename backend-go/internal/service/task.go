package service

import (
	"errors"
	"fmt"

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

func (s *Task) buildQuery(db *gorm.DB, f TaskFilters, caller *model.User) (*gorm.DB, error) {
	q := db.Model(&model.EvaluationTask{}).Where("is_deleted = 0")

	if f.Keyword != "" {
		q = q.Where("course_name LIKE ?", "%"+f.Keyword+"%")
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
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
	err = q.Session(&gorm.Session{}).
		Order("id DESC").
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
	var u model.User
	if err := db.Preload("College").First(&u, id).Error; err != nil {
		return nil, errors.New("被评教师不存在")
	}
	return &u, nil
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
}

// Create 创建任务（含权限与去重校验）
func (s *Task) Create(db *gorm.DB, caller *model.User, p CreateTaskParams) (*model.EvaluationTask, error) {
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

	// 去重：同教师+同课程+同上课时间，未删除且未取消。
	// class_time 参与比较：不同时间（同一天不同节次、不同周）的课程不算重复。
	var dup model.EvaluationTask
	q := db.Where("teacher_id = ? AND course_name = ? AND is_deleted = 0 AND status <> ?",
		p.TeacherID, p.CourseName, model.TaskStatusCancelled)
	if p.ClassTime != nil {
		q = q.Where("class_time = ?", p.ClassTime)
	} else {
		q = q.Where("class_time IS NULL")
	}
	err = q.First(&dup).Error
	if err == nil {
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
	if err := db.Create(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// BatchCreateResult 批量创建结果
type BatchCreateResult struct {
	Created      []model.EvaluationTask   `json:"-"`
	CreatedTasks []map[string]interface{} `json:"created_tasks"`
	Skipped      []map[string]interface{} `json:"skipped"`
}

// BatchCreate 批量创建
func (s *Task) BatchCreate(db *gorm.DB, caller *model.User, items []CreateTaskParams) (*BatchCreateResult, error) {
	res := &BatchCreateResult{CreatedTasks: []map[string]interface{}{}, Skipped: []map[string]interface{}{}}
	for _, p := range items {
		t, err := s.Create(db, caller, p)
		if err != nil {
			res.Skipped = append(res.Skipped, map[string]interface{}{
				"teacher_id": p.TeacherID, "course_name": p.CourseName, "reason": err.Error(),
			})
			continue
		}
		res.Created = append(res.Created, *t)
	}
	return res, nil
}

// CancelTask 取消任务（对齐旧端 cancel：404 -> 权限 -> 已评 400 -> 已取消 400 -> 删除记录 -> 状态置取消）
func (s *Task) CancelTask(db *gorm.DB, caller *model.User, id int) (t *model.EvaluationTask, cancelled int, err error) {
	t, err = s.Get(db, id)
	if err != nil {
		return nil, 0, err
	}

	// 只有创建者或管理员可以取消
	isCreator := t.CreateBy != nil && *t.CreateBy == caller.ID
	isAdmin := caller.HasRole(model.RoleSystemAdmin) || caller.HasRole(model.RoleSchoolAdmin)
	if !isAdmin {
		teacher, terr := LoadTeacherUser(db, t.TeacherID)
		if terr == nil && IsSupervisor(caller) && !CollegeInScope(caller, teacher.CollegeID) {
			return nil, 0, errors.New("您只能操作自己负责学院的教师任务")
		}
		if !isCreator && !IsSupervisor(caller) && !IsAdminRole(caller) {
			return nil, 0, errors.New("无权取消此任务")
		}
	}

	// 已评的任务不能取消
	if t.Status == model.TaskStatusEvaluated {
		return nil, 0, errors.New("已评的任务不能取消")
	}
	// 已取消的任务不能重复取消
	if t.Status == model.TaskStatusCancelled {
		return nil, 0, errors.New("任务已取消")
	}

	// 统计并删除关联评教记录
	var records []model.EvaluationRecord
	db.Where("task_id = ?", id).Find(&records)
	cancelled = len(records)
	return t, cancelled, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&model.EvaluationRecord{}).Error; err != nil {
			return err
		}
		return tx.Model(t).Updates(map[string]interface{}{
			"status": model.TaskStatusCancelled, "evaluation_count": 0, "has_supervisor_eval": false,
		}).Error
	})
}

// UpdateTaskParams 编辑参数
type UpdateTaskParams struct {
	CourseName *string          `json:"course_name"`
	ClassTime  *model.LocalTime `json:"class_time"`
	Classroom  *string          `json:"classroom"`
	StartTime  *model.LocalTime `json:"start_time"`
	EndTime    *model.LocalTime `json:"end_time"`
	Status     *int16           `json:"status"`
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

// Delete 软删除
func (s *Task) Delete(db *gorm.DB, id int) error {
	t, err := s.Get(db, id)
	if err != nil {
		return err
	}
	return db.Model(t).Update("is_deleted", true).Error
}
