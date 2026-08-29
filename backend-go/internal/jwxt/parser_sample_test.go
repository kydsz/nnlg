package jwxt

import (
	"os"
	"testing"
)

// TestParseScheduleHTMLSample 用仓库样本文件验证解析器
func TestParseScheduleHTMLSample(t *testing.T) {
	data, err := os.ReadFile("../../../backend/app/crawl/teacher_schedule.html")
	if err != nil {
		t.Skip("样本文件不存在，跳过")
	}
	schedules, stats, err := ParseScheduleHTML(string(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if stats.TotalTeachers == 0 {
		t.Fatal("解析结果为空")
	}
	if stats.TotalCourses == 0 {
		t.Fatal("课程数为 0")
	}
	t.Logf("教师数: %d, 课程数: %d", stats.TotalTeachers, stats.TotalCourses)
	first := schedules[0]
	t.Logf("首位教师: %s, 课程数: %d", first.TeacherName, len(first.Courses))
	if len(first.Courses) > 0 {
		c := first.Courses[0]
		t.Logf("示例课程: %+v", c)
		if c.CourseName == "" || c.WeekDay < 1 || c.WeekDay > 7 || c.Section == "" {
			t.Fatalf("课程字段不完整: %+v", c)
		}
	}
}
