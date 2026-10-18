package handler_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"backend-go/internal/model"
)

// 管理员经 PUT /evaluations/:id 修改评教记录（含课堂课程信息）的端到端验收：
// 路由放行、JSON 绑定（is_anonymous 指针 + task 嵌套）、课程信息落库、操作日志留痕。
func TestAdminUpdateEvaluationCourseInfoAPI(t *testing.T) {
	env := newTestServer(t)
	if err := env.db.AutoMigrate(
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
	); err != nil {
		t.Fatalf("补齐评教相关表失败: %v", err)
	}
	env.seedRole(t, model.RoleCollegeAdmin, "学院管理员", 1, []string{
		"evaluation:view", "evaluation:update", "task:view", "task:update",
	})

	college := model.College{Code: "WL", Name: "文理学院", Status: 1}
	if err := env.db.Create(&college).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	teacher := &model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1}
	env.seedUser(t, teacher)
	other := &model.User{UserNo: "T002", Username: "目标教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1}
	env.seedUser(t, other)
	admin := &model.User{UserNo: "A001", Username: "学院管理员", Role: model.RoleCollegeAdmin, CollegeID: &college.ID, Status: 1}
	env.seedUser(t, admin)
	env.seedUserRole(t, admin.ID, model.RoleCollegeAdmin)

	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "高等数学",
		Status: model.TaskStatusEvaluated, EvaluationCount: 1,
	}
	if err := env.db.Create(&task).Error; err != nil {
		t.Fatalf("灌入任务失败: %v", err)
	}
	rec := model.EvaluationRecord{
		TaskID: task.ID, EvaluatorID: &admin.ID, EvaluatorName: admin.Username,
		EvaluatorRole: model.RoleCollegeAdmin, DimensionValues: []byte(`{"score1":90}`),
	}
	if err := env.db.Create(&rec).Error; err != nil {
		t.Fatalf("灌入评教记录失败: %v", err)
	}

	body := `{"is_anonymous":true,"task":{"teacher_id":` + strconv.Itoa(other.ID) +
		`,"course_name":"线性代数","schedule":{"classroom":"2-202","class_info":"2402(60)"}}}`
	w := env.do(t, http.MethodPut, "/api/v1/evaluations/"+strconv.Itoa(rec.ID), body,
		env.authHeader(t, int64(admin.ID)))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /evaluations/%d 状态码=%d body=%s", rec.ID, w.Code, w.Body.String())
	}

	var gotTask model.EvaluationTask
	if err := env.db.First(&gotTask, task.ID).Error; err != nil {
		t.Fatalf("读取任务失败: %v", err)
	}
	if gotTask.TeacherID != other.ID || gotTask.TeacherName != other.Username {
		t.Fatalf("换教师未同步快照: teacher_id=%d teacher_name=%q", gotTask.TeacherID, gotTask.TeacherName)
	}
	if gotTask.CourseName != "线性代数" {
		t.Fatalf("课程名未更新: %q", gotTask.CourseName)
	}
	if !strings.Contains(string(gotTask.ScheduleSnapshot), "2402(60)") {
		t.Fatalf("课表快照未更新: %s", gotTask.ScheduleSnapshot)
	}
	if gotTask.EvaluationCount != 1 {
		t.Fatalf("换课程信息不应改评教数: %d", gotTask.EvaluationCount)
	}

	var gotRec model.EvaluationRecord
	if err := env.db.First(&gotRec, rec.ID).Error; err != nil {
		t.Fatalf("读取记录失败: %v", err)
	}
	if !gotRec.IsAnonymous {
		t.Fatal("匿名标记未更新")
	}
	if gotRec.EvaluatorID == nil || *gotRec.EvaluatorID != admin.ID {
		t.Fatal("评教人不应被修改")
	}

	var logs int64
	env.db.Model(&model.OperationLog{}).
		Where("module = ? AND operation_type = ? AND target_id = ?", "evaluation", "update", rec.ID).
		Count(&logs)
	if logs == 0 {
		t.Fatal("修改评教记录应写入操作日志")
	}
}
