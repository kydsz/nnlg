package service

import (
	"testing"

	"backend-go/internal/model"
)

func TestSemesterDurationDays(t *testing.T) {
	cases := []struct {
		name  string
		weeks int
		want  int
	}{
		{"默认20周", 20, 140},
		{"未配置取默认", 0, 140},
		{"负数取默认", -5, 140},
		{"自定义16周", 16, 112},
		{"自定义25周", 25, 175},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := semesterDurationDays(c.weeks); got != c.want {
				t.Fatalf("semesterDurationDays(%d) = %d, 期望 %d", c.weeks, got, c.want)
			}
		})
	}
}

func TestParseStudentCount(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		wantN int
		want  bool
	}{
		{"取最后一个括号数字", "2404计算机班,2403计算机(125)", 125, true},
		{"无括号", "2404计算机班", 0, false},
		{"多个括号取最后", "a(10)b(20)", 20, true},
		{"括号内非数字", "a(x)", 0, false},
		{"全角括号不匹配", "（30）", 0, false},
		{"空字符串", "", 0, false},
		{"括号内含空格不匹配", "a( 30 )", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, ok := ParseStudentCount(c.in)
			if ok != c.want || n != c.wantN {
				t.Fatalf("ParseStudentCount(%q) = (%d, %v), 期望 (%d, %v)", c.in, n, ok, c.wantN, c.want)
			}
		})
	}
}

func TestScheduleDetailPayload(t *testing.T) {
	t.Run("取明细最大学生数", func(t *testing.T) {
		sc := &model.CourseSchedule{
			Model:       model.Model{ID: 1},
			TeacherID:   intPtr(7),
			TeacherName: "张三",
			Semester:    "2026-2027-1",
			Details: []model.CourseScheduleDetail{
				{Model: model.Model{ID: 1}, CourseName: "高等数学", ClassInfo: "2401(125)"},
				{Model: model.Model{ID: 2}, CourseName: "大学物理", ClassInfo: "2402(50)"},
				{Model: model.Model{ID: 3}, CourseName: "无人数课程", ClassInfo: "2403"},
			},
		}
		out := scheduleDetailPayload(sc)
		cnt, ok := out["student_count"].(*int)
		if !ok || cnt == nil || *cnt != 125 {
			t.Fatalf("student_count 应为 125, 得到 %v", out["student_count"])
		}
		details, ok := out["details"].([]map[string]interface{})
		if !ok || len(details) != 3 {
			t.Fatalf("details 应有 3 条, 得到 %d 条", len(details))
		}
		// 无人数明细的 student_count 应为 nil
		if got := details[2]["student_count"]; got != nil {
			t.Fatalf("无人数明细 student_count 应为 nil, 得到 %v", got)
		}
	})

	t.Run("无明细返回空壳", func(t *testing.T) {
		sc := &model.CourseSchedule{Model: model.Model{ID: 2}}
		out := scheduleDetailPayload(sc)
		// student_count 是 *int 类型的 typed-nil，需断言后判空
		cnt, ok := out["student_count"].(*int)
		if !ok || cnt != nil {
			t.Fatalf("无明细 student_count 应为 nil 的 *int, 得到 %v", out["student_count"])
		}
		if d, ok := out["details"].([]map[string]interface{}); !ok || len(d) != 0 {
			t.Fatalf("无明细 details 应为空, 得到 %v", out["details"])
		}
	})
}

func intPtr(v int) *int { return &v }
