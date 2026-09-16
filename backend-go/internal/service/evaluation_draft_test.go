package service

import (
	"testing"
	"time"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// draftNotNullTestDB 构造与线上一致的表结构：evaluation_draft 由迁移 009 建表，
// create_time/update_time 为 NOT NULL DEFAULT CURRENT_TIMESTAMP(3)。
// 注意：AutoMigrate 依据 *LocalTime 指针字段建出的是可空列，恰好掩盖本类缺陷，
// 故此处用与 009 等价的建表语句（NOT NULL）建表。
func draftNotNullTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.EvaluationTask{}); err != nil {
		t.Fatalf("建任务表失败: %v", err)
	}
	if err := db.Exec(`CREATE TABLE evaluation_draft (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id BIGINT NULL,
		evaluator_id BIGINT NULL,
		evaluator_name VARCHAR(64) NULL,
		dimension_values JSON NULL,
		is_anonymous TINYINT(1) NOT NULL DEFAULT 0,
		create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (task_id, evaluator_id)
	)`).Error; err != nil {
		t.Fatalf("建草稿表失败: %v", err)
	}
	return db
}

// TestDraftSavePersistsTimestamps 暂存必须在库里落上 create_time/update_time，且覆盖暂存要刷新 update_time。
// 背景：线上曾整条暂存不可用 —— 新建草稿未赋值 Model.CreateTime/UpdateTime，而 Model 的时间字段是
// *LocalTime（指针、字段名非 GORM 约定的 CreatedAt），nil 时 GORM 会把 NULL 显式写进列，
// 绕过列的 server_default，MySQL 严格模式直接报
// Error 1048 (23000): Column 'create_time' cannot be null。
func TestDraftSavePersistsTimestamps(t *testing.T) {
	db := draftNotNullTestDB(t)
	svc := NewEvaluation()
	viewer := model.User{UserNo: "D002", Username: "督导兼教师", Role: model.RoleTeacher}
	viewer.ID = 7

	task := model.EvaluationTask{
		TeacherID: 1, TeacherName: "被评教师", CourseName: "高等数学",
		Status: model.TaskStatusPending, CreateBy: intPtr(1),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}

	// 读回落库行（时间列 NULL 时对应字段为 nil）
	readDraft := func(id int) model.EvaluationDraft {
		var row model.EvaluationDraft
		if err := db.First(&row, id).Error; err != nil {
			t.Fatalf("读回草稿失败: %v", err)
		}
		return row
	}

	// 首次暂存（INSERT）
	draft, err := svc.SaveDraft(db, &viewer, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 70.0},
	})
	if err != nil {
		t.Fatalf("首次暂存失败: %v", err)
	}
	if row := readDraft(draft.ID); row.CreateTime == nil || row.UpdateTime == nil {
		t.Fatalf("草稿时间列不得为 NULL: create_time=%v update_time=%v", row.CreateTime, row.UpdateTime)
	}

	// 覆盖暂存（UPDATE）：预置一个历史 update_time，保存后必须被刷新，且 create_time 不得被置空
	history := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	if err := db.Exec("UPDATE evaluation_draft SET update_time = ? WHERE id = ?", history, draft.ID).Error; err != nil {
		t.Fatalf("预置历史 update_time 失败: %v", err)
	}
	if _, err := svc.SaveDraft(db, &viewer, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 85.0}, IsAnonymous: true,
	}); err != nil {
		t.Fatalf("覆盖暂存失败: %v", err)
	}
	row := readDraft(draft.ID)
	if row.CreateTime == nil {
		t.Fatal("覆盖暂存后 create_time 为 NULL")
	}
	if row.UpdateTime == nil || row.UpdateTime.ToTime().Year() == history.Year() {
		t.Fatalf("覆盖暂存未刷新 update_time，仍为预置值 %v", row.UpdateTime)
	}
}

// buildDraftBase 建一对用户（被评教师+评教人）与待评任务，黑板复用正式提交用例的督导场景
func buildDraftBase(t *testing.T, db *gorm.DB) *model.EvaluationTask {
	users := []model.User{
		{UserNo: "D001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
		{UserNo: "D002", Username: "督导兼教师", Role: model.RoleTeacher, CollegeID: intPtr(6), Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	// 督导兼教师：user_role 关联督导角色，负责文理学院(5)，与正式提交用例一致
	users[1].UserRoles = []model.UserRole{
		{UserID: users[1].ID, Role: model.RoleTeacher},
		{UserID: users[1].ID, Role: model.RoleSupervisor},
	}
	users[1].UserColleges = []model.UserCollege{{UserID: users[1].ID, CollegeID: 5}}
	if err := db.Create(&users[1].UserRoles).Error; err != nil {
		t.Fatalf("建用户角色失败: %v", err)
	}
	if err := db.Create(&users[1].UserColleges).Error; err != nil {
		t.Fatalf("建督导学院失败: %v", err)
	}
	task := model.EvaluationTask{
		TeacherID: users[0].ID, TeacherName: "被评教师", CourseName: "高等数学",
		Status: model.TaskStatusPending, CreateBy: intPtr(users[0].ID),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	return &task
}

func TestDraftSaveUpsert(t *testing.T) {
	db := evaluationTestDB(t)
	task := buildDraftBase(t, db)
	svc := NewEvaluation()

	var evaluator model.User
	db.Where("user_no = ?", "D002").First(&evaluator)

	// 首次创建
	draft, err := svc.SaveDraft(db, &evaluator, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 70.0},
	})
	if err != nil {
		t.Fatalf("首次暂存失败: %v", err)
	}
	if draft.ID == 0 {
		t.Fatal("草稿应成功创建并获得 id")
	}

	// 再次保存为覆盖更新，行数仍为 1、内容变更
	got, err := svc.SaveDraft(db, &evaluator, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 85.0}, IsAnonymous: true,
	})
	if err != nil {
		t.Fatalf("再次暂存失败: %v", err)
	}
	if got.ID != draft.ID {
		t.Fatalf("覆盖更新应复用同一条草稿 id=%d, 实际 id=%d", draft.ID, got.ID)
	}
	var count int64
	db.Model(&model.EvaluationDraft{}).Where("task_id = ? AND evaluator_id = ?", task.ID, evaluator.ID).Count(&count)
	if count != 1 {
		t.Fatalf("同一任务同一评教人草稿行数 = %d, 期望 1", count)
	}
	var persisted model.EvaluationDraft
	db.First(&persisted, draft.ID)
	if !persisted.IsAnonymous {
		t.Fatal("覆盖保存后 is_anonymous 应为 true")
	}
	if string(persisted.DimensionValues) != `{"score1":85}` {
		t.Fatalf("覆盖保存后 dimension_values = %s, 期望 {\"score1\":85}", string(persisted.DimensionValues))
	}
}

func TestDraftGetMyDraft(t *testing.T) {
	db := evaluationTestDB(t)
	task := buildDraftBase(t, db)
	svc := NewEvaluation()

	var evaluator, other model.User
	db.Where("user_no = ?", "D002").First(&evaluator)
	db.Where("user_no = ?", "D001").First(&other)

	// 无草稿返回 nil
	d, err := svc.GetMyDraft(db, &evaluator, task.ID)
	if err != nil {
		t.Fatalf("查询无草稿报错: %v", err)
	}
	if d != nil {
		t.Fatal("无草稿时应返回 nil")
	}

	// 他人保存草稿后，本人仍不可见
	if _, err := svc.SaveDraft(db, &other, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 60.0},
	}); err != nil {
		t.Fatalf("他人暂存失败: %v", err)
	}
	d, err = svc.GetMyDraft(db, &evaluator, task.ID)
	if err != nil {
		t.Fatalf("查询本人草稿报错: %v", err)
	}
	if d != nil {
		t.Fatal("他人的草稿对本人不可见，应返回 nil")
	}

	// 本人保存后可查回
	if _, err := svc.SaveDraft(db, &evaluator, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 90.0},
	}); err != nil {
		t.Fatalf("本人暂存失败: %v", err)
	}
	d, err = svc.GetMyDraft(db, &evaluator, task.ID)
	if err != nil {
		t.Fatalf("查询本人草稿报错: %v", err)
	}
	if d == nil || d.TaskID != task.ID || d.EvaluatorID != evaluator.ID {
		t.Fatalf("应返回本人草稿, 实际 = %+v", d)
	}
}

func TestDraftSaveInvalidTask(t *testing.T) {
	db := evaluationTestDB(t)
	task := buildDraftBase(t, db)
	svc := NewEvaluation()

	var evaluator model.User
	db.Where("user_no = ?", "D002").First(&evaluator)

	// 不存在的任务
	_, err := svc.SaveDraft(db, &evaluator, SaveDraftParams{
		TaskID: 999999, DimensionValues: map[string]interface{}{"score1": 70.0},
	})
	if err == nil || err.Error() != "任务不存在" {
		t.Fatalf("不存在任务应报 '任务不存在', 实际 = %v", err)
	}

	// 已取消的任务
	db.Model(&model.EvaluationTask{}).Where("id = ?", task.ID).Update("status", model.TaskStatusCancelled)
	_, err = svc.SaveDraft(db, &evaluator, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 70.0},
	})
	if err == nil || err.Error() != "任务已取消，不能暂存" {
		t.Fatalf("已取消任务应报 '任务已取消，不能暂存', 实际 = %v", err)
	}
}

// TestDraftClearedOnSubmit：草稿在正式提交落库成功后被删除
func TestDraftClearedOnSubmit(t *testing.T) {
	db := evaluationTestDB(t)
	task := buildDraftBase(t, db)
	svc := NewEvaluation()

	var evaluator model.User
	db.Where("user_no = ?", "D002").First(&evaluator)
	// 恢复多角色督导关联（与建库一致），供正式提交的角色判定使用
	evaluator.UserRoles = []model.UserRole{
		{UserID: evaluator.ID, Role: model.RoleTeacher},
		{UserID: evaluator.ID, Role: model.RoleSupervisor},
	}
	evaluator.UserColleges = []model.UserCollege{{UserID: evaluator.ID, CollegeID: 5}}

	// 先暂存一条草稿（内容不完整也不校验）
	draft, err := svc.SaveDraft(db, &evaluator, SaveDraftParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 66.0},
	})
	if err != nil {
		t.Fatalf("暂存失败: %v", err)
	}

	// 正式提交（督导可评同学院教师，score1 维度满分 100）
	if _, err := svc.Submit(db, &evaluator, SubmitParams{
		TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 95.0},
	}); err != nil {
		t.Fatalf("正式提交失败: %v", err)
	}

	// 提交后草稿应被删除
	var count int64
	db.Model(&model.EvaluationDraft{}).Where("id = ?", draft.ID).Count(&count)
	if count != 0 {
		t.Fatalf("正式提交后草稿应被删除, 剩余 %d 条", count)
	}
	var recCount int64
	db.Model(&model.EvaluationRecord{}).Where("task_id = ? AND evaluator_id = ?", task.ID, evaluator.ID).Count(&recCount)
	if recCount != 1 {
		t.Fatalf("正式提交应落库 1 条记录, 实际 %d", recCount)
	}
}