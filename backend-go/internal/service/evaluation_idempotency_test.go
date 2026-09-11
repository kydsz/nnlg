package service

import (
	"testing"

	"backend-go/internal/model"
)

// TestPersistBatchDuplicateSkipped 同一 (task_id, evaluator_id) 的唯一索引冲突：
// PersistBatch 应跳过冲突条目（幂等成功）而非整批回滚，且不重复计数。
func TestPersistBatchDuplicateSkipped(t *testing.T) {
	db := evaluationTestDB(t)
	// 模拟 MySQL 的 (task_id, evaluator_id) 唯一约束（迁移 012 的 sqlite 等价物）：
	// 仅对 is_deleted=0 生效。生成列 active_key 在 sqlite 中不易建，
	// 直接建 (task_id, evaluator_id, is_deleted) 唯一索引近似验证冲突语义。
	// 注：生产用迁移 012 的生成列方案（软删行不触发唯一），此处验证核心冲突跳过逻辑。
	if err := db.Exec(`CREATE UNIQUE INDEX uk_te ON evaluation_record (task_id, evaluator_id, is_deleted)`).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}

	teacher := model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatalf("建教师失败: %v", err)
	}
	viewer := model.User{UserNo: "T002", Username: "评教人", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("建评教人失败: %v", err)
	}
	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "大学语文",
		Status: model.TaskStatusPending, CreateBy: intPtr(teacher.ID),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	mk := func(msgID string) PendingSubmit {
		return PendingSubmit{MsgID: msgID, Task: task, EvaluatorRole: model.RoleTeacher,
			Params: SubmitParams{TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 90.0}},
			Viewer: &viewer}
	}

	svc := NewEvaluation()
	if err := svc.PersistBatch(db, []PendingSubmit{mk("m1")}); err != nil {
		t.Fatalf("首次落库失败: %v", err)
	}
	// 同 task+user 再次提交（模拟死信重放/并发），应被唯一索引拦截为幂等成功
	if err := svc.PersistBatch(db, []PendingSubmit{mk("m2")}); err != nil {
		t.Fatalf("重复提交应按幂等成功处理: %v", err)
	}
	if err := svc.PersistBatch(db, []PendingSubmit{mk("m3")}); err != nil {
		t.Fatalf("第三次重复提交也应幂等成功: %v", err)
	}

	var cnt int64
	db.Model(&model.EvaluationRecord{}).Where("task_id = ? AND evaluator_id = ?", task.ID, viewer.ID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("重复提交后应只有 1 条记录, 实际 %d", cnt)
	}
	var t2 model.EvaluationTask
	db.First(&t2, task.ID)
	if t2.EvaluationCount != 1 {
		t.Fatalf("任务计数应只加 1, 实际 %d", t2.EvaluationCount)
	}
}

// TestPersistSubmitDuplicateIdempotent 同步提交的唯一冲突：PersistSubmit 返回既有记录且不重复计数。
func TestPersistSubmitDuplicateIdempotent(t *testing.T) {
	db := evaluationTestDB(t)
	if err := db.Exec(`CREATE UNIQUE INDEX uk_te2 ON evaluation_record (task_id, evaluator_id, is_deleted)`).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}
	teacher := model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatalf("建教师失败: %v", err)
	}
	viewer := model.User{UserNo: "T002", Username: "评教人", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("建评教人失败: %v", err)
	}
	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "高等数学",
		Status: model.TaskStatusPending, CreateBy: intPtr(teacher.ID),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	p := SubmitParams{TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 88.0}}

	svc := NewEvaluation()
	rec1, err := svc.PersistSubmit(db, task, model.RoleTeacher, p, &viewer)
	if err != nil {
		t.Fatalf("首次提交失败: %v", err)
	}
	if rec1.ID == 0 {
		t.Fatal("首次提交应返回带 ID 的记录")
	}
	rec2, err := svc.PersistSubmit(db, task, model.RoleTeacher, p, &viewer)
	if err != nil {
		t.Fatalf("重复提交应按幂等成功: %v", err)
	}
	if rec2.ID != rec1.ID {
		t.Fatalf("重复提交应返回既有记录(id=%d), 实际 id=%d", rec1.ID, rec2.ID)
	}
	var t2 model.EvaluationTask
	db.First(&t2, task.ID)
	if t2.EvaluationCount != 1 {
		t.Fatalf("任务计数应只加 1, 实际 %d", t2.EvaluationCount)
	}
}
