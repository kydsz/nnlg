package jwxt

import (
	"testing"
	"time"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// saveSchedulesTestDB 建课表保存所需的最小表集合
func saveSchedulesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&model.CourseSchedule{}, &model.CourseScheduleDetail{}, &model.CourseScheduleVersion{},
		&model.User{},
	); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

func oneSchedule(teacherName string, courses []CourseInfo) []TeacherSchedule {
	return []TeacherSchedule{{TeacherName: teacherName, Courses: courses}}
}

func TestSaveSchedulesPropagatesErrors(t *testing.T) {
	db := saveSchedulesTestDB(t)

	t.Run("正常保存返回统计", func(t *testing.T) {
		stats, err := SaveSchedules(db, oneSchedule("张老师", []CourseInfo{
			{CourseName: "高等数学", ClassInfo: "计科1班", WeekPattern: "1-16周", WeekDay: 1, Section: "0102", Classroom: "A101"},
		}), "2026-2027-1", time.Now(), 0)
		if err != nil {
			t.Fatalf("正常保存不应报错: %v", err)
		}
		if stats["total_teachers"].(int) != 1 || stats["total_courses"].(int) != 1 {
			t.Fatalf("stats 不符: %+v", stats)
		}
	})

	t.Run("DB 写入失败时返回错误", func(t *testing.T) {
		// 删除 course_schedule 表模拟写路径失败：任何 db 操作报错都应向上传播
		if err := db.Migrator().DropTable(&model.CourseSchedule{}); err != nil {
			t.Fatalf("删表失败: %v", err)
		}
		_, err := SaveSchedules(db, oneSchedule("张老师", []CourseInfo{
			{CourseName: "高等数学", WeekDay: 1, Section: "0102"},
		}), "2026-2027-1", time.Now(), 0)
		if err == nil {
			t.Fatal("DB 表缺失时 SaveSchedules 应返回错误，而非静默成功")
		}
	})
}

// 版本切换必须原子：先清 is_current 后建新版本，若新版本创建失败，
// 旧版本不得被置为 is_current=false（回滚后保持原状，无孤儿数据）。
func TestSaveSchedulesTransactionRollback(t *testing.T) {
	db := saveSchedulesTestDB(t)

	// 先存一条当前版本
	if _, err := SaveSchedules(db, oneSchedule("张老师", []CourseInfo{
		{CourseName: "高等数学", WeekDay: 1, Section: "0102"},
	}), "2026-2027-1", time.Now(), 0); err != nil {
		t.Fatalf("初始保存失败: %v", err)
	}

	// 制造第二次保存失败：删除 course_schedule_detail 表（createScheduleWithDetails 写详情失败）
	if err := db.Migrator().DropTable(&model.CourseScheduleDetail{}); err != nil {
		t.Fatalf("删表失败: %v", err)
	}

	_, err := SaveSchedules(db, oneSchedule("张老师", []CourseInfo{
		{CourseName: "大学物理", WeekDay: 1, Section: "0102"},
	}), "2026-2027-1", time.Now(), 0)
	if err == nil {
		t.Fatal("详情表缺失时保存应返回错误")
	}

	// 旧版本应保持 is_current=1（事务回滚），不出现"无当前课表"
	var cur model.CourseSchedule
	if derr := db.Where("semester = ? AND is_current = 1", "2026-2027-1").First(&cur).Error; derr != nil {
		t.Fatalf("回滚后应保留 is_current=1 的旧版本, 查询失败: %v", derr)
	}
	if cur.TeacherName == "" {
		t.Fatalf("回滚后旧版本数据不符: %+v", cur)
	}
	// 无孤儿新版本（半成品）
	var stale int64
	db.Model(&model.CourseSchedule{}).Where("semester = ?", "2026-2027-1").Count(&stale)
	if stale != 1 {
		t.Fatalf("回滚后应只有 1 条（旧版本）, 实际 %d 条", stale)
	}
}

// 重复调用不得产生重复版本号（MAX+1 并发竞态收敛为唯一约束/行锁兜底）
func TestSaveSchedulesVersionMonotonic(t *testing.T) {
	db := saveSchedulesTestDB(t)

	for i := 0; i < 3; i++ {
		stats, err := SaveSchedules(db, oneSchedule("张老师", []CourseInfo{
			{CourseName: "课程", WeekDay: 1, Section: "0102"},
		}), "2026-2027-1", time.Now(), 0)
		if err != nil {
			t.Fatalf("第 %d 次保存失败: %v", i+1, err)
		}
		// 第 1 次新增、后续同内容为 unchanged；版本历史始终递增
		if i == 0 && stats["new_teachers"].(int) != 1 {
			t.Fatalf("第 1 次应为新增: %+v", stats)
		}
		if i > 0 && stats["unchanged_teachers"].(int) != 1 {
			t.Fatalf("第 %d 次应为 unchanged: %+v", i+1, stats)
		}
	}
	// 全部版本号各不重复
	var versions []int
	db.Model(&model.CourseScheduleVersion{}).Where("semester = ?", "2026-2027-1").
		Pluck("version", &versions)
	seen := map[int]bool{}
	for _, v := range versions {
		if seen[v] {
			t.Fatalf("版本号 %d 重复", v)
		}
		seen[v] = true
	}
	if len(versions) != 3 {
		t.Fatalf("应有 3 个版本, 实际 %d", len(versions))
	}
}
