package handler

import (
	"os"
	"strings"
	"testing"

	"backend-go/internal/service"
)

// 回归：课表信息行的值必须解引用指针——fmt.Sprint(*int) 会打出
// 0x... 内存地址（线上症状「应到人数0x2b6082351468人」）。
func TestBuildScheduleRows(t *testing.T) {
	cnt := 125
	room := "1-101"
	sch := map[string]interface{}{
		"class_time_text": "周六 第5-6节",
		"classroom":       &room,
		"class_info":      "2401(125)",
		"student_count":   &cnt,
		"week_pattern":    nil,
	}
	rows := buildScheduleRows(sch)
	want := []struct{ label, value string }{
		{"上课时间", "周六 第5-6节"},
		{"教室", "1-101"},
		{"班级", "2401(125)"},
		{"应到人数", "125人"},
		{"周次", "-"},
	}
	if len(rows) != len(want) {
		t.Fatalf("应生成 %d 行, 得到 %d 行", len(want), len(rows))
	}
	for i, w := range want {
		if rows[i].Label != w.label || rows[i].Value != w.value {
			t.Fatalf("第 %d 行 = %q/%q, 期望 %q/%q", i, rows[i].Label, rows[i].Value, w.label, w.value)
		}
	}

	// int 而非指针也必须正常
	rows2 := buildScheduleRows(map[string]interface{}{"student_count": 88})
	if rows2[3].Value != "88人" {
		t.Fatalf("非指针 student_count 应为 88人, 得到 %q", rows2[3].Value)
	}
	// nil 指针 → "-"
	nilCnt := (*int)(nil)
	rows3 := buildScheduleRows(map[string]interface{}{"student_count": nilCnt})
	if rows3[3].Value != "-" {
		t.Fatalf("nil 指针 student_count 应为 -, 得到 %q", rows3[3].Value)
	}
}

// 端到端：指针学生数走完整 renderEvaluationPDF 路径不报错且可出图（PDF_DUMP 落盘目检）
func TestRenderEvaluationPDFSchedulePointerValue(t *testing.T) {
	cnt := 125
	data := map[string]interface{}{
		"course_name": "机械原理", "teacher_name": "马英",
		"college_name": "信息与电气工程学院",
		"schedule": map[string]interface{}{
			"class_time_text": "周六 第5-6节",
			"classroom":       "1-101",
			"class_info":      "2401(125)",
			"student_count":   &cnt,
			"week_pattern":    "第1-16周",
		},
		"dimension_groups": []service.ExportGroup{},
	}
	out, err := renderEvaluationPDF(data, "uploads")
	if err != nil {
		if strings.Contains(err.Error(), "中文字体") {
			t.Skipf("本机无中文字体，跳过: %v", err)
		}
		t.Fatalf("渲染失败: %v", err)
	}
	if p := os.Getenv("PDF_DUMP"); p != "" {
		_ = os.WriteFile(p, out, 0o644)
	}
}
