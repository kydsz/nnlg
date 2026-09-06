package service

import (
	"testing"
	"time"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// groupKey 归一化：对齐 MySQL 默认排序规则（大小写不敏感、尾随空格不敏感），
// 与任务创建去重口径一致（ADR-0001）
func TestGroupKeyNormalization(t *testing.T) {
	base := groupKey(7, "高等数学A", "2025-2026-1")
	if base != groupKey(7, "高等数学a", "2025-2026-1") {
		t.Fatal("课程名大小写异写应归入同一同课组")
	}
	if base != groupKey(7, "高等数学A  ", "2025-2026-1") {
		t.Fatal("课程名首尾空格应忽略")
	}
	if base == groupKey(8, "高等数学A", "2025-2026-1") {
		t.Fatal("不同教师不应同组")
	}
	if base == groupKey(7, "高等数学B", "2025-2026-1") {
		t.Fatal("不同课程不应同组")
	}
	if base == groupKey(7, "高等数学A", "2025-2026-2") {
		t.Fatal("不同学期不应同组")
	}
}

func courseEvalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.SemesterConfig{}, &model.EvaluationTask{}, &model.EvaluationRecord{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	cfgs := []model.SemesterConfig{
		{Semester: "2025-2026-1", StartDate: *model.LocalDatePtr(time.Date(2025, 9, 1, 0, 0, 0, 0, time.Local)), Weeks: 20},
		{Semester: "2025-2026-2", StartDate: *model.LocalDatePtr(time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)), Weeks: 20},
	}
	if err := db.Create(&cfgs).Error; err != nil {
		t.Fatalf("建学期配置失败: %v", err)
	}
	return db
}

func createCourseTask(t *testing.T, db *gorm.DB, teacherID int, course string, classTime time.Time) *model.EvaluationTask {
	t.Helper()
	task := model.EvaluationTask{
		TeacherID: teacherID, TeacherName: "被评教师", CourseName: course,
		ClassTime: model.LocalTimePtr(classTime), Status: model.TaskStatusPending,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	return &task
}

func createCourseRecord(t *testing.T, db *gorm.DB, taskID, evaluatorID int, role string) {
	t.Helper()
	rec := model.EvaluationRecord{TaskID: taskID, EvaluatorID: &evaluatorID, EvaluatorRole: role}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("建评教记录失败: %v", err)
	}
}

func TestCourseEvalStatsForTasks(t *testing.T) {
	sem1 := time.Date(2025, 9, 15, 10, 0, 0, 0, time.Local)
	sem2 := time.Date(2026, 3, 15, 10, 0, 0, 0, time.Local)

	t.Run("跨任务聚合按评教人去重", func(t *testing.T) {
		db := courseEvalTestDB(t)
		t1 := createCourseTask(t, db, 1, "高等数学", sem1)
		t2 := createCourseTask(t, db, 1, "高等数学", sem1.AddDate(0, 0, 1))
		createCourseRecord(t, db, t1.ID, 101, model.RoleTeacher)
		createCourseRecord(t, db, t2.ID, 101, model.RoleTeacher) // 同一评教人跨任务只计一人
		createCourseRecord(t, db, t2.ID, 102, model.RoleTeacher)

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*t1, *t2})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		for _, task := range []*model.EvaluationTask{t1, t2} {
			stat, ok := stats[task.ID]
			if !ok {
				t.Fatalf("任务 %d 应有汇总", task.ID)
			}
			if stat.EvaluatorCount != 2 {
				t.Fatalf("评教人数 = %d, 期望按评教人去重后 2", stat.EvaluatorCount)
			}
			if stat.SupervisorEvaluated {
				t.Fatal("仅教师评教不应判督导已评")
			}
		}
	})

	t.Run("督导角色判定", func(t *testing.T) {
		db := courseEvalTestDB(t)
		t1 := createCourseTask(t, db, 1, "高等数学", sem1)
		createCourseRecord(t, db, t1.ID, 101, model.RoleTeacher)
		createCourseRecord(t, db, t1.ID, 102, model.RoleSchoolSupervisor)

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*t1})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		if !stats[t1.ID].SupervisorEvaluated {
			t.Fatal("存在督导评教记录应判督导已评")
		}
	})

	t.Run("同课异写归入同一组", func(t *testing.T) {
		db := courseEvalTestDB(t)
		t1 := createCourseTask(t, db, 1, "高等数学A", sem1)
		t2 := createCourseTask(t, db, 1, "高等数学a", sem1.AddDate(0, 0, 1))
		createCourseRecord(t, db, t1.ID, 101, model.RoleTeacher)
		createCourseRecord(t, db, t2.ID, 102, model.RoleTeacher)

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*t1, *t2})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		for _, task := range []*model.EvaluationTask{t1, t2} {
			stat, ok := stats[task.ID]
			if !ok {
				t.Fatalf("任务 %d 应有汇总", task.ID)
			}
			if stat.EvaluatorCount != 2 {
				t.Fatalf("任务 %d 评教人数 = %d, 期望异写课程合并去重后 2", task.ID, stat.EvaluatorCount)
			}
		}
	})

	t.Run("学期周数未配置按20周兜底", func(t *testing.T) {
		db := courseEvalTestDB(t)
		// 结构体零值会被 gorm default:20 覆盖，显式 Update 模拟周数未配置（<=0）的真实数据
		if err := db.Model(&model.SemesterConfig{}).Where("semester = ?", "2025-2026-2").Update("weeks", 0).Error; err != nil {
			t.Fatalf("置零周数失败: %v", err)
		}
		t1 := createCourseTask(t, db, 1, "线性代数", sem2) // 学期配置 Weeks=0
		createCourseRecord(t, db, t1.ID, 101, model.RoleCollegeSupervisor)

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*t1})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		stat, ok := stats[t1.ID]
		if !ok {
			t.Fatal("Weeks<=0 的学期不应丢汇总")
		}
		if !stat.SupervisorEvaluated || stat.EvaluatorCount != 1 {
			t.Fatalf("汇总 = %+v, 期望督导已评且 1 人", stat)
		}
	})

	t.Run("无学期归属不在结果中", func(t *testing.T) {
		db := courseEvalTestDB(t)
		noTime := createCourseTask(t, db, 1, "高等数学", sem1)
		noTime.ClassTime = nil
		if err := db.Model(noTime).Update("class_time", nil).Error; err != nil {
			t.Fatalf("置空上课时间失败: %v", err)
		}
		outside := createCourseTask(t, db, 1, "高等数学", time.Date(2027, 1, 10, 10, 0, 0, 0, time.Local))

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*noTime, *outside})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		if len(stats) != 0 {
			t.Fatalf("无学期归属的任务不应有汇总, 得到 %v", stats)
		}
	})

	t.Run("已删除任务与记录不计入", func(t *testing.T) {
		db := courseEvalTestDB(t)
		t1 := createCourseTask(t, db, 1, "高等数学", sem1)
		deleted := createCourseTask(t, db, 1, "高等数学", sem1.AddDate(0, 0, 1))
		if err := db.Model(deleted).Update("is_deleted", true).Error; err != nil {
			t.Fatalf("删除任务失败: %v", err)
		}
		createCourseRecord(t, db, t1.ID, 101, model.RoleTeacher)
		deletedRec := model.EvaluationRecord{TaskID: t1.ID, EvaluatorID: &[]int{102}[0], EvaluatorRole: model.RoleTeacher, IsDeleted: true}
		if err := db.Create(&deletedRec).Error; err != nil {
			t.Fatalf("建已删除记录失败: %v", err)
		}

		stats, err := CourseEvalStatsForTasks(db, []model.EvaluationTask{*t1})
		if err != nil {
			t.Fatalf("汇总失败: %v", err)
		}
		if got := stats[t1.ID].EvaluatorCount; got != 1 {
			t.Fatalf("评教人数 = %d, 期望已删除数据不计入后 1", got)
		}
	})
}
