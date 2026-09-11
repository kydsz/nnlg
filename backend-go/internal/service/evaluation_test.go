package service

import (
	"encoding/json"
	"testing"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func evaluationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.UserRole{}, &model.UserCollege{},
		&model.UserRoom{}, &model.College{}, &model.ResearchRoom{},
		&model.Role{},
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
		&model.EvaluationDraft{},
	); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if err := db.Create(&model.EvaluationDimension{
		Code: "score1", Name: "评分", FieldType: model.FieldScore,
		FieldConfig: []byte(`{"max_score":100,"min_score":0}`), IsRequired: true, Status: 1,
	}).Error; err != nil {
		t.Fatalf("建维度失败: %v", err)
	}
	return db
}

// 异步消费者攒批落库时，匿名标记必须原样写入 evaluation_record.is_anonymous
// （issue #13 验收 1/3 的"落库"级断言：解析出的 IsAnonymous 需贯穿到 DB 行）。
func TestPersistBatchKeepsAnonymous(t *testing.T) {
	db := evaluationTestDB(t)
	teacher := model.User{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatalf("建教师失败: %v", err)
	}
	viewer := model.User{UserNo: "T002", Username: "评教人", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("建评教人失败: %v", err)
	}
	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: teacher.Username, CourseName: "大学英语",
		Status: model.TaskStatusPending, CreateBy: intPtr(teacher.ID),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}

	svc := NewEvaluation()
	if err := svc.PersistBatch(db, []PendingSubmit{{
		MsgID: "m1", Task: task, EvaluatorRole: model.RoleTeacher,
		Params: SubmitParams{TaskID: task.ID, IsAnonymous: true, DimensionValues: map[string]interface{}{"score1": 90.0}},
		Viewer: &viewer,
	}}); err != nil {
		t.Fatalf("PersistBatch: %v", err)
	}

	var rec model.EvaluationRecord
	if err := db.First(&rec).Error; err != nil {
		t.Fatalf("读取落库记录: %v", err)
	}
	if !rec.IsAnonymous {
		t.Fatalf("匿名评教经 PersistBatch 落库后 is_anonymous 应为 true, 实际 false")
	}
}

// 多角色用户（user_role 同时有 teacher + supervisor，主角色列为 teacher）提交评教：
// 应记为督导评教（记录角色=督导、任务置 has_supervisor_eval），对齐督导权限分支
func TestSubmitMultiRoleSupervisor(t *testing.T) {
	db := evaluationTestDB(t)
	users := []model.User{
		{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
		{UserNo: "T002", Username: "督导兼教师", Role: model.RoleTeacher, CollegeID: intPtr(6), Status: 1},
		{UserNo: "T003", Username: "纯教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	// 督导兼教师：user_role 关联督导角色，负责文理学院(5)
	// （内存结构与库中记录保持一致，模拟 auth 中间件 Preload 后的 viewer）
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

	svc := NewEvaluation()

	t.Run("督导兼教师提交记为督导评教", func(t *testing.T) {
		rec, err := svc.Submit(db, &users[1], SubmitParams{
			TaskID: task.ID, DimensionValues: map[string]interface{}{"score1": 95.0},
		})
		if err != nil {
			t.Fatalf("提交失败: %v", err)
		}
		if rec.EvaluatorRole != model.RoleSupervisor {
			t.Fatalf("评教记录角色 = %q, 期望 supervisor", rec.EvaluatorRole)
		}
		var taskAfter model.EvaluationTask
		db.First(&taskAfter, task.ID)
		if !taskAfter.HasSupervisorEval {
			t.Fatal("多角色督导提交后任务应置 has_supervisor_eval = true")
		}
		if taskAfter.EvaluationCount != 1 {
			t.Fatalf("evaluation_count = %d, 期望 1", taskAfter.EvaluationCount)
		}
		if taskAfter.Status != model.TaskStatusEvaluated {
			t.Fatalf("任务状态 = %d, 期望已评(%d)", taskAfter.Status, model.TaskStatusEvaluated)
		}
	})

	t.Run("纯教师提交仍是同行评教", func(t *testing.T) {
		teacherTask := model.EvaluationTask{
			TeacherID: users[0].ID, TeacherName: "被评教师", CourseName: "大学物理",
			Status: model.TaskStatusPending, CreateBy: intPtr(users[0].ID),
		}
		if err := db.Create(&teacherTask).Error; err != nil {
			t.Fatalf("建任务失败: %v", err)
		}
		rec, err := svc.Submit(db, &users[2], SubmitParams{
			TaskID: teacherTask.ID, DimensionValues: map[string]interface{}{"score1": 88.0},
		})
		if err != nil {
			t.Fatalf("提交失败: %v", err)
		}
		if rec.EvaluatorRole != model.RoleTeacher {
			t.Fatalf("评教记录角色 = %q, 期望 teacher", rec.EvaluatorRole)
		}
		var taskAfter model.EvaluationTask
		db.First(&taskAfter, teacherTask.ID)
		if taskAfter.HasSupervisorEval {
			t.Fatal("教师同行评教不应置 has_supervisor_eval")
		}
	})
}

func TestSupervisorRoleOf(t *testing.T) {
	cases := []struct {
		name  string
		role  string
		roles []string
		want  string
	}{
		{"多角色含督导取督导", model.RoleTeacher, []string{model.RoleTeacher, model.RoleSupervisor}, model.RoleSupervisor},
		{"校级督导优先", model.RoleSupervisor, []string{model.RoleCollegeSupervisor, model.RoleSchoolSupervisor}, model.RoleSchoolSupervisor},
		{"仅主角色督导", model.RoleCollegeSupervisor, nil, model.RoleCollegeSupervisor},
		{"非督导返回空", model.RoleTeacher, nil, ""},
		{"仅教师关联", model.RoleTeacher, []string{model.RoleTeacher}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := model.User{Role: c.role}
			for _, r := range c.roles {
				u.UserRoles = append(u.UserRoles, model.UserRole{Role: r})
			}
			if got := u.SupervisorRole(); got != c.want {
				t.Fatalf("SupervisorRole() = %q, 期望 %q", got, c.want)
			}
		})
	}
}

// 回归：matchScheduleForTask 的 student_count 必须是 int/nil 而非 *int——
// 导出（PDF/HTML）用 fmt.Sprint 渲染，指针会打出 0x... 内存地址
//（线上截图「应到人数0x2b6082351468人」）。
func TestMatchScheduleForTaskStudentCountPlainInt(t *testing.T) {
	snap := `{"class_time_text":"周六 第5-6节","classroom":"1-101","class_info":"2401(125)","student_count":125,"week_pattern":"第1-16周"}`
	out := matchScheduleForTask(nil, &model.EvaluationTask{
		ScheduleSnapshot: json.RawMessage(snap),
		CourseName:       "高等数学",
	})
	cnt, ok := out["student_count"].(int)
	if !ok || cnt != 125 {
		t.Fatalf("快照路径 student_count 应为 int 125, 得到 %T %v", out["student_count"], out["student_count"])
	}
	if out["classroom"] != "1-101" {
		t.Fatalf("快照路径 classroom 应为 1-101, 得到 %v", out["classroom"])
	}

	outNil := matchScheduleForTask(nil, &model.EvaluationTask{
		ScheduleSnapshot: json.RawMessage(`{"class_info":"2403","student_count":null}`),
		CourseName:       "无人数课程",
	})
	if outNil["student_count"] != nil {
		t.Fatalf("student_count 为 null 时应返回 nil, 得到 %T %v", outNil["student_count"], outNil["student_count"])
	}
}
