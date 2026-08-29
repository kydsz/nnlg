package pdfgen

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestExtractFirstFont(t *testing.T) {
	// 非 TTC 数据原样返回
	plain := []byte{0x00, 0x01, 0x00, 0x00, 0xff, 0xff}
	if got := extractFirstFont(plain); !bytes.Equal(got, plain) {
		t.Fatal("非 TTC 数据应原样返回")
	}

	// 构造最小合法 TTC：ttcf 头 + face 目录(1 表) + head 表数据
	// 布局：[0:16] ttcf 头（numFonts=1, offset=20）[20:32] face 头 [32:48] 目录项 [48:52] 表数据
	ttc := make([]byte, 52)
	copy(ttc, "ttcf")
	ttc[11] = 1  // numFonts = 1
	ttc[15] = 20 // OffsetTable[0]
	face := ttc[20:]
	face[1] = 1                                        // sfnt version 0x00010000
	binary.BigEndian.PutUint16(face[4:6], 1)           // numTables = 1
	copy(ttc[32:36], "head")                           // 表 tag
	binary.BigEndian.PutUint32(ttc[36:40], 0x12345678) // checksum
	binary.BigEndian.PutUint32(ttc[40:44], 48)         // 原始 offset（相对 TTC 文件）
	binary.BigEndian.PutUint32(ttc[44:48], 4)          // length
	copy(ttc[48:52], []byte{0x5F, 0x0F, 0x3C, 0xF5})   // head magic

	got := extractFirstFont(ttc)
	if len(got) != 32 { // 12 头 + 16 目录 + 4 数据
		t.Fatalf("重建长度错误: %d", len(got))
	}
	if binary.BigEndian.Uint32(got[0:4]) != 0x00010000 {
		t.Fatal("sfnt version 错误")
	}
	if string(got[12:16]) != "head" {
		t.Fatal("表目录 tag 错误")
	}
	if binary.BigEndian.Uint32(got[20:24]) != 28 { // 新 offset = 12+16
		t.Fatal("表新 offset 错误")
	}
	if !bytes.Equal(got[28:32], []byte{0x5F, 0x0F, 0x3C, 0xF5}) {
		t.Fatal("表数据错误")
	}
}

// TestRenderPDFs 本机存在中文字体时做真实渲染冒烟（CI 无字体时跳过）
func TestRenderPDFs(t *testing.T) {
	if _, err := loadChineseFont(); err != nil {
		t.Skipf("本机无中文字体，跳过渲染冒烟: %v", err)
	}

	t.Run("evaluation", func(t *testing.T) {
		d := EvalDetail{
			CourseName: "计算机组装与维护技术", TeacherName: "马英", CollegeName: "信息与电气工程学院",
			EvaluatorName: "系统管理员", EvaluatorRole: "系统管理员", Submit: "2026-08-29 02:32",
			TotalScore: 100, MaxTotalScore: 100,
			Groups: []EvalDetailGroup{
				{Name: "考勤信息", Dimensions: []EvalDetailDim{
					{Name: "实到人数", FieldType: "text", Display: "111"},
					{Name: "课堂考勤", FieldType: "text", Display: "考勤"},
					{Name: "应到人数", FieldType: "text", Display: "111"},
				}},
				{Name: "评价选项", Dimensions: []EvalDetailDim{
					{Name: "课堂秩序", FieldType: "single_choice", Display: "好"},
					{Name: "教学文档", FieldType: "text", Display: "教学大纲"},
					{Name: "教学进度", FieldType: "single_choice", Display: "符合"},
				}},
				{Name: "教学态度", Score: 20, MaxScore: 20, Dimensions: []EvalDetailDim{
					{Name: "严格课堂管理，检查学生出勤情况", FieldType: "score", Score: 5, MaxScore: 5},
					{Name: "仪态大方、精神饱满、富有感染力", FieldType: "score", Score: 10, MaxScore: 10},
					{Name: "按时上、下课，无迟到、拖堂等现象", FieldType: "score", Score: 5, MaxScore: 5},
				}},
				{Name: "教学内容", Score: 50, MaxScore: 50, Dimensions: []EvalDetailDim{
					{Name: "熟悉教学内容，节奏流畅，重点难点突出", FieldType: "score", Score: 10, MaxScore: 10},
					{Name: "课程思政融入明显", FieldType: "score", Score: 10, MaxScore: 10},
					{Name: "教学理念/设计/改革具有高阶性、创新性、挑战度", FieldType: "score", Score: 10, MaxScore: 10},
					{Name: "教学目标明确，符合教学大纲", FieldType: "score", Score: 10, MaxScore: 10},
					{Name: "理论联系实际，注重能力培养", FieldType: "score", Score: 10, MaxScore: 10},
				}},
				{Name: "教学方法与手段", Score: 15, MaxScore: 15, Dimensions: []EvalDetailDim{
					{Name: "板书合理、课件规范", FieldType: "score", Score: 5, MaxScore: 5},
					{Name: "启发式/讨论式教学，注重互动", FieldType: "score", Score: 10, MaxScore: 10},
				}},
				{Name: "教学效果", Score: 15, MaxScore: 15, Dimensions: []EvalDetailDim{
					{Name: "课堂气氛活跃，听课率高", FieldType: "score", Score: 10, MaxScore: 10},
					{Name: "能启发学生创新，收获大", FieldType: "score", Score: 5, MaxScore: 5},
				}},
				{Name: "评语及建议", Dimensions: []EvalDetailDim{
					{Name: "教学优点", FieldType: "text", Display: "1"},
					{Name: "存在问题", FieldType: "text", Display: "1"},
					{Name: "改进建议", FieldType: "text", Display: "1"},
				}},
				{Name: "听课情况", Dimensions: []EvalDetailDim{
					{Name: "出勤率", FieldType: "text", Display: "1"},
					{Name: "听课内容", FieldType: "text", Display: "课堂氛围活跃，讲解清晰透彻，学生参与度高，整体教学效果良好，值得推广学习"},
				}},
			},
		}
		out, err := RenderEvaluationPDF(d, "uploads")
		if err != nil {
			t.Fatalf("生成评教 PDF 失败: %v", err)
		}
		assertPDF(t, out)
	})

	t.Run("table", func(t *testing.T) {
		rows := []map[string]interface{}{
			{"teacher_name": "张三", "user_no": 1001, "total_tasks": 12, "average_score": 88.5},
			{"teacher_name": "王五", "user_no": 1002, "total_tasks": 8, "average_score": 92},
		}
		out, err := RenderTablePDF("教师评教统计报表  |  时间段: 2026-01-01 至 2026-06-30", []TableCol{
			{Key: "teacher_name", Label: "教师姓名"},
			{Key: "user_no", Label: "工号"},
			{Key: "total_tasks", Label: "总任务数"},
			{Key: "average_score", Label: "平均分"},
		}, rows)
		if err != nil {
			t.Fatalf("生成表格 PDF 失败: %v", err)
		}
		assertPDF(t, out)
	})
}

func assertPDF(t *testing.T, out []byte) {
	t.Helper()
	if len(out) == 0 || !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("输出不是合法 PDF 文件")
	}
	if !bytes.Contains(out, []byte("%%EOF")) {
		t.Fatalf("PDF 缺少结束标记")
	}
	if os.Getenv("PDF_DUMP") != "" {
		_ = os.WriteFile(os.Getenv("PDF_DUMP"), out, 0o644)
	}
}
