package handler

import (
	"encoding/json"
	"testing"
)

func TestCellValue(t *testing.T) {
	str := "中文教室"
	f := 88.5
	nilStr := (*string)(nil)
	pp := &str // 二级指针

	cases := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{"nil", nil, ""},
		{"int", 42, 42},
		{"float64", 88.5, 88.5},
		{"bool", true, true},
		{"数字字符串", "125", int64(125)},
		{"小数数字字符串", "3.14", float64(3.14)},
		{"普通字符串", "abc", "abc"},
		{"空字符串", "", ""},
		{"字符串指针解引用", &str, "中文教室"},
		{"float指针解引用", &f, 88.5},
		{"二级指针解引用", &pp, "中文教室"},
		{"nil指针返回空串", nilStr, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cellValue(c.in); got != c.want {
				t.Fatalf("cellValue(%v) = %v (%T), 期望 %v (%T)", c.in, got, got, c.want, c.want)
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	str := "中文"
	cases := []struct {
		name string
		in   interface{}
		want int
	}{
		{"nil", nil, 0},
		{"纯中文2字", "中文", 4},
		{"纯英文3字", "abc", 3},
		{"中英混排", "a中", 3},
		{"数字浮点", 88.5, 4},
		{"字符串指针", &str, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := displayWidth(c.in); got != c.want {
				t.Fatalf("displayWidth(%v) = %d, 期望 %d", c.in, got, c.want)
			}
		})
	}
}

func TestExportDateSuffix(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		want       string
	}{
		{"起止都有", "2026-09-01", "2026-12-31", "_20260901-20261231"},
		{"只有开始", "2026-09-01", "", "_20260901起"},
		{"只有结束", "", "2026-12-31", "_截至20261231"},
		{"都为空", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exportDateSuffix(c.start, c.end); got != c.want {
				t.Fatalf("exportDateSuffix(%q, %q) = %q, 期望 %q", c.start, c.end, got, c.want)
			}
		})
	}
}

func TestExportDateLabel(t *testing.T) {
	cases := []struct {
		name       string
		start, end string
		want       string
	}{
		{"起止都有", "2026-09-01", "2026-12-31", "2026-09-01 至 2026-12-31"},
		{"只有开始", "2026-09-01", "", "2026-09-01 起"},
		{"只有结束", "", "2026-12-31", "截至 2026-12-31"},
		{"都为空", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exportDateLabel(c.start, c.end); got != c.want {
				t.Fatalf("exportDateLabel(%q, %q) = %q, 期望 %q", c.start, c.end, got, c.want)
			}
		})
	}
}

func TestColsByFields(t *testing.T) {
	mapping := map[string]string{"teacher_name": "教师姓名", "total_tasks": "总任务数"}
	def := []string{"teacher_name"}

	t.Run("空字段用默认列", func(t *testing.T) {
		cols := colsByFields(nil, mapping, def)
		if len(cols) != 1 || cols[0].Key != "teacher_name" || cols[0].Label != "教师姓名" {
			t.Fatalf("默认列错误: %+v", cols)
		}
	})
	t.Run("已知字段映射中文", func(t *testing.T) {
		cols := colsByFields([]string{"teacher_name", "total_tasks"}, mapping, def)
		if len(cols) != 2 || cols[1].Label != "总任务数" {
			t.Fatalf("字段映射错误: %+v", cols)
		}
	})
	t.Run("未知字段取自身", func(t *testing.T) {
		cols := colsByFields([]string{"custom_field"}, mapping, def)
		if len(cols) != 1 || cols[0].Label != "custom_field" {
			t.Fatalf("未知字段标题错误: %+v", cols)
		}
	})
}

func TestSplitRoles(t *testing.T) {
	if got := splitRoles(""); got != nil {
		t.Fatalf("空字符串应返回 nil, 得到 %v", got)
	}
	got := splitRoles("supervisor,teacher")
	if len(got) != 2 || got[0] != "supervisor" || got[1] != "teacher" {
		t.Fatalf("角色拆分错误: %v", got)
	}
	got = splitRoles("  supervisor ,  teacher  ")
	if len(got) != 2 || got[0] != "supervisor" || got[1] != "teacher" {
		t.Fatalf("含空格角色拆分错误: %v", got)
	}
}

func TestIntSliceOf(t *testing.T) {
	got := intSliceOf([]string{"1", " 2 ", "abc", "3"})
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("intSliceOf 结果错误: %v", got)
	}
}

func TestStrListUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"逗号分隔字符串", `"a,b"`, []string{"a", "b"}},
		{"JSON数组", `["x","y"]`, []string{"x", "y"}},
		{"null", `null`, nil},
		{"空字符串", `""`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sl strList
			if err := json.Unmarshal([]byte(c.in), &sl); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if len(sl) != len(c.want) {
				t.Fatalf("strList = %v, 期望 %v", sl, c.want)
			}
			for i := range sl {
				if sl[i] != c.want[i] {
					t.Fatalf("strList = %v, 期望 %v", sl, c.want)
				}
			}
		})
	}
}
