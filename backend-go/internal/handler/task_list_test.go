package handler_test

import (
	"net/http"
	"testing"

	"backend-go/internal/model"
)

// taskListResp 任务列表响应（仅关注 create_by_name 相关字段）。
type taskListResp struct {
	Code int `json:"code"`
	Data struct {
		List []struct {
			ID           int     `json:"id"`
			CourseName   string  `json:"course_name"`
			CreateBy     *int    `json:"create_by"`
			CreateByName *string `json:"create_by_name"`
		} `json:"list"`
		Total int64 `json:"total"`
	} `json:"data"`
}

// TestTaskListCreatorName 任务列表应正确返回创建人姓名。
// 背景：批量查创建人若用 Scan（model.User 含指针字段 + Select 组合）会报
// unsupported data type 且错误被忽略，导致 create_by_name 恒为 null。
func TestTaskListCreatorName(t *testing.T) {
	env := newTestServer(t)
	// 任务列表链路依赖任务/评教记录/评教维度表，测试库需补齐
	if err := env.db.AutoMigrate(
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
	); err != nil {
		t.Fatalf("补齐任务相关表失败: %v", err)
	}

	admin := systemAdminUser()
	env.seedUser(t, admin)
	creator := &model.User{UserNo: "U100", Username: "张创建", Role: model.RoleTeacher, Status: 1}
	env.seedUser(t, creator)

	cb := creator.ID
	task := model.EvaluationTask{
		TeacherID: 999999, TeacherName: "李老师", CourseName: "高等数学",
		Status: model.TaskStatusPending, CreateBy: &cb,
	}
	if err := env.db.Create(&task).Error; err != nil {
		t.Fatalf("灌入任务失败: %v", err)
	}

	w := env.do(t, http.MethodGet, "/api/v1/tasks", "", env.authHeader(t, int64(admin.ID)))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /tasks 状态码=%d, body=%s", w.Code, w.Body.String())
	}

	var resp taskListResp
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != http.StatusOK {
		t.Fatalf("响应 code=%d, body=%s", resp.Code, w.Body.String())
	}
	if len(resp.Data.List) != 1 {
		t.Fatalf("应返回 1 条任务, 得到 %d 条, body=%s", len(resp.Data.List), w.Body.String())
	}
	got := resp.Data.List[0]
	if got.CreateBy == nil || *got.CreateBy != creator.ID {
		t.Fatalf("create_by 应为 %d, 得到 %v", creator.ID, got.CreateBy)
	}
	if got.CreateByName == nil || *got.CreateByName != "张创建" {
		t.Fatalf("create_by_name 应为 张创建, 得到 %v", got.CreateByName)
	}
}
