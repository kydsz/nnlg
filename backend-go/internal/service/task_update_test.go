package service

import (
	"strings"
	"testing"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// taskUpdateEnv 任务编辑测试环境：甲/乙两学院、两名教师、一条已评任务。
func taskUpdateEnv(t *testing.T) (*gorm.DB, *model.EvaluationTask, *model.User, *model.User, int, int) {
	t.Helper()
	db := taskBatchTestDB(t)
	collegeA := 11
	collegeB := 22
	teacherAID := seedTaskTeacher(t, db, collegeA)
	other := model.User{UserNo: "T301", Username: "目标教师", Role: model.RoleTeacher,
		Status: 1, CollegeID: &collegeA}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("建目标教师失败: %v", err)
	}
	teacherB := model.User{UserNo: "T302", Username: "乙院教师", Role: model.RoleTeacher,
		Status: 1, CollegeID: &collegeB}
	if err := db.Create(&teacherB).Error; err != nil {
		t.Fatalf("建乙院教师失败: %v", err)
	}
	classTime := time.Date(2026, 3, 2, 8, 0, 0, 0, time.Local)
	task := model.EvaluationTask{
		TeacherID: teacherAID, TeacherName: "任课教师", CourseName: "高等数学",
		ClassTime: model.LocalTimePtr(classTime), Status: model.TaskStatusEvaluated,
		EvaluationCount: 1, HasSupervisorEval: true,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	return db, &task, &other, &teacherB, collegeA, collegeB
}

// 已有评教提交的任务仅管理员可编辑课程信息：督导即使持 task:update 也不允许。
func TestTaskUpdateEvaluatedRequiresAdmin(t *testing.T) {
	db, task, _, _, collegeAID, _ := taskUpdateEnv(t)
	svc := NewTask()

	sup := &model.User{Model: model.Model{ID: 8001}, Username: "督导",
		Role: model.RoleSupervisor, CollegeID: &collegeAID, Status: 1}
	_, err := svc.Update(db, sup, task.ID, UpdateTaskParams{CourseName: pstr("督导改的课")})
	if err == nil || !strings.Contains(err.Error(), "仅管理员") {
		t.Fatalf("督导改已评任务应被拒, 实际 err=%v", err)
	}
	if tk := taskAfter(t, db, task.ID); tk.CourseName != "高等数学" {
		t.Fatalf("被拒后课程名不应变化: %q", tk.CourseName)
	}

	admin := &model.User{Model: model.Model{ID: 8002}, Username: "院管理员",
		Role: model.RoleCollegeAdmin, CollegeID: &collegeAID, Status: 1}
	if _, err := svc.Update(db, admin, task.ID, UpdateTaskParams{CourseName: pstr("线性代数")}); err != nil {
		t.Fatalf("管理员改已评任务应放行: %v", err)
	}
}

// 管理员可更换被评教师（同步更新 teacher_name 快照）；目标教师需在范围内；
// 改后撞「同教师+同课程+同上课时间」的现存任务返回可读重复错误。
func TestTaskUpdateTeacherChangeAndDedupe(t *testing.T) {
	db, task, other, teacherB, collegeAID, _ := taskUpdateEnv(t)
	svc := NewTask()
	admin := &model.User{Model: model.Model{ID: 8002}, Username: "院管理员",
		Role: model.RoleCollegeAdmin, CollegeID: &collegeAID, Status: 1}

	t.Run("更换本院教师成功", func(t *testing.T) {
		if _, err := svc.Update(db, admin, task.ID, UpdateTaskParams{TeacherID: &other.ID}); err != nil {
			t.Fatalf("更换教师失败: %v", err)
		}
		tk := taskAfter(t, db, task.ID)
		if tk.TeacherID != other.ID || tk.TeacherName != other.Username {
			t.Fatalf("换教师未同步快照: teacher_id=%d teacher_name=%q", tk.TeacherID, tk.TeacherName)
		}
		if tk.EvaluationCount != 1 {
			t.Fatalf("换教师不应改评教数: %d", tk.EvaluationCount)
		}
	})

	t.Run("更换校外教师被拒", func(t *testing.T) {
		_, err := svc.Update(db, admin, task.ID, UpdateTaskParams{TeacherID: &teacherB.ID})
		if err == nil || !strings.Contains(err.Error(), "本学院") {
			t.Fatalf("跨学院换教师应被拒, 实际 err=%v", err)
		}
	})

	t.Run("与现存任务重复被拒", func(t *testing.T) {
		newTime := time.Date(2026, 3, 3, 10, 0, 0, 0, time.Local)
		dup := model.EvaluationTask{
			TeacherID: other.ID, TeacherName: other.Username, CourseName: "概率论",
			ClassTime: model.LocalTimePtr(newTime), Status: model.TaskStatusPending,
		}
		if err := db.Create(&dup).Error; err != nil {
			t.Fatalf("建重复任务失败: %v", err)
		}
		_, err := svc.Update(db, admin, task.ID, UpdateTaskParams{
			CourseName: pstr("概率论"), ClassTime: model.LocalTimePtr(newTime),
		})
		if err == nil || !strings.Contains(err.Error(), "已存在相同课程") {
			t.Fatalf("重复任务应返回可读错误, 实际 err=%v", err)
		}
	})
}

// 去重键未变化的编辑不应触发查重：历史重复数据下，只改教室/原样回传教师等
// 无关编辑不应被「已存在相同课程」拦住。
func TestTaskUpdateUnchangedKeySkipsDedupe(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 11
	teacherID := seedTaskTeacher(t, db, collegeID)
	classTime := time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local)
	mk := func() model.EvaluationTask {
		return model.EvaluationTask{
			TeacherID: teacherID, TeacherName: "任课教师", CourseName: "编译原理",
			ClassTime: model.LocalTimePtr(classTime), Status: model.TaskStatusPending,
		}
	}
	t1, t2 := mk(), mk()
	if err := db.Create(&t1).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	if err := db.Create(&t2).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	admin := &model.User{Model: model.Model{ID: 8002}, Username: "院管理员",
		Role: model.RoleCollegeAdmin, CollegeID: &collegeID, Status: 1}
	svc := NewTask()

	// 只改教室（去重键不变）
	if _, err := svc.Update(db, admin, t1.ID, UpdateTaskParams{Classroom: pstr("3-303")}); err != nil {
		t.Fatalf("只改教室不应触发查重, 实际 err=%v", err)
	}
	// 前端编辑弹窗原样回传教师 + 课程名（值与库中相同）
	if _, err := svc.Update(db, admin, t1.ID, UpdateTaskParams{TeacherID: &teacherID, CourseName: pstr("编译原理")}); err != nil {
		t.Fatalf("键未变化不应触发查重, 实际 err=%v", err)
	}
	if tk := taskAfter(t, db, t1.ID); tk.Classroom == nil || *tk.Classroom != "3-303" {
		t.Fatalf("教室应已更新: %+v", tk.Classroom)
	}
}

// 历史遗留的重复任务（同教师+课程+时间两条并存）可通过「取消」其一化解：
// 取消使任务退出有效集合，去重预检查不应把这条路径拦住。
func TestTaskUpdateCancelDuplicateAllowed(t *testing.T) {
	db := taskBatchTestDB(t)
	collegeID := 11
	teacherID := seedTaskTeacher(t, db, collegeID)
	classTime := time.Date(2026, 4, 1, 8, 0, 0, 0, time.Local)
	mk := func() model.EvaluationTask {
		return model.EvaluationTask{
			TeacherID: teacherID, TeacherName: "任课教师", CourseName: "数据结构",
			ClassTime: model.LocalTimePtr(classTime), Status: model.TaskStatusPending,
		}
	}
	t1, t2 := mk(), mk()
	if err := db.Create(&t1).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	if err := db.Create(&t2).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	admin := &model.User{Model: model.Model{ID: 8002}, Username: "院管理员",
		Role: model.RoleCollegeAdmin, CollegeID: &collegeID, Status: 1}
	cancelled := model.TaskStatusCancelled
	if _, err := NewTask().Update(db, admin, t1.ID, UpdateTaskParams{Status: &cancelled}); err != nil {
		t.Fatalf("取消重复任务应放行, 实际 err=%v", err)
	}
	if tk := taskAfter(t, db, t1.ID); tk.Status != model.TaskStatusCancelled {
		t.Fatalf("任务应已取消, 实际状态 %d", tk.Status)
	}
}
