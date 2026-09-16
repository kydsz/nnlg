package pdfgen

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 本文件是「导出 PDF 文字重叠」bug 的反馈回路与回归测试：
// 渲染评教详情 → 解析 PDF 内容流中的文本绘制坐标 →
// 断言同 x 起点的两段文本基线间距不小于字高的 85%（叠字即红）。

// textDraw 内容流中一次文本绘制（坐标为 pt，y 自页底起；width 为精确渲染宽度）
type textDraw struct {
	x, y, size float64
	width      float64 // Σ W[gid]/1000×size，未知字形按 1em 估
	page       int
}

// vSeg 内容流中一条竖直线段（坐标为 pt，y 自页底起）
type vSeg struct {
	x, yMin, yMax float64
	page          int
}

var (
	reStream   = regexp.MustCompile(`(?s)stream\r?\n(.*?)endstream`)
	reTextDraw = regexp.MustCompile(`(?s)BT\n([0-9.-]+) ([0-9.-]+) TD\n/F\d+ ([0-9.]+) Tf(.*?)ET`)
	reGlyphHex = regexp.MustCompile(`[0-9A-F]{4}`)
	reVLine    = regexp.MustCompile(`([0-9.]+) ([0-9.-]+) m ([0-9.]+) ([0-9.-]+) l [Ss]`)
	reWEntry   = regexp.MustCompile(`(\d+)\[(-?\d+)\]`)
	reWArray   = regexp.MustCompile(`/W \[((?:\d+\[[\d-]+\])+)\]`)
)

// parseGlyphWidths 从原始 PDF 解析子集字体 /W 宽度表（GID → 千分之 em 宽）
func parseGlyphWidths(pdf []byte) map[int]float64 {
	w := map[int]float64{}
	if m := reWArray.FindSubmatch(pdf); m != nil {
		for _, e := range reWEntry.FindAllSubmatch(m[1], -1) {
			gid, _ := strconv.Atoi(string(e[1]))
			uw, _ := strconv.ParseFloat(string(e[2]), 64)
			w[gid] = uw / 1000.0
		}
	}
	return w
}

// parsePDFContent 解压所有页内容流，提取文本绘制与竖直线段
func parsePDFContent(t *testing.T, pdf []byte) ([]textDraw, []vSeg) {
	t.Helper()
	widths := parseGlyphWidths(pdf)
	var draws []textDraw
	var segs []vSeg
	for page, m := range reStream.FindAllSubmatch(pdf, -1) {
		raw := m[1]
		data := raw
		if zr, zerr := zlib.NewReader(bytes.NewReader(raw)); zerr == nil {
			if dec, derr := io.ReadAll(zr); derr == nil {
				data = dec
			}
		}
		for _, tm := range reTextDraw.FindAllSubmatch(data, -1) {
			x, _ := strconv.ParseFloat(string(tm[1]), 64)
			y, _ := strconv.ParseFloat(string(tm[2]), 64)
			size, _ := strconv.ParseFloat(string(tm[3]), 64)
			w := 0.0
			for _, g := range reGlyphHex.FindAll(tm[4], -1) {
				gid, _ := strconv.ParseInt(string(g), 16, 32)
				if u, ok := widths[int(gid)]; ok {
					w += u
				} else {
					w += 1.0 // 未知字形按全宽估
				}
			}
			draws = append(draws, textDraw{x: x, y: y, size: size, width: w * size, page: page})
		}
		for _, lm := range reVLine.FindAllSubmatch(data, -1) {
			x1, _ := strconv.ParseFloat(string(lm[1]), 64)
			y1, _ := strconv.ParseFloat(string(lm[2]), 64)
			x2, _ := strconv.ParseFloat(string(lm[3]), 64)
			y2, _ := strconv.ParseFloat(string(lm[4]), 64)
			if abs(x1-x2) > 0.5 {
				continue // 只关注竖线
			}
			// 排除左右页框：右对齐文本允许贴齐右框线
			const leftPt, rightPt = 28.35, 566.93
			if x1 < leftPt+1 || x1 > rightPt-1 {
				continue
			}
			segs = append(segs, vSeg{x: x1, yMin: min(y1, y2), yMax: max(y1, y2), page: page})
		}
	}
	if len(draws) == 0 {
		t.Fatal("内容流中未解析到任何文本绘制，解析器与 gopdf 输出格式不匹配")
	}
	return draws, segs
}

// assertNoLayoutDefects 布局缺陷总断言：叠字 + 文字跨越竖线（标签溢出单元格）
func assertNoLayoutDefects(t *testing.T, pdf []byte) {
	t.Helper()
	draws, segs := parsePDFContent(t, pdf)
	assertNoOverlappingText(t, draws)
	assertNoTextCrossingLines(t, draws, segs)
}

// assertNoOverlappingText 断言同一页上同 x 起点（±1pt）的两次文本绘制
// 基线垂直距离 ≥ 0.85×字号（即不叠字）。折行文本同格共 x 起点，
// 行距 4.4mm≈12.5pt（11pt 字），阈值 0.85×11≈9.35pt 不会误报；
// 叠字版本行距减半为 2.2mm≈6.2pt，必红。
func assertNoOverlappingText(t *testing.T, draws []textDraw) {
	t.Helper()
	for i := 0; i < len(draws); i++ {
		for j := i + 1; j < len(draws); j++ {
			a, b := draws[i], draws[j]
			if a.page != b.page || abs(a.x-b.x) > 1.0 {
				continue
			}
			minGap := 0.85 * max(a.size, b.size)
			if d := abs(a.y - b.y); d < minGap {
				t.Fatalf("检测到文字重叠: page=%d x=%.2fpt 两段文本基线相距 %.2fpt < 阈值 %.2fpt (字号 %.1f/%.1f)",
					a.page, a.x, d, minGap, a.size, b.size)
			}
		}
	}
}

// assertNoTextCrossingLines 断言没有文本跨越内容区内的竖直分隔线
//（线上症状：超长维度名溢出 46mm 标签列、画过名称/值分隔线）。
// 文本宽度由 PDF 内嵌 /W 宽度表精确计算；页框线不参与断言（右对齐文本允许贴齐右框）。
func assertNoTextCrossingLines(t *testing.T, draws []textDraw, segs []vSeg) {
	t.Helper()
	for _, d := range draws {
		for _, sg := range segs {
			if d.page != sg.page {
				continue
			}
			if sg.x > d.x+0.5 && sg.x < d.x+d.width-0.5 && d.y > sg.yMin && d.y < sg.yMax {
				t.Fatalf("文本跨越竖直分隔线: page=%d 文本 x=%.2fpt 宽 %.2fpt 跨过 x=%.2fpt (y=%.2fpt, 字号 %.1f)",
					d.page, d.x, d.width, sg.x, d.y, d.size)
			}
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// renderDetail 渲染并落盘（PDF_DUMP 指定输出路径，便于栅格化目检）。
// 与包内冒烟测试一致：无中文字体的环境（CI）跳过——重叠检测依赖真实字体度量。
func renderDetail(t *testing.T, d EvalDetail) []byte {
	t.Helper()
	if _, err := loadChineseFont(); err != nil {
		t.Skipf("本机无中文字体，跳过: %v", err)
	}
	out, err := RenderEvaluationPDF(d, "uploads")
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	assertPDF(t, out)
	if p := os.Getenv("PDF_DUMP"); p != "" {
		_ = os.WriteFile(p, out, 0o644)
		t.Logf("PDF 已转储: %s", p)
	}
	return out
}

// textDim 构造一个折行文本维度
func textDim(name, display string) EvalDetailGroup {
	return EvalDetailGroup{Name: "评语及建议", Dimensions: []EvalDetailDim{
		{Name: name, FieldType: "text", Display: display},
	}}
}

// TestEvalDetailNoTextOverlap 最小复现：一个文本维度折成 ≥2 行不得叠字
//（线上截图「意见与建议」「听课内容」的叠字症状）
func TestEvalDetailNoTextOverlap(t *testing.T) {
	d := EvalDetail{
		CourseName: "机械原理", TeacherName: "马英", TotalScore: 27, MaxTotalScore: 30,
		Groups: []EvalDetailGroup{
			textDim("意见与建议", "1.老师理论丰富，内容多，讲解详细。2.教学环节比较单一，思政元素无，学生有讲不好。3.有点板书，结合讲解例子。"),
		},
	}
	assertNoLayoutDefects(t, renderDetail(t, d))
}

// TestEvalDetailNoTextOverlapExplicitNewline 变体：值内含显式换行（听课内容场景）
func TestEvalDetailNoTextOverlapExplicitNewline(t *testing.T) {
	d := EvalDetail{Groups: []EvalDetailGroup{
		textDim("听课内容", "机械位移系统微分方程的建立\n建立初始微分方程\n…"),
	}}
	assertNoLayoutDefects(t, renderDetail(t, d))
}

// TestEvalDetailNoTextOverlapLongText 变体：长评语全额折行绘制（不截断）
func TestEvalDetailNoTextOverlapLongText(t *testing.T) {
	d := EvalDetail{Groups: []EvalDetailGroup{
		textDim("意见与建议", strings.Repeat("超长文本折行完整展示路径验证。", 20)),
	}}
	assertNoLayoutDefects(t, renderDetail(t, d))
}

// evalValXpt 维度值列的文本 x（valX+2 = 56+2 = 58mm，单位 pt）
const evalValXpt = 58.0 / 25.4 * 72

// valueColumnDraws 值列上的文本绘制次数（每一折行一次）
func valueColumnDraws(t *testing.T, pdf []byte) int {
	t.Helper()
	draws, _ := parsePDFContent(t, pdf)
	n := 0
	for _, d := range draws {
		if abs(d.x-evalValXpt) < 0.5 {
			n++
		}
	}
	return n
}

// TestEvalDetailLongTextFullyRendered 长评语必须完整绘制，不得截断。
// 旧版按「折行 3 行 + 省略号」截断，值列上恒定 4 次文本绘制——本测试即该缺陷的回归锁。
func TestEvalDetailLongTextFullyRendered(t *testing.T) {
	d := EvalDetail{Groups: []EvalDetailGroup{
		textDim("意见与建议", strings.Repeat("超长文本折行完整展示路径验证。", 30)),
	}}
	pdf := renderDetail(t, d)
	assertNoLayoutDefects(t, pdf)
	if n := valueColumnDraws(t, pdf); n <= 4 {
		t.Fatalf("长评语在值列应有超过 4 次文本绘制（旧版截断为 3 行 + 省略号），实际 %d 次", n)
	}
}

// TestEvalDetailLongTextSpansPages 超出单页容量的评语续写到下一页，而不是被裁掉。
func TestEvalDetailLongTextSpansPages(t *testing.T) {
	d := EvalDetail{Groups: []EvalDetailGroup{
		textDim("意见与建议", strings.Repeat("超长文本折行跨页续写路径验证。", 300)),
	}}
	pdf := renderDetail(t, d)
	assertNoLayoutDefects(t, pdf)
	// 单页正文上限约 floor((pageBottom-margin-headH-1.6)/4.4) ≈ 58 行
	if n := valueColumnDraws(t, pdf); n <= 58 {
		t.Fatalf("超长评语应跨页续写（单页上限约 58 行），实际共 %d 行", n)
	}
}

// TestScheduleNoTextOverlap 变体：课表信息多行值（同一公式的另一处调用点）
func TestScheduleNoTextOverlap(t *testing.T) {
	d := EvalDetail{
		ScheduleRows: []EvalInfoRow{
			{Label: "上课时间", Value: "第1-2节"},
			{Label: "听课内容", Value: "机械位移系统微分方程的建立\n建立初始微分方程\n…"},
		},
		Groups: []EvalDetailGroup{
			textDim("意见与建议", "1"),
		},
	}
	assertNoLayoutDefects(t, renderDetail(t, d))
}

// TestDimLabelNoCrossDivider 最小复现（线上截图）：超长维度名溢出 46mm 标签列、
// 画过名称/值分隔线
func TestDimLabelNoCrossDivider(t *testing.T) {
	d := EvalDetail{Groups: []EvalDetailGroup{{
		Name: "教学内容", Score: 50, MaxScore: 50,
		Dimensions: []EvalDetailDim{
			{Name: "熟悉教学内容，节奏流畅，重点难点突出", FieldType: "score", Score: 10, MaxScore: 10},
			{Name: "课程思政融入明显", FieldType: "score", Score: 10, MaxScore: 10},
			{Name: "教学理念/设计/改革具有高阶性、创新性、挑战度", FieldType: "score", Score: 10, MaxScore: 10},
			{Name: "教学目标明确，符合教学大纲", FieldType: "score", Score: 10, MaxScore: 10},
			{Name: "理论联系实际，注重能力培养", FieldType: "score", Score: 10, MaxScore: 10},
		},
	}}}
	assertNoLayoutDefects(t, renderDetail(t, d))
}

// TestEvalDetailFullScenarioNoTextOverlap 端到端：按线上截图还原的完整评教详情
func TestEvalDetailFullScenarioNoTextOverlap(t *testing.T) {
	d := EvalDetail{
		CourseName: "机械原理", TeacherName: "马英", CollegeName: "信息与电气工程学院",
		EvaluatorName: "系统管理员", EvaluatorRole: "督导", Submit: "2026-09-05 10:00",
		ClassTime: "第1-2节", TotalScore: 27, MaxTotalScore: 30,
		ScheduleRows: []EvalInfoRow{
			{Label: "上课时间", Value: "第1-2节"},
			{Label: "听课内容", Value: "机械位移系统微分方程的建立\n建立初始微分方程\n…"},
		},
		Groups: []EvalDetailGroup{
			{Name: "教学内容", Score: 50, MaxScore: 50, Dimensions: []EvalDetailDim{
				{Name: "熟悉教学内容，节奏流畅，重点难点突出", FieldType: "score", Score: 10, MaxScore: 10},
				{Name: "课程思政融入明显", FieldType: "score", Score: 10, MaxScore: 10},
				{Name: "教学理念/设计/改革具有高阶性、创新性、挑战度", FieldType: "score", Score: 10, MaxScore: 10},
				{Name: "教学目标明确，符合教学大纲", FieldType: "score", Score: 10, MaxScore: 10},
				{Name: "理论联系实际，注重能力培养", FieldType: "score", Score: 10, MaxScore: 10},
			}},
			{Name: "教学方法与手段", Score: 14, MaxScore: 15, Dimensions: []EvalDetailDim{
				{Name: "板书合理、课件规范", FieldType: "score", Score: 5, MaxScore: 5},
				{Name: "启发式/讨论式教学，注重互动", FieldType: "score", Score: 9, MaxScore: 10},
			}},
			{Name: "教学效果", Score: 13, MaxScore: 15, Dimensions: []EvalDetailDim{
				{Name: "课堂气氛活跃，听课率高", FieldType: "score", Score: 9, MaxScore: 10},
				{Name: "能启发学生创新，收获大", FieldType: "score", Score: 4, MaxScore: 5},
			}},
			{Name: "评语及建议", Dimensions: []EvalDetailDim{
				{Name: "意见与建议", FieldType: "text", Display: "1.老师理论丰富，内容多，讲解详细。2.教学环节比较单一，思政元素无，学生有讲不好。3.有点板书，结合讲解例子。"},
			}},
			{Name: "听课情况", Dimensions: []EvalDetailDim{
				{Name: "听课内容", FieldType: "text", Display: "机械位移系统微分方程的建立\n建立初始微分方程\n…"},
			}},
		},
	}
	assertNoLayoutDefects(t, renderDetail(t, d))
}

// TestEvalDetailNoTextOverlapWidthBoundary 扫过「折行 3 行→截断 3 行+省略号」的
// 长度边界窗口：行高预算与绘制行槽必须同折行口径，否则省略号溢出压到下一行
func TestEvalDetailNoTextOverlapWidthBoundary(t *testing.T) {
	for l := 90; l <= 130; l++ {
		d := EvalDetail{Groups: []EvalDetailGroup{
			textDim("意见与建议", strings.Repeat("评", l)),
			textDim("存在问题", "下一行锚点文本"),
		}}
		out := renderDetail(t, d)
		t.Run(fmt.Sprintf("长度%d", l), func(t *testing.T) {
			assertNoLayoutDefects(t, out)
		})
	}
}
