package service

import (
	"testing"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// statsScopeTestDB 统计口径测试库：在评教测试库基础上补学期配置表。
func statsScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := evaluationTestDB(t)
	if err := db.AutoMigrate(&model.SemesterConfig{}); err != nil {
		t.Fatalf("建学期配置表失败: %v", err)
	}
	return db
}

// 统计口径：评教记录以记录自身是否有效为准，不因所属任务被软删而排除（issue #28）。
// 任务维度的统计（任务数 / 已评任务 / 待评任务）仍按任务是否软删过滤，两者互不牵连。
func TestStatsScopeIgnoresTaskSoftDelete(t *testing.T) {
	t.Run("同课评教汇总仍计入已软删任务上的记录", func(t *testing.T) {
		db := courseEvalTestDB(t)
		sem1 := time.Date(2025, 9, 15, 10, 0, 0, 0, time.Local)
		t1 := createCourseTask(t, db, 1, "高等数学", sem1)
		if err := db.Model(t1).Update("is_deleted", true).Error; err != nil {
			t.Fatalf("软删任务失败: %v", err)
		}
		createCourseRecord(t, db, t1.ID, 101, model.RoleTeacher)
		createCourseRecord(t, db, t1.ID, 102, model.RoleCollegeSupervisor)

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*t1})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		stat, ok := stats[t1.ID]
		if !ok {
			t.Fatal("已软删任务仍应有同课汇总（评教记录不随任务删除消失）")
		}
		if stat.EvaluatorCount != 2 || !stat.SupervisorEvaluated {
			t.Fatalf("汇总 = %+v, 期望 2 人且督导已评", stat)
		}
	})

	t.Run("taskStatsFor 记录数计入软删任务而任务数不计入", func(t *testing.T) {
		db := evaluationTestDB(t)
		live := model.EvaluationTask{
			TeacherID: 1, TeacherName: "被评教师", CourseName: "高等数学",
			Status: model.TaskStatusEvaluated,
		}
		gone := model.EvaluationTask{
			TeacherID: 1, TeacherName: "被评教师", CourseName: "高等数学",
			Status: model.TaskStatusEvaluated, IsDeleted: true,
		}
		if err := db.Create(&live).Error; err != nil {
			t.Fatalf("建任务失败: %v", err)
		}
		if err := db.Create(&gone).Error; err != nil {
			t.Fatalf("建已软删任务失败: %v", err)
		}
		evaluator := 101
		recs := []model.EvaluationRecord{
			{TaskID: live.ID, EvaluatorID: &evaluator, EvaluatorRole: model.RoleTeacher},
			{TaskID: gone.ID, EvaluatorID: &evaluator, EvaluatorRole: model.RoleTeacher},
		}
		if err := db.Create(&recs).Error; err != nil {
			t.Fatalf("建记录失败: %v", err)
		}

		total, evaluated, pending, evaluations, _ := taskStatsFor(db, taskStatsInput{TeacherIDs: []int{1}})
		if total != 1 || evaluated != 1 || pending != 0 {
			t.Fatalf("任务维度统计 = (总 %d, 已评 %d, 待评 %d), 期望 (1, 1, 0)：软删任务不计入",
				total, evaluated, pending)
		}
		if evaluations != 2 {
			t.Fatalf("评教记录数 = %d, 期望 2：记录不随任务软删消失", evaluations)
		}
	})

	t.Run("教师评教汇总的收到的评教计入软删任务", func(t *testing.T) {
		db := statsScopeTestDB(t)
		college := model.College{Code: "WL", Name: "文理学院", Status: 1}
		if err := db.Create(&college).Error; err != nil {
			t.Fatalf("建学院失败: %v", err)
		}
		teacher := model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1}
		evaluator := model.User{UserNo: "T002", Username: "评教人", Role: model.RoleTeacher, Status: 1}
		if err := db.Create(&teacher).Error; err != nil {
			t.Fatalf("建教师失败: %v", err)
		}
		if err := db.Create(&evaluator).Error; err != nil {
			t.Fatalf("建评教人失败: %v", err)
		}

		start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
		end := start.AddDate(0, 6, 0)
		classTime := model.LocalTime(start.AddDate(0, 0, 10))
		submitTime := model.LocalTime(start.AddDate(0, 0, 11))
		task := model.EvaluationTask{
			TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "高等数学",
			ClassTime: &classTime, Status: model.TaskStatusEvaluated, IsDeleted: true,
			EvaluationCount: 1, CreateBy: &evaluator.ID,
		}
		if err := db.Create(&task).Error; err != nil {
			t.Fatalf("建任务失败: %v", err)
		}
		if err := db.Create(&model.EvaluationRecord{
			TaskID: task.ID, EvaluatorID: &evaluator.ID, EvaluatorName: evaluator.Username,
			EvaluatorRole: model.RoleTeacher, SubmitTime: &submitTime,
			DimensionValues: []byte(`{"score1":88}`),
		}).Error; err != nil {
			t.Fatalf("建记录失败: %v", err)
		}

		viewer := &model.User{Role: model.RoleSystemAdmin}
		list, _, err := NewStats().TeacherEvaluationSummary(db, viewer, SummaryFilters{
			Start: &start, End: &end, Page: 1, PageSize: 10,
		})
		if err != nil {
			t.Fatalf("教师评教汇总失败: %v", err)
		}
		var row map[string]interface{}
		for _, item := range list {
			if item["teacher_id"] == teacher.ID {
				row = item
			}
		}
		if row == nil {
			t.Fatalf("汇总结果中应包含教师 %d, 得到 %d 行", teacher.ID, len(list))
		}
		if got, _ := row["received_count"].(int); got != 1 {
			t.Fatalf("收到的评教 = %v, 期望 1：已软删任务上的记录仍应计入", row["received_count"])
		}
		if got, _ := row["total_tasks"].(int); got != 0 {
			t.Fatalf("任务数 = %v, 期望 0：软删任务不计入任务维度", row["total_tasks"])
		}
	})

	t.Run("学院教师详情的被评数计入软删任务", func(t *testing.T) {
		db := statsScopeTestDB(t)
		college := model.College{Code: "WL", Name: "文理学院", Status: 1}
		if err := db.Create(&college).Error; err != nil {
			t.Fatalf("建学院失败: %v", err)
		}
		teacher := model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1}
		if err := db.Create(&teacher).Error; err != nil {
			t.Fatalf("建教师失败: %v", err)
		}
		task := model.EvaluationTask{
			TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "高等数学",
			Status: model.TaskStatusEvaluated, IsDeleted: true, EvaluationCount: 1,
		}
		if err := db.Create(&task).Error; err != nil {
			t.Fatalf("建已软删任务失败: %v", err)
		}
		evaluator := 101
		if err := db.Create(&model.EvaluationRecord{
			TaskID: task.ID, EvaluatorID: &evaluator, EvaluatorRole: model.RoleTeacher,
		}).Error; err != nil {
			t.Fatalf("建记录失败: %v", err)
		}

		viewer := &model.User{Role: model.RoleSystemAdmin}
		data, err := NewStats().CollegeTeacherDetails(db, viewer, college.ID)
		if err != nil {
			t.Fatalf("学院教师详情失败: %v", err)
		}
		details, _ := data["teacher_details"].([]map[string]interface{})
		if len(details) != 1 {
			t.Fatalf("教师明细应为 1 行, 得到 %d", len(details))
		}
		if got, _ := details[0]["be_evaluated_count"].(int); got != 1 {
			t.Fatalf("被评数 = %v, 期望 1：已软删任务上的记录仍应计入", details[0]["be_evaluated_count"])
		}
	})
}
