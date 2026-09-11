package handler

import (
	"strings"
	"testing"

	"backend-go/internal/service"
)

// renderEvaluationHTMLTestData 组装含恶意 payload 的导出数据。
// 每个外部字段使用唯一载荷，便于断言定位漏转义的字段
// （覆盖 issue #19 验收三处：元信息行 / 课表信息行 / 分组标题）。
func renderEvaluationHTMLTestData() map[string]interface{} {
	return map[string]interface{}{
		"id":             int64(1),
		"course_name":    `<script>course</script>`,
		"class_time":     `<script>class_time</script>`,
		"teacher_name":   `<script>teacher</script>`,
		"college_name":   `<script>college</script>`,
		"evaluator_name": `<script>evaluator</script>`,
		"evaluator_role_name": `<script>role</script>`,
		"submit_time":    `<script>submit</script>`,
		"total_score":    88.5,
		"max_total_score": 100.0,
		"schedule": map[string]interface{}{
			"class_time_text": `<script>sch_time</script>`,
			"classroom":       `<script>room</script>`,
			"class_info":      `<script>cls</script>`,
			"student_count":   `<script>count</script>`,
			"week_pattern":    `<script>week</script>`,
		},
		"dimension_groups": []service.ExportGroup{
			{
				Name:      `<script>group</script>`,
				Score:     90, MaxScore: 100,
				Dimensions: []service.ExportDimension{
					{Name: `<script>dim</script>`, DisplayValue: `<script>val</script>`, FieldType: "score", Score: 90, MaxScore: 100},
				},
			},
			{Name: "高等数学（上）", Score: 80, MaxScore: 100},
		},
	}
}

func TestRenderEvaluationHTMLEscapesExternalFields(t *testing.T) {
	out := renderEvaluationHTML(renderEvaluationHTMLTestData())

	t.Run("元信息行值已转义", func(t *testing.T) {
		// 每个外部字段的唯一载荷必须转义后出现、原始 <script> 不得出现
		for _, want := range []string{
			"course", "class_time", "teacher", "college", "evaluator", "role", "submit",
		} {
			if strings.Contains(out, "<script>"+want+"</script>") {
				t.Fatalf("元信息字段 %s 未转义", want)
			}
			if !strings.Contains(out, "&lt;script&gt;"+want+"&lt;/script&gt;") {
				t.Fatalf("元信息字段 %s 缺少转义实体", want)
			}
		}
	})

	t.Run("课表信息行值已转义", func(t *testing.T) {
		for _, want := range []string{
			"sch_time", "room", "cls", "count", "week",
		} {
			if strings.Contains(out, "<script>"+want+"</script>") {
				t.Fatalf("课表字段 %s 未转义", want)
			}
			if !strings.Contains(out, "&lt;script&gt;"+want+"&lt;/script&gt;") {
				t.Fatalf("课表字段 %s 缺少转义实体", want)
			}
		}
	})

	t.Run("分组标题与维度名已转义", func(t *testing.T) {
		for _, want := range []string{"group", "dim", "val"} {
			if strings.Contains(out, "<script>"+want+"</script>") {
				t.Fatalf("分组字段 %s 未转义", want)
			}
			if !strings.Contains(out, "&lt;script&gt;"+want+"&lt;/script&gt;") {
				t.Fatalf("分组字段 %s 缺少转义实体", want)
			}
		}
	})

	t.Run("注入载荷不再作为 HTML 执行", func(t *testing.T) {
		if strings.Contains(out, "<script>") {
			t.Fatalf("输出含未转义 <script> 标签: %s", out)
		}
	})

	t.Run("不含特殊字符的正常文本输出与现有一致", func(t *testing.T) {
		if !strings.Contains(out, "<h3>高等数学（上）") {
			t.Fatalf("正常文本应原样输出: %s", out)
		}
	})
}