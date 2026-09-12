package service

import (
	"strings"
	"testing"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// taskBatchTestDB 建批量创建任务所需表集合（含 LoadTeacherUser 的关联预加载）
func taskBatchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.EvaluationTask{}, &model.User{}, &model.UserRole{},
		&model.UserCollege{}, &model.UserRoom{}, &model.College{}, &model.ResearchRoom{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// seedTaskTeacher 建一个在职教师（被评对象）并返回其 ID
func seedTaskTeacher(t *testing.T, db *gorm.DB, collegeID int) int {
	t.Helper()
	u := model.User{
		UserNo: "T300", Username: "任课教师", Role: model.RoleTeacher,
		Status: 1, CollegeID: &collegeID,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("建教师失败: %v", err)
	}
	if err := db.Create(&model.UserRole{UserID: u.ID, Role: model.RoleTeacher}).Error; err != nil {
		t.Fatalf("建教师角色失败: %v", err)
	}
	return u.ID
}

func taskCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.EvaluationTask{}).Count(&n).Error; err != nil {
		t.Fatalf("统计任务数失败: %v", err)
	}
	return n
}

// TestBatchCreateTransactional 批量创建的两条关键约束：
//  1. 校验失败的项按 skipped 返回，不阻断本批其余项（保持既有语义）；
//  2. 落库阶段整体事务化——任一项因非重复原因写库失败，本批已写入的任务全部回滚，
//     不留下「接口报错、库里却有半批任务」的脏状态。
func TestBatchCreateTransactional(t *testing.T) {
	db := taskBatchTestDB(t)
	svc := NewTask()
	collegeID := 1
	teacherID := seedTaskTeacher(t, db, collegeID)
	// 系统管理员：不受学院范围限制
	caller := &model.User{Model: model.Model{ID: 99}, Username: "管理员",
		Role: model.RoleSystemAdmin, Status: 1, CollegeID: &collegeID}

	t.Run("正常批量全部落库", func(t *testing.T) {
		res, err := svc.BatchCreate(db, caller, []CreateTaskParams{
			{TeacherID: teacherID, CourseName: "高等数学"},
			{TeacherID: teacherID, CourseName: "线性代数"},
		})
		if err != nil {
			t.Fatalf("批量创建失败: %v", err)
		}
		if len(res.Created) != 2 || len(res.Skipped) != 0 {
			t.Fatalf("期望创建 2 条、跳过 0 条, 实际 %d/%d", len(res.Created), len(res.Skipped))
		}
		if n := taskCount(t, db); n != 2 {
			t.Fatalf("库中应有 2 条任务, 实际 %d", n)
		}
	})

	t.Run("批内重复只建一条", func(t *testing.T) {
		res, err := svc.BatchCreate(db, caller, []CreateTaskParams{
			{TeacherID: teacherID, CourseName: "概率论"},
			{TeacherID: teacherID, CourseName: "概率论"},
		})
		if err != nil {
			t.Fatalf("批量创建失败: %v", err)
		}
		if len(res.Created) != 1 || len(res.Skipped) != 1 {
			t.Fatalf("同批重复应创建 1 条、跳过 1 条, 实际 %d/%d", len(res.Created), len(res.Skipped))
		}
		if reason, _ := res.Skipped[0]["reason"].(string); !strings.Contains(reason, "本次提交") {
			t.Fatalf("批内重复应说明原因, 实际 %q", reason)
		}
	})

	t.Run("已存在课程跳过且不影响其它项", func(t *testing.T) {
		res, err := svc.BatchCreate(db, caller, []CreateTaskParams{
			{TeacherID: teacherID, CourseName: "高等数学"}, // 已存在
			{TeacherID: teacherID, CourseName: "离散数学"}, // 新课程
		})
		if err != nil {
			t.Fatalf("批量创建失败: %v", err)
		}
		if len(res.Created) != 1 || len(res.Skipped) != 1 {
			t.Fatalf("期望创建 1 条、跳过 1 条, 实际 %d/%d", len(res.Created), len(res.Skipped))
		}
		if reason, _ := res.Skipped[0]["reason"].(string); !strings.Contains(reason, "已存在相同课程") {
			t.Fatalf("重复项原因应提示已存在, 实际 %q", reason)
		}
	})

	t.Run("落库失败整批回滚", func(t *testing.T) {
		before := taskCount(t, db)
		// 用触发器模拟「插入过程发生非重复类错误」：命中该课程名即中止插入
		if err := db.Exec(`CREATE TRIGGER fail_task_insert BEFORE INSERT ON evaluation_task
			WHEN NEW.course_name = 'BOOM' BEGIN SELECT RAISE(ABORT, 'boom'); END;`).Error; err != nil {
			t.Fatalf("建触发器失败: %v", err)
		}
		defer func() { _ = db.Exec(`DROP TRIGGER fail_task_insert`).Error }()

		res, err := svc.BatchCreate(db, caller, []CreateTaskParams{
			{TeacherID: teacherID, CourseName: "数值分析"}, // 先写入成功
			{TeacherID: teacherID, CourseName: "BOOM"},  // 触发错误 → 整批回滚
		})
		if err == nil {
			t.Fatalf("落库失败应返回错误, 实际 created=%d skipped=%d", len(res.Created), len(res.Skipped))
		}
		if n := taskCount(t, db); n != before {
			t.Fatalf("回滚后任务数应保持 %d, 实际 %d（存在部分提交）", before, n)
		}
	})
}
