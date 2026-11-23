package service

import (
	"testing"

	"backend-go/internal/model"
)

// 外聘教师建任务：任务落双学院快照，授课学院范围校验生效。
func TestTaskCreateUsesTeachingCollege(t *testing.T) {
	db := taskBatchTestDB(t)

	xd := model.College{Code: "XD", Name: "信息与电气工程学院", Status: 1}
	tm := model.College{Code: "TM", Name: "土木与工程学院", Status: 1}
	wl := model.College{Code: "WL", Name: "文理学院", Status: 1}
	for _, c := range []*model.College{&xd, &tm, &wl} {
		if err := db.Create(c).Error; err != nil {
			t.Fatal(err)
		}
	}
	teacher := model.User{UserNo: "T920", Username: "陈守学", Role: model.RoleTeacher, CollegeID: &tm.ID, Status: 1}
	if err := db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.TeacherTeachingCollege{
		UserID: teacher.ID, CollegeID: xd.ID, Source: "manual",
	}).Error; err != nil {
		t.Fatal(err)
	}

	xdAdmin := &model.User{Model: model.Model{ID: 9201}, Role: model.RoleCollegeAdmin, CollegeID: &xd.ID}
	wlAdmin := &model.User{Model: model.Model{ID: 9202}, Role: model.RoleCollegeAdmin, CollegeID: &wl.ID}
	sysAdmin := &model.User{Model: model.Model{ID: 9203}, Role: model.RoleSystemAdmin}

	task, err := NewTask().Create(db, xdAdmin, CreateTaskParams{
		TeacherID: teacher.ID, CourseName: "模拟电子技术",
	})
	if err != nil {
		t.Fatalf("信电管理员应为外聘教师建任务: %v", err)
	}
	if task.TeacherCollegeID == nil || *task.TeacherCollegeID != tm.ID {
		t.Fatalf("人事学院快照应为土木, 得到 %v", task.TeacherCollegeID)
	}
	if task.CourseCollegeID == nil || *task.CourseCollegeID != xd.ID {
		t.Fatalf("授课学院快照应为信电, 得到 %v", task.CourseCollegeID)
	}

	if _, err := NewTask().Create(db, wlAdmin, CreateTaskParams{
		TeacherID: teacher.ID, CourseName: "无关学院任务",
	}); err == nil {
		t.Fatal("文理学院管理员不应能为该教师建任务")
	}

	if _, err := NewTask().Create(db, xdAdmin, CreateTaskParams{
		TeacherID: teacher.ID, CourseName: "显式土木", CollegeID: &tm.ID,
	}); err == nil {
		t.Fatal("信电管理员显式指定土木应被拒绝")
	}

	// 编辑：信电管理员不能把任务改到土木；系统管理员可以
	if _, err := NewTask().Update(db, xdAdmin, task.ID, UpdateTaskParams{CollegeID: &tm.ID}); err == nil {
		t.Fatal("信电管理员不应能把任务改到土木")
	}
	updated, err := NewTask().Update(db, sysAdmin, task.ID, UpdateTaskParams{CollegeID: &tm.ID})
	if err != nil {
		t.Fatalf("系统管理员改授课学院失败: %v", err)
	}
	if updated.CourseCollegeID == nil || *updated.CourseCollegeID != tm.ID {
		t.Fatalf("编辑后授课学院应为土木, 得到 %v", updated.CourseCollegeID)
	}

	// 新任务不指定学院时，信电教师个人创建仍自动落信电（teacher 角色走同学院校验）
	xdTeacher := &model.User{Model: model.Model{ID: 9204}, Role: model.RoleTeacher, CollegeID: &xd.ID}
	t2, err := NewTask().Create(db, xdTeacher, CreateTaskParams{
		TeacherID: teacher.ID, CourseName: "自动解析",
	})
	if err != nil {
		t.Fatalf("信电教师应可为授课本院的外聘教师建任务: %v", err)
	}
	if t2.CourseCollegeID == nil || *t2.CourseCollegeID != xd.ID {
		t.Fatalf("信电教师建任务应自动落信电, 得到 %v", t2.CourseCollegeID)
	}
}
