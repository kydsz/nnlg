package service

import (
	"errors"
	"fmt"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrTaskFull 任务参与人数已满
var ErrTaskFull = errors.New("评教任务参与人数已满")

// ParticipantJoinResult 加入任务的结果
type ParticipantJoinResult struct {
	Task             *model.EvaluationTask
	ParticipantCount int64
	MaxParticipants  int
	AlreadyJoined    bool
	OverLimit        bool
	Created          bool
}

// TaskParticipantCount 任务当前参与人数
func TaskParticipantCount(db *gorm.DB, taskID int) (int64, error) {
	var n int64
	err := db.Model(&model.EvaluationTaskParticipant{}).Where("task_id = ?", taskID).Count(&n).Error
	return n, err
}

// TaskParticipantCounts 批量读取任务参与人数（列表页一次聚合查询）
func TaskParticipantCounts(db *gorm.DB, taskIDs []int) map[int]int64 {
	out := map[int]int64{}
	if len(taskIDs) == 0 {
		return out
	}
	var rows []struct {
		TaskID int
		Cnt    int64
	}
	db.Model(&model.EvaluationTaskParticipant{}).
		Select("task_id, COUNT(*) AS cnt").
		Where("task_id IN ?", taskIDs).
		Group("task_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.TaskID] = r.Cnt
	}
	return out
}

// JoinedTaskIDs 当前用户已加入的任务 ID 集合
func JoinedTaskIDs(db *gorm.DB, userID int, taskIDs []int) map[int]bool {
	out := map[int]bool{}
	if userID == 0 || len(taskIDs) == 0 {
		return out
	}
	var ids []int
	db.Model(&model.EvaluationTaskParticipant{}).
		Where("task_id IN ? AND user_id = ?", taskIDs, userID).
		Pluck("task_id", &ids)
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// IsTaskParticipant 判断用户是否已加入任务
func IsTaskParticipant(db *gorm.DB, taskID, userID int) (bool, error) {
	var n int64
	if err := db.Model(&model.EvaluationTaskParticipant{}).
		Where("task_id = ? AND user_id = ?", taskID, userID).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// EnsureTaskParticipant 加入任务（幂等），独立事务版本。
// force=true 时允许超过上限（task:join_force）。
func EnsureTaskParticipant(db *gorm.DB, taskID, userID int, force bool) (*ParticipantJoinResult, error) {
	max := TaskMaxParticipants(db)
	var res *ParticipantJoinResult
	err := db.Transaction(func(tx *gorm.DB) error {
		r, err := ensureParticipant(tx, taskID, userID, force, max)
		res = r
		return err
	})
	return res, err
}

// ensureParticipant 在给定事务内完成「锁任务行 → 判重 → 判满 → 插入参与人」。
// 并发下由任务行锁（MySQL）串行化，防止两人同时抢最后一个名额。
func ensureParticipant(tx *gorm.DB, taskID, userID int, force bool, max int) (*ParticipantJoinResult, error) {
	q := tx.Where("id = ? AND is_deleted = 0", taskID)
	if tx.Dialector.Name() == "mysql" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var task model.EvaluationTask
	if err := q.First(&task).Error; err != nil {
		return nil, errors.New("评教任务不存在")
	}
	if task.Status == model.TaskStatusCancelled {
		return nil, errors.New("任务已取消，不能加入")
	}

	var count int64
	if err := tx.Model(&model.EvaluationTaskParticipant{}).
		Where("task_id = ?", taskID).Count(&count).Error; err != nil {
		return nil, err
	}
	var existing int64
	if err := tx.Model(&model.EvaluationTaskParticipant{}).
		Where("task_id = ? AND user_id = ?", taskID, userID).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return &ParticipantJoinResult{
			Task: &task, ParticipantCount: count, MaxParticipants: max,
			AlreadyJoined: true, OverLimit: count > int64(max),
		}, nil
	}
	if count >= int64(max) && !force {
		return nil, fmt.Errorf("%w：该任务已有 %d 人加入，无法加入（上限 %d 人）", ErrTaskFull, count, max)
	}

	now := model.LocalTimePtr(time.Now())
	p := model.EvaluationTaskParticipant{TaskID: taskID, UserID: userID, JoinedAt: now}
	if err := tx.Create(&p).Error; err != nil {
		if isDupKeyErr(err) {
			// 并发下另一请求已加入：按幂等处理并重新计数
			var c int64
			if cerr := tx.Model(&model.EvaluationTaskParticipant{}).
				Where("task_id = ?", taskID).Count(&c).Error; cerr != nil {
				return nil, cerr
			}
			return &ParticipantJoinResult{
				Task: &task, ParticipantCount: c, MaxParticipants: max,
				AlreadyJoined: true, OverLimit: c > int64(max),
			}, nil
		}
		return nil, err
	}
	count++
	return &ParticipantJoinResult{
		Task: &task, ParticipantCount: count, MaxParticipants: max,
		OverLimit: count > int64(max),
	}, nil
}

// syncTaskStatusByParticipants 按「全部参与人都已提交有效评教」重算任务状态。
// 历史任务没有参与人数据时保持旧口径：有有效记录即已评。
func syncTaskStatusByParticipants(tx *gorm.DB, taskID int) error {
	var task model.EvaluationTask
	if err := tx.Where("id = ? AND is_deleted = 0", taskID).First(&task).Error; err != nil {
		return nil // 任务不存在/已删除：不处理
	}
	if task.Status == model.TaskStatusCancelled {
		return nil
	}
	var participants, active int64
	if err := tx.Model(&model.EvaluationTaskParticipant{}).
		Where("task_id = ?", taskID).Count(&participants).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.EvaluationRecord{}).
		Where("task_id = ? AND is_deleted = 0", taskID).Count(&active).Error; err != nil {
		return err
	}
	newStatus := model.TaskStatusPending
	if participants > 0 {
		if active >= participants {
			newStatus = model.TaskStatusEvaluated
		}
	} else if active > 0 {
		newStatus = model.TaskStatusEvaluated
	}
	if task.Status != newStatus {
		return tx.Model(&model.EvaluationTask{}).Where("id = ?", taskID).Update("status", newStatus).Error
	}
	return nil
}

// Join 加入指定任务（校验可见范围/评教范围后占名额）。
func (s *Task) Join(db *gorm.DB, caller *model.User, taskID int) (*ParticipantJoinResult, error) {
	t, err := s.Get(db, taskID)
	if err != nil {
		return nil, err
	}
	if t.Status == model.TaskStatusCancelled {
		return nil, errors.New("任务已取消，不能加入")
	}
	if _, err := checkEvaluationScope(db, caller, t); err != nil {
		return nil, err
	}
	res, err := EnsureTaskParticipant(db, taskID, caller.ID, CanForceJoinTask(db, caller))
	if err != nil {
		return nil, err
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return syncTaskStatusByParticipants(tx, taskID)
	}); err != nil {
		return nil, err
	}
	if latest, err := s.Get(db, taskID); err == nil {
		res.Task = latest
	}
	return res, nil
}

// JoinOrCreate 课表「加入待评任务」：已有同教师+课程+上课时间的有效任务则加入，
// 否则创建任务并加入（创建人自动成为第 1 个参与人）。
// 管理端 Create/BatchCreate 不调用本方法，因此后台代建不占名额。
func (s *Task) JoinOrCreate(db *gorm.DB, caller *model.User, p CreateTaskParams) (*ParticipantJoinResult, error) {
	if p.TeacherID == caller.ID {
		return nil, errors.New("不能加入自己的评教任务")
	}
	existing, err := findActiveTaskByDedupe(db, p)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		res, err := s.Join(db, caller, existing.ID)
		if err != nil {
			return nil, err
		}
		res.Created = false
		return res, nil
	}

	t, err := s.Create(db, caller, p)
	if err != nil {
		// 并发下另一个请求先创建：改走加入
		if errors.Is(err, ErrDuplicateTask) {
			existing, ferr := findActiveTaskByDedupe(db, p)
			if ferr != nil {
				return nil, ferr
			}
			if existing != nil {
				res, jerr := s.Join(db, caller, existing.ID)
				if jerr != nil {
					return nil, jerr
				}
				res.Created = false
				return res, nil
			}
		}
		return nil, err
	}
	res, err := s.Join(db, caller, t.ID)
	if err != nil {
		return nil, err
	}
	res.Created = true
	return res, nil
}

// findActiveTaskByDedupe 按「教师+课程+上课时间」查找未删除未取消任务。
func findActiveTaskByDedupe(db *gorm.DB, p CreateTaskParams) (*model.EvaluationTask, error) {
	q := db.Where("teacher_id = ? AND course_name = ? AND is_deleted = 0 AND status <> ?",
		p.TeacherID, p.CourseName, model.TaskStatusCancelled)
	if p.ClassTime == nil {
		q = q.Where("class_time IS NULL")
	} else {
		q = q.Where("class_time = ?", *p.ClassTime)
	}
	var t model.EvaluationTask
	if err := q.Order("id ASC").First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}
