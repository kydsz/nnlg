package handler_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"backend-go/internal/model"
)

// 评教任务可见范围回归：未获 task:view_all 的普通教师只能看到「与我相关」的任务
//（我创建的 / 我被评的 / 我评过的）；本学院其他教师的任务（例如督导排进待评计划的课）不可见。
// 收敛在服务端完成，前端去掉/伪造 create_by 参数也拿不到无关任务；详情接口同一口径。
func TestTeacherTaskVisibilityScopedToSelf(t *testing.T) {
	env := newTestServer(t)
	if err := env.db.AutoMigrate(
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
	); err != nil {
		t.Fatalf("补齐评教相关表失败: %v", err)
	}
	env.seedRole(t, model.RoleTeacher, "教师", 1, []string{"task:view", "task:create"})
	env.seedRole(t, "teacher_va", "已放开可见的教师", 1,
		[]string{"task:view", "task:create", "task:view_all"})

	college := model.College{Code: "WL", Name: "文理学院", Status: 1}
	if err := env.db.Create(&college).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	mk := func(no, name, role string) *model.User {
		u := &model.User{UserNo: no, Username: name, Role: role, CollegeID: &college.ID, Status: 1}
		env.seedUser(t, u)
		return u
	}
	me := mk("T001", "本人", model.RoleTeacher)
	other := mk("T002", "同院同事", model.RoleTeacher)
	third := mk("T003", "另一位同事", model.RoleTeacher)
	granted := mk("T004", "已放开可见", "teacher_va")

	classTime := model.LocalTime(time.Now())
	newTask := func(teacher, creator *model.User, course string) model.EvaluationTask {
		tk := model.EvaluationTask{
			TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: course,
			ClassTime: &classTime, Status: model.TaskStatusPending, CreateBy: &creator.ID,
		}
		if err := env.db.Create(&tk).Error; err != nil {
			t.Fatalf("灌入任务失败: %v", err)
		}
		return tk
	}
	mine := newTask(other, me, "我创建的课")         // 我创建的
	aboutMe := newTask(me, other, "我被评的课")       // 我被评的
	unrelated := newTask(third, other, "与我无关的课") // 与我无关（同事被排进的待评计划）
	evaluated := newTask(third, other, "我评过的课")   // 我评过的
	if err := env.db.Create(&model.EvaluationRecord{
		TaskID: evaluated.ID, EvaluatorID: &me.ID, EvaluatorName: me.Username,
		EvaluatorRole: model.RoleTeacher,
	}).Error; err != nil {
		t.Fatalf("灌入评教记录失败: %v", err)
	}
	// 跨学院任务：用于校验详情接口在 task:view_all 之上还叠了一层学院范围
	collegeB := model.College{Code: "XX", Name: "信息学院", Status: 1}
	if err := env.db.Create(&collegeB).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	outsider := &model.User{
		UserNo: "T900", Username: "外院教师", Role: model.RoleTeacher,
		CollegeID: &collegeB.ID, Status: 1,
	}
	env.seedUser(t, outsider)
	foreign := newTask(outsider, other, "外院老师的课")

	visibleIDs := func(t *testing.T, u *model.User, query string) map[int]bool {
		t.Helper()
		w := env.do(t, http.MethodGet, "/api/v1/tasks"+query, "", env.authHeader(t, int64(u.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /tasks%s 状态码=%d, body=%s", query, w.Code, w.Body.String())
		}
		var resp struct {
			Data struct {
				List []struct {
					ID int `json:"id"`
				} `json:"list"`
			} `json:"data"`
		}
		decodeJSON(t, w.Body.Bytes(), &resp)
		got := map[int]bool{}
		for _, it := range resp.Data.List {
			got[it.ID] = true
		}
		return got
	}

	t.Run("教师默认只见与我相关的任务", func(t *testing.T) {
		got := visibleIDs(t, me, "")
		for _, want := range []model.EvaluationTask{mine, aboutMe, evaluated} {
			if !got[want.ID] {
				t.Fatalf("任务 %d（%s）应可见, 实际可见集合=%v", want.ID, want.CourseName, got)
			}
		}
		if got[unrelated.ID] {
			t.Fatalf("与我无关的任务 %d（%s）不应可见", unrelated.ID, unrelated.CourseName)
		}
	})

	t.Run("伪造创建人筛选也拿不到无关任务", func(t *testing.T) {
		got := visibleIDs(t, me, "?create_by_not="+strconv.Itoa(me.ID))
		if got[unrelated.ID] {
			t.Fatalf("传 create_by_not 后仍不应看到无关任务 %d", unrelated.ID)
		}
	})

	t.Run("详情接口同一口径", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/tasks/"+strconv.Itoa(unrelated.ID), "",
			env.authHeader(t, int64(me.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("他人任务详情应 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
		w = env.do(t, http.MethodGet, "/api/v1/tasks/"+strconv.Itoa(aboutMe.ID), "",
			env.authHeader(t, int64(me.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("我被评的任务详情应 200, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("持有 task:view_all 后可见本学院他人任务", func(t *testing.T) {
		got := visibleIDs(t, granted, "")
		if !got[unrelated.ID] {
			t.Fatalf("持有 task:view_all 应可见本学院他人任务 %d, 实际可见集合=%v", unrelated.ID, got)
		}
		if got[foreign.ID] {
			t.Fatalf("task:view_all 只解除「与我相关」这层，跨学院任务 %d 仍不可见", foreign.ID)
		}
	})

	t.Run("详情接口叠了学院范围：持权限也不能跨学院看", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/tasks/"+strconv.Itoa(foreign.ID), "",
			env.authHeader(t, int64(granted.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("跨学院任务详情应 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("系统管理员可跨学院看详情", func(t *testing.T) {
		admin := systemAdminUser()
		env.seedUser(t, admin)
		w := env.do(t, http.MethodGet, "/api/v1/tasks/"+strconv.Itoa(foreign.ID), "",
			env.authHeader(t, int64(admin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("系统管理员应可跨学院查看详情, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})
}
