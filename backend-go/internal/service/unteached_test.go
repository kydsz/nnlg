package service

import (
	"testing"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// unteachedTestDB 未经听课统计的最小数据：文理学院教师 + 一条学期配置。
// 复用 teacherScopeTestDB 的学院 5/6 与教师样例。
func unteachedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := teacherScopeTestDB(t)
	// teacherScopeTestDB 未建 semester_config / evaluation_task 表，这里补迁移
	if err := db.AutoMigrate(&model.SemesterConfig{}, &model.EvaluationTask{}); err != nil {
		t.Fatalf("补建表失败: %v", err)
	}
	if err := db.Create(&model.SemesterConfig{
		Semester:  "2026-2027-1",
		StartDate: model.LocalDate(time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)),
		Weeks:     20, IsCurrent: true,
	}).Error; err != nil {
		t.Fatalf("灌入学期配置失败: %v", err)
	}
	return db
}

func TestUnteachedTeachersPairwiseDates(t *testing.T) {
	db := unteachedTestDB(t)
	svc := &Stats{}

	t.Run("只传 start_date 返回明确错误而非 panic", func(t *testing.T) {
		start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
		_, _, err := svc.UnteachedTeachers(db, &model.User{}, nil, &start, nil, 1, 50)
		if err == nil {
			t.Fatal("单端传参应被拒绝")
		}
	})

	t.Run("只传 end_date 返回明确错误而非 panic", func(t *testing.T) {
		end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
		_, _, err := svc.UnteachedTeachers(db, &model.User{}, nil, nil, &end, 1, 50)
		if err == nil {
			t.Fatal("单端传参应被拒绝")
		}
	})

	t.Run("两端都不传走当前学期兜底", func(t *testing.T) {
		_, _, err := svc.UnteachedTeachers(db, &model.User{}, nil, nil, nil, 1, 50)
		if err != nil {
			t.Fatalf("双空走当前学期不应报错: %v", err)
		}
	})

	t.Run("成对传参与现状一致", func(t *testing.T) {
		start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
		end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local)
		_, _, err := svc.UnteachedTeachers(db, &model.User{}, nil, &start, &end, 1, 50)
		if err != nil {
			t.Fatalf("成对传参不应报错: %v", err)
		}
	})
}
