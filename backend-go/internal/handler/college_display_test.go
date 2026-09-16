package handler_test

import (
	"net/http"
	"testing"
	"time"

	"backend-go/internal/model"
)

// 学院展示回归：评教任务列表与评教记录列表必须返回「被评教师所属学院」。
//
// 背景：前端两处列（Tasks 的 teacher_college_name、Evaluations 的 college_name）早已按该
// 字段名渲染，但后端两个列表接口从不返回它们（只有详情返回），于是「学院」列恒为 "-"。
// 口径：被评教师的主学院（user.college_id），与任务可见范围、统计报表一致。
func TestListsExposeTeacherCollege(t *testing.T) {
	env := newTestServer(t)
	if err := env.db.AutoMigrate(
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
	); err != nil {
		t.Fatalf("补齐评教相关表失败: %v", err)
	}

	admin := systemAdminUser()
	env.seedUser(t, admin)

	college := model.College{Code: "WL", Name: "文理学院", Status: 1}
	if err := env.db.Create(&college).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	teacher := &model.User{
		UserNo: "T001", Username: "李老师", Role: model.RoleTeacher,
		CollegeID: &college.ID, Status: 1,
	}
	env.seedUser(t, teacher)
	// 未绑定学院的教师：字段应为 null（前端渲染 "-"），而不是报错或空串
	noCollege := &model.User{UserNo: "T002", Username: "王老师", Role: model.RoleTeacher, Status: 1}
	env.seedUser(t, noCollege)

	classTime := model.LocalTime(time.Now())
	tasks := []model.EvaluationTask{
		{
			TeacherID: teacher.ID, TeacherName: "李老师", CourseName: "高等数学",
			ClassTime: &classTime, Status: model.TaskStatusPending,
		},
		{
			TeacherID: noCollege.ID, TeacherName: "王老师", CourseName: "大学英语",
			ClassTime: &classTime, Status: model.TaskStatusPending,
		},
	}
	if err := env.db.Create(&tasks).Error; err != nil {
		t.Fatalf("灌入任务失败: %v", err)
	}
	rec := model.EvaluationRecord{
		TaskID: tasks[0].ID, EvaluatorRole: model.RoleTeacher, SubmitTime: &classTime,
	}
	if err := env.db.Create(&rec).Error; err != nil {
		t.Fatalf("灌入评教记录失败: %v", err)
	}

	h := env.authHeader(t, int64(admin.ID))

	t.Run("评教任务列表返回 teacher_college_name", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/tasks", "", h)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /tasks 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Data struct {
				List []struct {
					TeacherName        string  `json:"teacher_name"`
					TeacherCollegeID   *int    `json:"teacher_college_id"`
					TeacherCollegeName *string `json:"teacher_college_name"`
				} `json:"list"`
			} `json:"data"`
		}
		decodeJSON(t, w.Body.Bytes(), &resp)
		if len(resp.Data.List) != 2 {
			t.Fatalf("应返回 2 条任务, 得到 %d, body=%s", len(resp.Data.List), w.Body.String())
		}
		byName := map[string]int{}
		for i, it := range resp.Data.List {
			byName[it.TeacherName] = i
		}

		got := resp.Data.List[byName["李老师"]]
		if got.TeacherCollegeName == nil || *got.TeacherCollegeName != "文理学院" {
			t.Fatalf("李老师所在学院应为 文理学院, 得到 %v", got.TeacherCollegeName)
		}
		if got.TeacherCollegeID == nil || *got.TeacherCollegeID != college.ID {
			t.Fatalf("teacher_college_id 应为 %d, 得到 %v", college.ID, got.TeacherCollegeID)
		}

		none := resp.Data.List[byName["王老师"]]
		if none.TeacherCollegeID != nil || none.TeacherCollegeName != nil {
			t.Fatalf("未绑定学院的教师应返回 null, 得到 id=%v name=%v",
				none.TeacherCollegeID, none.TeacherCollegeName)
		}
	})

	t.Run("评教记录列表返回 college_name", func(t *testing.T) {
		w := env.do(t, http.MethodGet,
			"/api/v1/evaluations?start_date=2000-01-01&end_date=2100-01-01", "", h)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /evaluations 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Data struct {
				List []struct {
					ID          int     `json:"id"`
					CollegeID   *int    `json:"college_id"`
					CollegeName *string `json:"college_name"`
				} `json:"list"`
			} `json:"data"`
		}
		decodeJSON(t, w.Body.Bytes(), &resp)
		if len(resp.Data.List) != 1 {
			t.Fatalf("应返回 1 条记录, 得到 %d, body=%s", len(resp.Data.List), w.Body.String())
		}
		got := resp.Data.List[0]
		if got.CollegeName == nil || *got.CollegeName != "文理学院" {
			t.Fatalf("记录所属学院应为 文理学院, 得到 %v", got.CollegeName)
		}
		if got.CollegeID == nil || *got.CollegeID != college.ID {
			t.Fatalf("college_id 应为 %d, 得到 %v", college.ID, got.CollegeID)
		}
	})
}
