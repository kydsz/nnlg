package handler_test

import (
	"net/http"
	"strconv"
	"testing"

	"backend-go/internal/model"
)

// 督导删除「自己已提交的评教记录」的可行路径排查（反馈回路）。
//
// 背景：线上「督导老师」角色未勾选「评教记录·作废评教记录」(evaluation:delete)
// 与「评教记录·仅作废自己的评教记录」(evaluation:delete_own)，但有督导把自己
// 已提交的评教记录删掉了。本测试逐条验证督导能触达的记录作废/修改入口：
//
//	1) DELETE /evaluations/:id —— 期望被拒（无 evaluation:delete*）
//	2) PUT    /evaluations/:id —— 期望被拒（修改同样要求 evaluation:delete）
//	3) DELETE /tasks/:id       —— 期望放行（持 task:delete_own 且任务自建），
//	                              且任务名下评教记录**保留**（删任务不抹掉评教事实）
//
// 若 1/2 通过（200），说明 service 层权限门被绕过（真 bug）；
// 若 1/2 被拒而 3 通过且记录保留，即「删除任务只清理排课安排」的最终语义。
func TestSupervisorCannotDeleteSubmittedEvaluation(t *testing.T) {
	env := newTestServer(t)
	if err := env.db.AutoMigrate(
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
	); err != nil {
		t.Fatalf("补齐评教相关表失败: %v", err)
	}

	// 线上「督导老师」角色的权限集合：有 task:delete_own，无 evaluation:delete*
	env.seedRole(t, model.RoleSupervisor, "督导老师", 1, []string{
		"task:view", "task:view_all", "task:create", "task:update", "task:delete_own",
		"evaluation:view", "evaluation:create", "stats:view", "schedule:view",
	})

	college := model.College{Code: "WL", Name: "文理学院", Status: 1}
	if err := env.db.Create(&college).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	teacher := &model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: &college.ID, Status: 1}
	env.seedUser(t, teacher)
	sup := &model.User{UserNo: "S001", Username: "督导老师", Role: model.RoleSupervisor, CollegeID: &college.ID, Status: 1}
	env.seedUser(t, sup)
	env.seedUserRole(t, sup.ID, model.RoleSupervisor)

	// 已评任务：创建者是督导自己（移动端课表页把自己/本院的课加入待评任务的典型场景）
	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "高等数学",
		Status: model.TaskStatusEvaluated, CreateBy: &sup.ID,
	}
	if err := env.db.Create(&task).Error; err != nil {
		t.Fatalf("灌入任务失败: %v", err)
	}
	rec := model.EvaluationRecord{
		TaskID: task.ID, EvaluatorID: &sup.ID, EvaluatorName: sup.Username,
		EvaluatorRole: model.RoleSupervisor, DimensionValues: []byte(`{"score1":90}`),
	}
	if err := env.db.Create(&rec).Error; err != nil {
		t.Fatalf("灌入评教记录失败: %v", err)
	}

	header := env.authHeader(t, int64(sup.ID))
	evalPath := "/api/v1/evaluations/" + strconv.Itoa(rec.ID)

	recordDeleted := func() bool {
		var row model.EvaluationRecord
		if err := env.db.Where("id = ?", rec.ID).First(&row).Error; err != nil {
			t.Fatalf("读取评教记录失败: %v", err)
		}
		return row.IsDeleted
	}

	t.Run("直删评教记录被拒绝", func(t *testing.T) {
		w := env.do(t, http.MethodDelete, evalPath, "", header)
		t.Logf("DELETE %s -> %d body=%s", evalPath, w.Code, w.Body.String())
		if w.Code == http.StatusOK {
			t.Fatalf("无 evaluation:delete* 却删除成功：service 层权限门被绕过")
		}
		if recordDeleted() {
			t.Fatalf("请求被拒但记录已被软删：存在半提交/绕过")
		}
	})

	t.Run("修改评教记录被拒绝", func(t *testing.T) {
		w := env.do(t, http.MethodPut, evalPath,
			`{"dimension_values":{"score1":60},"is_anonymous":false}`, header)
		t.Logf("PUT %s -> %d body=%s", evalPath, w.Code, w.Body.String())
		if w.Code == http.StatusOK {
			t.Fatalf("无 evaluation:delete 却修改成功：service 层权限门被绕过")
		}
	})

	t.Run("删除他人创建的任务被拒绝", func(t *testing.T) {
		otherTask := model.EvaluationTask{
			TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "线性代数",
			Status: model.TaskStatusEvaluated, CreateBy: &teacher.ID,
		}
		if err := env.db.Create(&otherTask).Error; err != nil {
			t.Fatalf("灌入任务失败: %v", err)
		}
		otherPath := "/api/v1/tasks/" + strconv.Itoa(otherTask.ID)
		w := env.do(t, http.MethodDelete, otherPath, "", header)
		t.Logf("DELETE %s -> %d body=%s", otherPath, w.Code, w.Body.String())
		if w.Code == http.StatusOK {
			t.Fatalf("仅持 task:delete_own 不应能删除他人创建的任务")
		}
	})

	t.Run("删除自己创建的任务后评教记录保留", func(t *testing.T) {
		w := env.do(t, http.MethodDelete, "/api/v1/tasks/"+strconv.Itoa(task.ID), "", header)
		t.Logf("DELETE /api/v1/tasks/%d -> %d body=%s", task.ID, w.Code, w.Body.String())
		if w.Code != http.StatusOK {
			t.Fatalf("持 task:delete_own 删自建任务应放行, 得到 %d", w.Code)
		}
		var tk model.EvaluationTask
		if err := env.db.Where("id = ?", task.ID).First(&tk).Error; err != nil {
			t.Fatalf("读取任务失败: %v", err)
		}
		if !tk.IsDeleted {
			t.Fatalf("任务应已软删（可恢复）")
		}
		if recordDeleted() {
			t.Fatalf("删除任务不应影响评教记录：记录被连带软删")
		}
		// 删除任务只清理排课安排：记录仍应出现在记录列表（我评过的）
		lw := env.do(t, http.MethodGet, "/api/v1/evaluations?type=sent", "", header)
		if lw.Code != http.StatusOK {
			t.Fatalf("GET /evaluations?type=sent 状态码=%d body=%s", lw.Code, lw.Body.String())
		}
		var resp struct {
			Data struct {
				List []struct {
					ID int `json:"id"`
				} `json:"list"`
			} `json:"data"`
		}
		decodeJSON(t, lw.Body.Bytes(), &resp)
		found := false
		for _, it := range resp.Data.List {
			if it.ID == rec.ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("删除任务后记录 %d 应仍在记录列表中, 实际列表=%+v", rec.ID, resp.Data.List)
		}
	})
}
