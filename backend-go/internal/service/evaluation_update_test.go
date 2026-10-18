package service

import (
	"strings"
	"testing"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

func pstr(s string) *string { return &s }
func pbool(b bool) *bool    { return &b }

// updateEvalEnv 管理员修改评教记录（含课堂课程信息）测试环境：
// 甲/乙两学院、被评教师与同院候补教师、评教人、一条「已评」任务与一条记录。
func updateEvalEnv(t *testing.T) (*gorm.DB, *model.EvaluationTask, *model.EvaluationRecord, *int, *model.User) {
	t.Helper()
	db := evaluationTestDB(t)
	collegeA := model.College{Code: "A", Name: "甲学院", Status: 1}
	collegeB := model.College{Code: "B", Name: "乙学院", Status: 1}
	if err := db.Create(&collegeA).Error; err != nil {
		t.Fatalf("建学院失败: %v", err)
	}
	if err := db.Create(&collegeB).Error; err != nil {
		t.Fatalf("建学院失败: %v", err)
	}
	users := []model.User{
		{UserNo: "T001", Username: "被评教师", Role: model.RoleTeacher, CollegeID: &collegeA.ID, Status: 1},
		{UserNo: "T002", Username: "候补教师", Role: model.RoleTeacher, CollegeID: &collegeA.ID, Status: 1},
		{UserNo: "T003", Username: "评教人", Role: model.RoleTeacher, CollegeID: &collegeA.ID, Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	classTime := time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local)
	task := model.EvaluationTask{
		TeacherID: users[0].ID, TeacherName: users[0].Username, CourseName: "高等数学",
		ClassTime: model.LocalTimePtr(classTime), Classroom: pstr("1-101"),
		Status: model.TaskStatusEvaluated, EvaluationCount: 1, HasSupervisorEval: true,
		ScheduleSnapshot: []byte(`{"class_time_text":"周一 第1-2节","classroom":"1-101","class_info":"2401(50)","student_count":50,"week_pattern":"第1-16周"}`),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	rec := model.EvaluationRecord{
		TaskID: task.ID, EvaluatorID: &users[2].ID, EvaluatorName: users[2].Username,
		EvaluatorRole: model.RoleTeacher, IsAnonymous: true,
		DimensionValues: []byte(`{"score1":90}`),
	}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("建记录失败: %v", err)
	}
	collegeBID := collegeB.ID
	return db, &task, &rec, &collegeBID, &users[1]
}

func evalRecordByID(t *testing.T, db *gorm.DB, id int) model.EvaluationRecord {
	t.Helper()
	var r model.EvaluationRecord
	if err := db.Where("id = ?", id).First(&r).Error; err != nil {
		t.Fatalf("读取评教记录失败: %v", err)
	}
	return r
}

// 修改评教记录改用 Evaluation:update 权限门（不再借用 evaluation:delete）；
// 学院管理员按学院范围受限。
func TestUpdateEvaluationPermissionAndScope(t *testing.T) {
	db, task, rec, collegeB, _ := updateEvalEnv(t)
	svc := NewEvaluation()
	collegeAID := taskTeacherCollege(t, db, task.TeacherID)

	admin := &model.User{Model: model.Model{ID: 9001}, Username: "甲院管理员",
		Role: model.RoleCollegeAdmin, CollegeID: &collegeAID, Status: 1,
		Perms: []string{"evaluation:update"}}

	t.Run("持 evaluation:update 的本院管理员可改", func(t *testing.T) {
		if _, err := svc.Update(db, admin, rec.ID, UpdateParams{
			DimensionValues: map[string]interface{}{"score1": 88.0},
			IsAnonymous:     pbool(false),
		}); err != nil {
			t.Fatalf("管理员修改失败: %v", err)
		}
		got := evalRecordByID(t, db, rec.ID)
		if strings.Contains(string(got.DimensionValues), "90") {
			t.Fatalf("维度值未更新: %s", got.DimensionValues)
		}
		if got.IsAnonymous {
			t.Fatal("匿名标记未更新为 false")
		}
	})

	t.Run("无 evaluation:update 者被拒", func(t *testing.T) {
		sup := &model.User{Model: model.Model{ID: 9002}, Role: model.RoleSupervisor,
			CollegeID: &collegeAID, Status: 1, Perms: []string{"evaluation:view"}}
		_, err := svc.Update(db, sup, rec.ID, UpdateParams{
			DimensionValues: map[string]interface{}{"score1": 50.0},
			IsAnonymous:     pbool(false),
		})
		if err == nil || !strings.Contains(err.Error(), "无权修改评教记录") {
			t.Fatalf("无权限应被拒, 实际 err=%v", err)
		}
	})

	t.Run("跨学院管理员被拒", func(t *testing.T) {
		other := &model.User{Model: model.Model{ID: 9003}, Role: model.RoleCollegeAdmin,
			CollegeID: collegeB, Status: 1, Perms: []string{"evaluation:update"}}
		_, err := svc.Update(db, other, rec.ID, UpdateParams{
			DimensionValues: map[string]interface{}{"score1": 50.0},
			IsAnonymous:     pbool(false),
		})
		if err == nil || !strings.Contains(err.Error(), "本学院") {
			t.Fatalf("跨学院修改应被拒, 实际 err=%v", err)
		}
	})
}

// taskTeacherCollege 读取被评教师的学院 ID
func taskTeacherCollege(t *testing.T, db *gorm.DB, teacherID int) int {
	t.Helper()
	u, err := LoadTeacherUser(db, teacherID)
	if err != nil || u.CollegeID == nil {
		t.Fatalf("读取被评教师学院失败: %v", err)
	}
	return *u.CollegeID
}

// 记录编辑可同时改课堂课程信息（改任务本身）：教师/课程/时间/教室/课表快照
// 与记录维度在同一事务内生效，任务聚合口径不因课程信息修改而变化。
func TestUpdateEvaluationWithCourseInfo(t *testing.T) {
	db, task, rec, _, other := updateEvalEnv(t)
	svc := NewEvaluation()
	collegeAID := taskTeacherCollege(t, db, task.TeacherID)
	admin := &model.User{Model: model.Model{ID: 9001}, Username: "甲院管理员",
		Role: model.RoleCollegeAdmin, CollegeID: &collegeAID, Status: 1,
		Perms: []string{"evaluation:update", "task:update"}}

	newTime := time.Date(2026, 3, 3, 10, 0, 0, 0, time.Local)
	_, err := svc.Update(db, admin, rec.ID, UpdateParams{
		DimensionValues: map[string]interface{}{"score1": 95.0},
		IsAnonymous:     pbool(true),
		Task: &TaskCourseUpdate{
			TeacherID:  &other.ID,
			CourseName: pstr("线性代数"),
			ClassTime:  model.LocalTimePtr(newTime),
			Classroom:  pstr("2-202"),
			ScheduleSnapshot: &ScheduleSnapshot{
				ClassTimeText: pstr("周二 第3-4节"), Classroom: pstr("2-202"),
				ClassInfo: pstr("2402(60)"), StudentCount: intPtr(60), WeekPattern: pstr("第1-8周"),
			},
		},
	})
	if err != nil {
		t.Fatalf("修改课堂课程信息失败: %v", err)
	}

	tk := taskAfter(t, db, task.ID)
	if tk.TeacherID != other.ID || tk.TeacherName != other.Username {
		t.Fatalf("教师未更新: teacher_id=%d teacher_name=%q", tk.TeacherID, tk.TeacherName)
	}
	if tk.CourseName != "线性代数" || tk.Classroom == nil || *tk.Classroom != "2-202" {
		t.Fatalf("课程信息未更新: %+v", tk)
	}
	if tk.ClassTime == nil || !tk.ClassTime.ToTime().Equal(newTime) {
		t.Fatalf("上课时间未更新: %v", tk.ClassTime)
	}
	if !strings.Contains(string(tk.ScheduleSnapshot), "2402(60)") {
		t.Fatalf("课表快照未更新: %s", tk.ScheduleSnapshot)
	}
	if tk.EvaluationCount != 1 || !tk.HasSupervisorEval || tk.Status != model.TaskStatusEvaluated {
		t.Fatalf("任务聚合口径不应随课程信息修改变化: %+v", tk)
	}

	got := evalRecordByID(t, db, rec.ID)
	if !got.IsAnonymous {
		t.Fatal("记录匿名标记应随同一次保存生效")
	}
}

// 无 task:update 者不得借记录编辑入口改课堂信息
func TestUpdateEvaluationTaskFieldsRequireTaskUpdatePerm(t *testing.T) {
	db, task, rec, _, other := updateEvalEnv(t)
	svc := NewEvaluation()
	collegeAID := taskTeacherCollege(t, db, task.TeacherID)
	admin := &model.User{Model: model.Model{ID: 9001}, Role: model.RoleCollegeAdmin,
		CollegeID: &collegeAID, Status: 1, Perms: []string{"evaluation:update"}}

	_, err := svc.Update(db, admin, rec.ID, UpdateParams{
		Task: &TaskCourseUpdate{TeacherID: &other.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "无权修改课堂信息") {
		t.Fatalf("无 task:update 应被拒, 实际 err=%v", err)
	}
}

// 课程信息与记录维度一次性保存：任一侧失败整体回滚，不留半更新。
func TestUpdateEvaluationWithCourseInfoAtomic(t *testing.T) {
	svc := NewEvaluation()
	newTime := time.Date(2026, 3, 3, 10, 0, 0, 0, time.Local)

	t.Run("课堂信息冲突时记录维度不落库", func(t *testing.T) {
		db, task, rec, _, other := updateEvalEnv(t)
		collegeAID := taskTeacherCollege(t, db, task.TeacherID)
		admin := &model.User{Model: model.Model{ID: 9001}, Role: model.RoleCollegeAdmin,
			CollegeID: &collegeAID, Status: 1, Perms: []string{"evaluation:update", "task:update"}}

		dup := model.EvaluationTask{
			TeacherID: other.ID, TeacherName: other.Username, CourseName: "线性代数",
			ClassTime: model.LocalTimePtr(newTime), Status: model.TaskStatusPending,
		}
		if err := db.Create(&dup).Error; err != nil {
			t.Fatalf("建重复任务失败: %v", err)
		}

		_, err := svc.Update(db, admin, rec.ID, UpdateParams{
			DimensionValues: map[string]interface{}{"score1": 66.0},
			IsAnonymous:     pbool(false),
			Task: &TaskCourseUpdate{
				TeacherID: &other.ID, CourseName: pstr("线性代数"), ClassTime: model.LocalTimePtr(newTime),
			},
		})
		if err == nil || !strings.Contains(err.Error(), "已存在相同课程") {
			t.Fatalf("重复课堂应报可读错误, 实际 err=%v", err)
		}
		got := evalRecordByID(t, db, rec.ID)
		if !strings.Contains(string(got.DimensionValues), "90") {
			t.Fatalf("任务侧失败后记录维度不应落库: %s", got.DimensionValues)
		}
		if got.IsAnonymous != true {
			t.Fatal("任务侧失败后匿名标记不应落库")
		}
	})

	t.Run("维度校验失败时课堂信息不落库", func(t *testing.T) {
		db, task, rec, _, _ := updateEvalEnv(t)
		collegeAID := taskTeacherCollege(t, db, task.TeacherID)
		admin := &model.User{Model: model.Model{ID: 9001}, Role: model.RoleCollegeAdmin,
			CollegeID: &collegeAID, Status: 1, Perms: []string{"evaluation:update", "task:update"}}

		_, err := svc.Update(db, admin, rec.ID, UpdateParams{
			DimensionValues: map[string]interface{}{"score1": 200.0},
			IsAnonymous:     pbool(false),
			Task:            &TaskCourseUpdate{CourseName: pstr("改了但应回滚")},
		})
		if err == nil {
			t.Fatal("维度越界应报错")
		}
		tk := taskAfter(t, db, task.ID)
		if tk.CourseName != "高等数学" {
			t.Fatalf("维度校验失败后课程名不应落库: %q", tk.CourseName)
		}
	})
}

// 仅改匿名标记（不重复提交维度作答）也应可用——评教内容各字段互相独立
func TestUpdateEvaluationAnonymousOnly(t *testing.T) {
	db, task, rec, _, _ := updateEvalEnv(t)
	svc := NewEvaluation()
	collegeAID := taskTeacherCollege(t, db, task.TeacherID)
	admin := &model.User{Model: model.Model{ID: 9001}, Role: model.RoleCollegeAdmin,
		CollegeID: &collegeAID, Status: 1, Perms: []string{"evaluation:update"}}

	if _, err := svc.Update(db, admin, rec.ID, UpdateParams{IsAnonymous: pbool(false)}); err != nil {
		t.Fatalf("仅改匿名标记应放行: %v", err)
	}
	got := evalRecordByID(t, db, rec.ID)
	if got.IsAnonymous {
		t.Fatal("匿名标记未更新")
	}
	if !strings.Contains(string(got.DimensionValues), "90") {
		t.Fatalf("未提交维度作答时不应改动维度值: %s", got.DimensionValues)
	}
}

// 关联任务已删除时不允许从记录入口改课程信息，提示先恢复。
func TestUpdateEvaluationCourseInfoOnDeletedTask(t *testing.T) {
	db, task, rec, _, other := updateEvalEnv(t)
	svc := NewEvaluation()
	collegeAID := taskTeacherCollege(t, db, task.TeacherID)
	admin := &model.User{Model: model.Model{ID: 9001}, Role: model.RoleCollegeAdmin,
		CollegeID: &collegeAID, Status: 1, Perms: []string{"evaluation:update", "task:update"}}

	if err := db.Model(&model.EvaluationTask{}).Where("id = ?", task.ID).
		Update("is_deleted", true).Error; err != nil {
		t.Fatalf("软删任务失败: %v", err)
	}
	_, err := svc.Update(db, admin, rec.ID, UpdateParams{
		Task: &TaskCourseUpdate{TeacherID: &other.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "恢复") {
		t.Fatalf("已删除任务应提示先恢复, 实际 err=%v", err)
	}
}
