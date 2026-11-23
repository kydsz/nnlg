package service

import (
	"testing"
	"time"

	"backend-go/internal/model"
)

// 统计口径：任务按其授课学院归集；教师按主学院 ∪ 授课学院出现在学院名单中。
func TestStatsUseTaskTeachingCollege(t *testing.T) {
	db := evaluationTestDB(t)

	xd := model.College{Code: "XD", Name: "信息与电气工程学院", Status: 1}
	tm := model.College{Code: "TM", Name: "土木与工程学院", Status: 1}
	for _, c := range []*model.College{&xd, &tm} {
		if err := db.Create(c).Error; err != nil {
			t.Fatal(err)
		}
	}
	teacher := model.User{UserNo: "T930", Username: "陈守学", Role: model.RoleTeacher, CollegeID: &tm.ID, Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.TeacherTeachingCollege{
		UserID: teacher.ID, CollegeID: xd.ID, Source: "manual",
	}).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	taskXD := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "模拟电子技术",
		TeacherCollegeID: &tm.ID, CourseCollegeID: &xd.ID,
		ClassTime: model.LocalTimePtr(now), Status: model.TaskStatusEvaluated, EvaluationCount: 1,
	}
	taskTM := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "建筑工程物联网",
		TeacherCollegeID: &tm.ID, CourseCollegeID: &tm.ID,
		ClassTime: model.LocalTimePtr(now), Status: model.TaskStatusEvaluated, EvaluationCount: 1,
	}
	for _, tk := range []*model.EvaluationTask{&taskXD, &taskTM} {
		if err := db.Create(tk).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, taskID := range []int{taskXD.ID, taskTM.ID} {
		rec := model.EvaluationRecord{
			TaskID: taskID, EvaluatorID: intPtr(9999), EvaluatorName: "督导",
			EvaluatorRole: model.RoleSupervisor, DimensionValues: []byte(`{"score1":90}`),
			SubmitTime: model.LocalTimePtr(now),
		}
		if err := db.Create(&rec).Error; err != nil {
			t.Fatal(err)
		}
	}

	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	total, evaluated, _, evaluations, _ := taskStatsFor(db, taskStatsInput{
		CollegeIDs: []int{xd.ID}, ClassStart: &start, ClassEnd: &end,
	})
	if total != 1 || evaluated != 1 || evaluations != 1 {
		t.Fatalf("信电任务统计应为 1/1/1, 得到 %d/%d/%d", total, evaluated, evaluations)
	}

	stats := &Stats{}
	sysAdmin := &model.User{Model: model.Model{ID: 9301}, Role: model.RoleSystemAdmin}
	items, n, err := stats.TeacherStats(db, sysAdmin, TeacherStatsFilters{
		CollegeIDs: []int{xd.ID}, Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(items) != 1 {
		t.Fatalf("信电教师统计应命中 1 人, 得到 total=%d rows=%d", n, len(items))
	}
	if items[0].TotalTasks != 1 || items[0].TotalEvaluations != 1 {
		t.Fatalf("教师统计应只算信电任务, 得到 tasks=%d evals=%d", items[0].TotalTasks, items[0].TotalEvaluations)
	}
	if len(items[0].TeachingCollegeNames) != 1 || items[0].TeachingCollegeNames[0] != xd.Name {
		t.Fatalf("应返回授课学院名称, 得到 %+v", items[0].TeachingCollegeNames)
	}

	list, _, _, err := stats.CollegeStatsList(db, sysAdmin, CollegeStatsFilters{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int]map[string]interface{}{}
	for _, row := range list {
		if id, ok := row["college_id"].(int); ok {
			byID[id] = row
		}
	}
	if got := byID[xd.ID]["total_tasks"]; got != int64(1) {
		t.Fatalf("信电学院任务数应为 1, 得到 %v", got)
	}
	if got := byID[tm.ID]["total_tasks"]; got != int64(1) {
		t.Fatalf("土木学院任务数应为 1, 得到 %v", got)
	}

	details, err := stats.CollegeTeacherDetails(db, sysAdmin, xd.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := details["teacher_details"].([]map[string]interface{})
	if len(rows) != 1 {
		t.Fatalf("信电学院教师明细应为 1 行, 得到 %d", len(rows))
	}
	if got := rows[0]["be_evaluated_count"]; got != 1 {
		t.Fatalf("教师信电被评数应为 1, 得到 %v", got)
	}
}

// 历史任务无快照时回退教师人事学院。
func TestStatsLegacyTaskFallbackToTeacherCollege(t *testing.T) {
	db := evaluationTestDB(t)
	tm := model.College{Code: "TM2", Name: "土木与工程学院", Status: 1}
	if err := db.Create(&tm).Error; err != nil {
		t.Fatal(err)
	}
	teacher := model.User{UserNo: "T931", Username: "历史教师", Role: model.RoleTeacher, CollegeID: &tm.ID, Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "历史课程",
		ClassTime: model.LocalTimePtr(now), Status: model.TaskStatusPending,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if got := TaskCollegeID(db, &task); got == nil || *got != tm.ID {
		t.Fatalf("无快照任务应回退教师人事学院, 得到 %v", got)
	}
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	total, _, _, _, _ := taskStatsFor(db, taskStatsInput{
		CollegeIDs: []int{tm.ID}, ClassStart: &start, ClassEnd: &end,
	})
	if total != 1 {
		t.Fatalf("回退口径下土木任务数应为 1, 得到 %d", total)
	}
}
