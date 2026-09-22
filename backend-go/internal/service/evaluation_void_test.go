package service

import (
	"testing"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// setupVoidEnv 建学院/被评教师/督导/同行教师与一条"已评"任务（2 条有效记录）。
func setupVoidEnv(t *testing.T) (*gorm.DB, *model.User, *model.EvaluationTask, []model.EvaluationRecord) {
	t.Helper()
	db := evaluationTestDB(t)
	college := model.College{Code: "WL", Name: "文理学院", Status: 1}
	if err := db.Create(&college).Error; err != nil {
		t.Fatalf("建学院失败: %v", err)
	}
	users := []model.User{
		{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1},
		{UserNo: "S001", Username: "督导", Role: model.RoleSupervisor, CollegeID: &college.ID, Status: 1},
		{UserNo: "T002", Username: "同行教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	admin := model.User{UserNo: "A001", Username: "系统管理员", Role: model.RoleSystemAdmin, Status: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}
	task := model.EvaluationTask{
		TeacherID: users[0].ID, TeacherName: users[0].Username, CourseName: "高等数学",
		Status: model.TaskStatusEvaluated, EvaluationCount: 2, HasSupervisorEval: true,
		CreateBy: &users[1].ID,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	recs := []model.EvaluationRecord{
		{TaskID: task.ID, EvaluatorID: &users[1].ID, EvaluatorName: users[1].Username,
			EvaluatorRole: model.RoleSupervisor, DimensionValues: []byte(`{"score1":90}`)},
		{TaskID: task.ID, EvaluatorID: &users[2].ID, EvaluatorName: users[2].Username,
			EvaluatorRole: model.RoleTeacher, DimensionValues: []byte(`{"score1":80}`)},
	}
	if err := db.Create(&recs).Error; err != nil {
		t.Fatalf("建记录失败: %v", err)
	}
	return db, &admin, &task, recs
}

func taskAfter(t *testing.T, db *gorm.DB, id int) model.EvaluationTask {
	t.Helper()
	var tk model.EvaluationTask
	if err := db.Where("id = ?", id).First(&tk).Error; err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	return tk
}

// 作废评教记录后，任务的评教数 / 督导已评 / 状态应与「当前有效评教记录」一致（issue #26）：
// 否则任务列表的「已评 N 人」、任务导出「评教次数」会虚高，与被作废记录的实际口径冲突。
func TestVoidEvaluationSyncsTaskAggregates(t *testing.T) {
	svc := NewEvaluation()

	t.Run("逐条作废：计数递减、最后一条后督导已评与状态回退", func(t *testing.T) {
		db, admin, task, recs := setupVoidEnv(t)

		// 作废同行教师记录：计数 2 → 1，督导已评保持，任务仍为已评
		if _, err := svc.Delete(db, admin, recs[1].ID); err != nil {
			t.Fatalf("作废教师记录失败: %v", err)
		}
		tk := taskAfter(t, db, task.ID)
		if tk.EvaluationCount != 1 {
			t.Fatalf("作废后评教数 = %d, 期望 1", tk.EvaluationCount)
		}
		if !tk.HasSupervisorEval {
			t.Fatalf("仍有有效督导记录，督导已评不应回退")
		}
		if tk.Status != model.TaskStatusEvaluated {
			t.Fatalf("仍有有效记录，任务状态 = %d, 期望已评", tk.Status)
		}

		// 作废最后一条（督导）记录：计数归零，督导已评回退，任务回到待评
		if _, err := svc.Delete(db, admin, recs[0].ID); err != nil {
			t.Fatalf("作废督导记录失败: %v", err)
		}
		tk = taskAfter(t, db, task.ID)
		if tk.EvaluationCount != 0 {
			t.Fatalf("最后一条作废后评教数 = %d, 期望 0", tk.EvaluationCount)
		}
		if tk.HasSupervisorEval {
			t.Fatalf("无有效督导记录后督导已评应回退为否")
		}
		if tk.Status != model.TaskStatusPending {
			t.Fatalf("无有效记录后任务状态 = %d, 期望待评(%d)", tk.Status, model.TaskStatusPending)
		}
	})

	t.Run("仍有其他督导记录时督导已评保持", func(t *testing.T) {
		db, admin, task, recs := setupVoidEnv(t)
		other := model.User{UserNo: "S002", Username: "督导二", Role: model.RoleSupervisor, Status: 1}
		if err := db.Create(&other).Error; err != nil {
			t.Fatalf("建督导二失败: %v", err)
		}
		if err := db.Create(&model.EvaluationRecord{
			TaskID: task.ID, EvaluatorID: &other.ID, EvaluatorName: other.Username,
			EvaluatorRole: model.RoleCollegeSupervisor, DimensionValues: []byte(`{"score1":70}`),
		}).Error; err != nil {
			t.Fatalf("建记录失败: %v", err)
		}
		if err := db.Model(task).Update("evaluation_count", 3).Error; err != nil {
			t.Fatalf("更新计数失败: %v", err)
		}

		if _, err := svc.Delete(db, admin, recs[0].ID); err != nil {
			t.Fatalf("作废督导记录失败: %v", err)
		}
		tk := taskAfter(t, db, task.ID)
		if tk.EvaluationCount != 2 {
			t.Fatalf("作废后评教数 = %d, 期望 2", tk.EvaluationCount)
		}
		if !tk.HasSupervisorEval {
			t.Fatalf("仍有其他督导记录，督导已评应保持")
		}
		if tk.Status != model.TaskStatusEvaluated {
			t.Fatalf("任务状态 = %d, 期望已评", tk.Status)
		}
	})

	t.Run("重复作废不重复扣减", func(t *testing.T) {
		db, admin, task, recs := setupVoidEnv(t)
		if _, err := svc.Delete(db, admin, recs[1].ID); err != nil {
			t.Fatalf("首次作废失败: %v", err)
		}
		if _, err := svc.Delete(db, admin, recs[1].ID); err == nil {
			t.Fatalf("重复作废应报记录不存在")
		}
		tk := taskAfter(t, db, task.ID)
		if tk.EvaluationCount != 1 {
			t.Fatalf("重复作废后评教数 = %d, 期望 1", tk.EvaluationCount)
		}
	})
}
