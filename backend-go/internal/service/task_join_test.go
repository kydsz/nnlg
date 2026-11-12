package service

import (
	"errors"
	"testing"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

func seedJoinUser(t *testing.T, db *gorm.DB, collegeID int, no string) *model.User {
	t.Helper()
	u := model.User{
		UserNo: no, Username: "评教人" + no, Role: model.RoleTeacher,
		Status: 1, CollegeID: &collegeID,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("建评教人 %s 失败: %v", no, err)
	}
	if err := db.Create(&model.UserRole{UserID: u.ID, Role: model.RoleTeacher}).Error; err != nil {
		t.Fatalf("建评教人角色 %s 失败: %v", no, err)
	}
	return &u
}

func seedJoinTask(t *testing.T, db *gorm.DB, teacherID int) *model.EvaluationTask {
	t.Helper()
	task := model.EvaluationTask{
		TeacherID: teacherID, TeacherName: "任课教师", CourseName: "高等数学",
		Status: model.TaskStatusPending,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	return &task
}

// 默认上限 4：前 4 人加入成功，第 5 人满员拒绝；重复加入幂等。
func TestJoinTaskLimitAndIdempotency(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 1
	teacherID := seedTaskTeacher(t, db, collegeID)
	task := seedJoinTask(t, db, teacherID)
	svc := NewTask()

	users := make([]*model.User, 0, 5)
	for i := 0; i < 5; i++ {
		users = append(users, seedJoinUser(t, db, collegeID, "J00"+string(rune('1'+i))))
	}
	for i := 0; i < 4; i++ {
		res, err := svc.Join(db, users[i], task.ID)
		if err != nil {
			t.Fatalf("第 %d 人加入失败: %v", i+1, err)
		}
		if res.ParticipantCount != int64(i+1) || res.MaxParticipants != 4 {
			t.Fatalf("第 %d 人加入后人数=%d 上限=%d", i+1, res.ParticipantCount, res.MaxParticipants)
		}
	}

	if _, err := svc.Join(db, users[4], task.ID); !errors.Is(err, ErrTaskFull) {
		t.Fatalf("第 5 人应满员拒绝, 实际 err=%v", err)
	}
	res, err := svc.Join(db, users[0], task.ID)
	if err != nil {
		t.Fatalf("重复加入应幂等成功: %v", err)
	}
	if !res.AlreadyJoined || res.ParticipantCount != 4 {
		t.Fatalf("重复加入结果 already=%v count=%d", res.AlreadyJoined, res.ParticipantCount)
	}
}

// 系统配置改为 2 后，第 3 人无法加入。
func TestJoinTaskUsesConfiguredLimit(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 1
	teacherID := seedTaskTeacher(t, db, collegeID)
	task := seedJoinTask(t, db, teacherID)
	svc := NewTask()
	if err := SetTaskMaxParticipants(db, 1, 2); err != nil {
		t.Fatalf("设置上限失败: %v", err)
	}
	u1 := seedJoinUser(t, db, collegeID, "L001")
	u2 := seedJoinUser(t, db, collegeID, "L002")
	u3 := seedJoinUser(t, db, collegeID, "L003")
	if _, err := svc.Join(db, u1, task.ID); err != nil {
		t.Fatalf("第 1 人加入失败: %v", err)
	}
	if _, err := svc.Join(db, u2, task.ID); err != nil {
		t.Fatalf("第 2 人加入失败: %v", err)
	}
	if _, err := svc.Join(db, u3, task.ID); !errors.Is(err, ErrTaskFull) {
		t.Fatalf("第 3 人应满员拒绝, 实际 err=%v", err)
	}
}

// 持 task:join_force（系统管理员恒可）可超员加入，Count > Max。
func TestJoinTaskForceOverLimit(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 1
	teacherID := seedTaskTeacher(t, db, collegeID)
	task := seedJoinTask(t, db, teacherID)
	svc := NewTask()
	for i := 0; i < 4; i++ {
		u := seedJoinUser(t, db, collegeID, "F00"+string(rune('1'+i)))
		if _, err := svc.Join(db, u, task.ID); err != nil {
			t.Fatalf("第 %d 人加入失败: %v", i+1, err)
		}
	}
	admin := model.User{UserNo: "ADM", Username: "系统管理员", Role: model.RoleSystemAdmin, Status: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}
	res, err := svc.Join(db, &admin, task.ID)
	if err != nil {
		t.Fatalf("管理员超员加入失败: %v", err)
	}
	if res.ParticipantCount != 5 || res.MaxParticipants != 4 || !res.OverLimit {
		t.Fatalf("超员结果 count=%d max=%d over=%v", res.ParticipantCount, res.MaxParticipants, res.OverLimit)
	}
}

// 状态口径：全部参与人都提交后才已评；作废一人回到待评，恢复后回到已评。
func TestTaskStatusRequiresAllParticipants(t *testing.T) {
	db := evaluationTestDB(t)
	collegeID := 5
	teacher := model.User{UserNo: "T900", Username: "被评教师", Role: model.RoleTeacher,
		Status: 1, CollegeID: &collegeID}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatalf("建被评教师失败: %v", err)
	}
	u1 := seedJoinUser(t, db, collegeID, "E001")
	u2 := seedJoinUser(t, db, collegeID, "E002")
	admin := model.User{UserNo: "A900", Username: "系统管理员", Role: model.RoleSystemAdmin, Status: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}
	task := seedJoinTask(t, db, teacher.ID)
	taskSvc := NewTask()
	evalSvc := NewEvaluation()
	if _, err := taskSvc.Join(db, u1, task.ID); err != nil {
		t.Fatalf("u1 加入失败: %v", err)
	}
	if _, err := taskSvc.Join(db, u2, task.ID); err != nil {
		t.Fatalf("u2 加入失败: %v", err)
	}

	rec1, err := evalSvc.Submit(db, u1, SubmitParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 90.0},
	})
	if err != nil {
		t.Fatalf("u1 提交失败: %v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.Status != model.TaskStatusPending {
		t.Fatalf("仅 1/2 提交后状态=%d, 期望待评", tk.Status)
	}
	if _, err := evalSvc.Submit(db, u2, SubmitParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 80.0},
	}); err != nil {
		t.Fatalf("u2 提交失败: %v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.Status != model.TaskStatusEvaluated {
		t.Fatalf("2/2 提交后状态=%d, 期望已评", tk.Status)
	}

	if _, err := evalSvc.Delete(db, &admin, rec1.ID); err != nil {
		t.Fatalf("作废 u1 记录失败: %v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.Status != model.TaskStatusPending {
		t.Fatalf("作废 1/2 后状态=%d, 期望待评", tk.Status)
	}
	if _, err := evalSvc.Restore(db, &admin, rec1.ID); err != nil {
		t.Fatalf("恢复 u1 记录失败: %v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.Status != model.TaskStatusEvaluated {
		t.Fatalf("恢复后 2/2 状态=%d, 期望已评", tk.Status)
	}
}

// 已评任务新增参与人后回到待评。
func TestJoinAfterAllEvaluatedReopensTask(t *testing.T) {
	db := evaluationTestDB(t)
	collegeID := 5
	teacher := model.User{UserNo: "T901", Username: "被评教师", Role: model.RoleTeacher,
		Status: 1, CollegeID: &collegeID}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatalf("建被评教师失败: %v", err)
	}
	u1 := seedJoinUser(t, db, collegeID, "R001")
	u2 := seedJoinUser(t, db, collegeID, "R002")
	task := seedJoinTask(t, db, teacher.ID)
	taskSvc := NewTask()
	evalSvc := NewEvaluation()
	if _, err := taskSvc.Join(db, u1, task.ID); err != nil {
		t.Fatalf("u1 加入失败: %v", err)
	}
	if _, err := evalSvc.Submit(db, u1, SubmitParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 90.0},
	}); err != nil {
		t.Fatalf("u1 提交失败: %v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.Status != model.TaskStatusEvaluated {
		t.Fatalf("唯一参与人提交后状态=%d, 期望已评", tk.Status)
	}
	if _, err := taskSvc.Join(db, u2, task.ID); err != nil {
		t.Fatalf("u2 加入失败: %v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.Status != model.TaskStatusPending {
		t.Fatalf("新增参与人后状态=%d, 期望待评", tk.Status)
	}
}

// JoinOrCreate 复用同教师+课程+上课时间的任务，不新增第二条。
func TestJoinOrCreateReusesExistingTask(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 1
	teacherID := seedTaskTeacher(t, db, collegeID)
	u1 := seedJoinUser(t, db, collegeID, "C001")
	u2 := seedJoinUser(t, db, collegeID, "C002")
	svc := NewTask()
	classTime := time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local)
	p := CreateTaskParams{TeacherID: teacherID, CourseName: "线性代数", ClassTime: model.LocalTimePtr(classTime)}

	res1, err := svc.JoinOrCreate(db, u1, p)
	if err != nil {
		t.Fatalf("u1 加入或创建失败: %v", err)
	}
	if !res1.Created || res1.ParticipantCount != 1 {
		t.Fatalf("u1 结果 created=%v count=%d", res1.Created, res1.ParticipantCount)
	}
	res2, err := svc.JoinOrCreate(db, u2, p)
	if err != nil {
		t.Fatalf("u2 加入失败: %v", err)
	}
	if res2.Created || res2.ParticipantCount != 2 {
		t.Fatalf("u2 结果 created=%v count=%d", res2.Created, res2.ParticipantCount)
	}
	if n := taskCount(t, db); n != 1 {
		t.Fatalf("应只有 1 条任务, 实际 %d", n)
	}
}

// 加入后未创建/未被评/未评过的人也能看到任务；管理端代建不产生参与人。
func TestJoinedTaskVisibilityAndAdminCreateNoParticipant(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 1
	teacherID := seedTaskTeacher(t, db, collegeID)
	creator := seedJoinUser(t, db, collegeID, "V001")
	evaluator := seedJoinUser(t, db, collegeID, "V002")
	svc := NewTask()

	// 管理端 Create 不自动加入
	task, err := svc.Create(db, creator, CreateTaskParams{TeacherID: teacherID, CourseName: "大学物理"})
	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}
	if n, _ := TaskParticipantCount(db, task.ID); n != 0 {
		t.Fatalf("管理端代建不应产生参与人, 实际 %d", n)
	}
	// 加入前不可见
	if CanViewTask(db, evaluator, task) {
		t.Fatal("加入前不应可见")
	}
	if _, err := svc.Join(db, evaluator, task.ID); err != nil {
		t.Fatalf("加入失败: %v", err)
	}
	if !CanViewTask(db, evaluator, task) {
		t.Fatal("加入后应可见")
	}
	list, total, err := svc.List(db, evaluator, TaskFilters{Page: 1, PageSize: 10, JoinedBy: &evaluator.ID})
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].ID != task.ID {
		t.Fatalf("我加入的列表 total=%d len=%d", total, len(list))
	}
	// JoinOrCreate 拒绝加入自己的任务
	if _, err := svc.JoinOrCreate(db, &model.User{Model: model.Model{ID: teacherID}}, CreateTaskParams{
		TeacherID: teacherID, CourseName: "大学物理",
	}); err == nil {
		t.Fatal("加入自己的任务应被拒绝")
	}
}
