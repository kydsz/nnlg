package pdfgen

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/signintech/gopdf"
)

// 注意：gopdf 的 SetFont size 即 pt 值（UnitMM 下 MeasureTextWidth 已换算为 mm），勿再转 mm。

// A4 尺寸（mm）与页边距（对齐旧端 Playwright 10mm 页边距）
const (
	pageW, pageH = 210.0, 297.0
	margin       = 10.0
)

// v2 模板配色（generate_evaluation_html）
var (
	colPrimary  = [3]uint8{25, 137, 250}  // #1989fa
	colText     = [3]uint8{51, 51, 51}    // #333
	colSub      = [3]uint8{102, 102, 102} // #666
	colMuted    = [3]uint8{153, 153, 153} // #999
	colInfoBG   = [3]uint8{245, 247, 250} // #f5f7fa
	colBorder   = [3]uint8{235, 238, 245} // #ebeef5
	colWhite    = [3]uint8{255, 255, 255}
	colDanger   = [3]uint8{204, 0, 0}   // #c00 总分/提示红（对齐打印模板）
	colDeepBlue = [3]uint8{16, 65, 134} // #104186 分组下划线（对齐打印模板）
)

// EvalDetail 评教详情 PDF 输入（由 handler 从 ExportData 组装）
type EvalDetail struct {
	CourseName, TeacherName, CollegeName string
	EvaluatorName, EvaluatorRole, Submit string
	ClassTime                            string
	IsAnonymous                          bool
	TotalScore, MaxTotalScore            float64
	// 课表信息（与提交页一致）：上课时间/教室/班级/应到人数/周次
	ScheduleRows []EvalInfoRow
	Groups       []EvalDetailGroup
}

// EvalInfoRow 课表信息单行（label + value）
type EvalInfoRow struct {
	Label, Value string
}

// EvalDetailGroup 维度分组
type EvalDetailGroup struct {
	Name            string
	Score, MaxScore float64
	Dimensions      []EvalDetailDim
}

// EvalDetailDim 单个维度
type EvalDetailDim struct {
	Name, FieldType string
	Value           interface{} // 原始值（image/file 为 URL 数组）
	Display         string      // 显示值（选项已转标签）
	Score, MaxScore float64
}

// RenderEvaluationPDF 生成单条评教记录 PDF（布局对齐旧端 generate_evaluation_html 紧凑模板）。
func RenderEvaluationPDF(d EvalDetail, uploadDir string) ([]byte, error) {
	font, err := loadChineseFont()
	if err != nil {
		return nil, err
	}
	gp := gopdf.GoPdf{}
	gp.Start(gopdf.Config{Unit: gopdf.UnitMM, PageSize: *gopdf.PageSizeA4})
	if err := gp.AddTTFFontData(FontFamily, font); err != nil {
		return nil, fmt.Errorf("中文字体加载失败: %w", err)
	}
	gp.AddPage()

	b := &evalPainter{gp: &gp, uploadDir: uploadDir, y: margin}
	b.drawHeader(d)
	b.drawInfo(d)
	if len(d.ScheduleRows) > 0 {
		b.drawSchedule(d.ScheduleRows)
	}
	for i := range d.Groups {
		b.drawGroup(&d.Groups[i])
	}
	b.drawFooter()
	return gp.GetBytesPdfReturnErr()
}

// evalPainter 纵向游标绘制器
type evalPainter struct {
	gp        *gopdf.GoPdf
	uploadDir string
	y         float64
}

func (b *evalPainter) setFont(pt float64)  { _ = b.gp.SetFont(FontFamily, "", pt) }
func (b *evalPainter) setColor(c [3]uint8) { b.gp.SetTextColor(c[0], c[1], c[2]) }

// baseLine 行顶+行高内垂直居中的基线位置（gopdf Text 以基线定位，CJK 可视高度约 0.75em）
func baseLine(yTop, h, pt float64) float64 {
	return yTop + (h + pt*0.3528*0.75) / 2
}

func (b *evalPainter) text(x, y float64, s string, pt float64, c [3]uint8) {
	b.setFont(pt)
	b.setColor(c)
	b.gp.SetXY(x, y)
	_ = b.gp.Text(s)
}

func (b *evalPainter) textCenter(y float64, s string, pt float64, c [3]uint8) {
	b.setFont(pt)
	b.setColor(c)
	w, _ := b.gp.MeasureTextWidth(s)
	b.gp.SetXY((pageW-w)/2, y)
	_ = b.gp.Text(s)
}

func (b *evalPainter) textRight(right, y float64, s string, pt float64, c [3]uint8) {
	b.setFont(pt)
	b.setColor(c)
	w, _ := b.gp.MeasureTextWidth(s)
	b.gp.SetXY(right-w, y)
	_ = b.gp.Text(s)
}

func (b *evalPainter) fillRect(x, y, w, h float64, c [3]uint8) {
	b.gp.SetFillColor(c[0], c[1], c[2])
	b.gp.RectFromUpperLeftWithStyle(x, y, w, h, "F")
}

func (b *evalPainter) strokeRect(x, y, w, h float64, c [3]uint8, lw float64) {
	b.gp.SetStrokeColor(c[0], c[1], c[2])
	b.gp.SetLineWidth(lw)
	b.gp.RectFromUpperLeftWithStyle(x, y, w, h, "S")
}

// newPage 空间不足时换页（底部预留页脚区）
func (b *evalPainter) newPage(need float64) {
	if b.y+need > pageH-margin-8 {
		b.gp.AddPage()
		b.y = margin
	}
}

// drawHeader 标题区：黑色大标题 + 灰色副标题 + 深蓝分隔线（对齐打印模板 h1）
func (b *evalPainter) drawHeader(d EvalDetail) {
	b.textCenter(margin, "评教详情", 18, colText)
	b.textCenter(margin+6.5, "南宁理工学院", 9, colSub)
	// 深蓝分隔线与副标题保持间距
	b.gp.SetStrokeColor(colDeepBlue[0], colDeepBlue[1], colDeepBlue[2])
	b.gp.SetLineWidth(0.8)
	b.gp.Line(margin, margin+12.0, pageW-margin, margin+12.0)
	b.y = margin + 16.0
}

// drawInfo 信息区：3×3 网格表格（灰 label + 黑 value），总分行红色突出（对齐打印模板信息表）
func (b *evalPainter) drawInfo(d EvalDetail) {
	scoreLabel := fmt.Sprintf("%s/%s 分", trimFloat(d.TotalScore), trimFloat(d.MaxTotalScore))
	typ := "实名"
	if d.IsAnonymous {
		typ = "匿名"
	}
	labels := [9]string{"教师", "课程", "上课时间", "学院", "评教人", "评教角色", "提交时间", "总分", "是否匿名"}
	values := [9]string{
		orDash(d.TeacherName), orDash(d.CourseName), orDash(d.ClassTime),
		orDash(d.CollegeName), orDash(d.EvaluatorName), orDash(d.EvaluatorRole),
		orDash(d.Submit), scoreLabel, typ,
	}

	const pad, rowH = 2.2, 6.2
	cardH := rowH*3 + 0.6 // 边框厚度
	b.newPage(cardH)
	top := b.y

	colW := (pageW - 2*margin) / 3
	// 外框
	b.strokeRect(margin, top, pageW-2*margin, cardH, colBorder, 0.4)
	for i := 0; i < 9; i++ {
		row, col := i/3, i%3
		x := margin + float64(col)*colW
		y := top + float64(row)*rowH
		// 网格线（右侧竖线、行间横线）
		if col < 2 {
			b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
			b.gp.SetLineWidth(0.25)
			b.gp.Line(x+colW, y, x+colW, y+rowH)
		}
		if row < 2 {
			b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
			b.gp.SetLineWidth(0.25)
			b.gp.Line(x, y+rowH, x+colW, y+rowH)
		}
		if labels[i] == "" {
			continue
		}
		label := labels[i] + "："
		b.text(x+pad, baseLine(y, rowH, 9.5), label, 9.5, colSub)
		b.setFont(9.5)
		lw, _ := b.gp.MeasureTextWidth(label)
		vx := x + pad + lw
		if i == 7 { // 总分红色加粗显示
			b.text(vx, baseLine(y, rowH, 12), values[i], 12, colDanger)
		} else {
			b.text(vx, baseLine(y, rowH, 11), values[i], 11, colText)
		}
	}
	b.y = top + cardH + 3.0
}

// drawSchedule 课表信息区：标题 + 两列表格（课表信息，对齐提交页展示内容）
func (b *evalPainter) drawSchedule(rows []EvalInfoRow) {
	const headH = 8.0
	const labelW = 46.0
	left, right := margin, pageW-margin
	valX := left + labelW
	valW := right - valX - 2

	// 标题
	b.text(left, b.y+4.2, "课表信息", 12, colText)
	b.gp.SetStrokeColor(colDeepBlue[0], colDeepBlue[1], colDeepBlue[2])
	b.gp.SetLineWidth(0.8)
	b.gp.Line(left, b.y+headH-1.2, right, b.y+headH-1.2)
	b.y += headH

	for _, row := range rows {
		lines := b.wrapText(row.Value, valW-2, 11)
		h := 6.0
		if len(lines) > 1 {
			h = float64(len(lines))*4.4 + 1.6
		}
		if b.y+h > pageH-margin-8 {
			b.gp.AddPage()
			b.y = margin
			b.text(left, b.y+4.2, "课表信息（续）", 12, colText)
			b.gp.SetStrokeColor(colDeepBlue[0], colDeepBlue[1], colDeepBlue[2])
			b.gp.SetLineWidth(0.8)
			b.gp.Line(left, b.y+headH-1.2, right, b.y+headH-1.2)
			b.y += headH
		}
		b.fillRect(left, b.y, labelW, h, colInfoBG)
		b.text(left+3, baseLine(b.y, h, 10), row.Label, 10, colText)
		b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
		b.gp.SetLineWidth(0.25)
		b.gp.Line(valX, b.y, valX, b.y+h)
		b.gp.Line(left, b.y+h, right, b.y+h)
		for i, ln := range lines {
			b.text(valX+2, baseLine(b.y, h, 11)+float64(i-(len(lines)-1))/2*4.4, ln, 11, colText)
		}
		b.y += h
	}
	b.y += 3.0
}

// drawGroup 分组区块：组头（组名/得分 + 深蓝下划线）+ 两列维度表；跨页时重复组头（对齐打印模板 h3）
func (b *evalPainter) drawGroup(g *EvalDetailGroup) {
	const headH = 8.0
	const nameW = 46.0
	left, right := margin, pageW-margin
	valX := left + nameW
	valW := right - valX - 2

	// 组头绘制（新页重复调用）：组名靠上，深蓝下划线与组名保持间距
	drawHead := func() {
		nameBase := b.y + 4.2
		b.text(left, nameBase, g.Name, 12, colText)
		if g.MaxScore > 0 {
			b.textRight(right, nameBase, trimFloat(g.Score)+"/"+trimFloat(g.MaxScore)+" 分", 10, colPrimary)
		}
		// 深蓝下划线（对齐打印模板 border-bottom:2px solid #104186）
		b.gp.SetStrokeColor(colDeepBlue[0], colDeepBlue[1], colDeepBlue[2])
		b.gp.SetLineWidth(0.8)
		b.gp.Line(left, b.y+headH-1.2, right, b.y+headH-1.2)
		b.y += headH
	}

	// 换页预留组头+首行，避免组头孤悬页底
	firstNeed := headH
	if len(g.Dimensions) > 0 {
		firstNeed += b.dimRowHeight(&g.Dimensions[0], valW)
	}
	if b.y+firstNeed > pageH-margin-8 {
		b.gp.AddPage()
		b.y = margin
	}
	frameTop := b.y
	// 表框顶部细线
	b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
	b.gp.SetLineWidth(0.25)
	b.gp.Line(left, b.y, right, b.y)
	drawHead()

	for i := range g.Dimensions {
		need := b.dimRowHeight(&g.Dimensions[i], valW)
		if b.y+need > pageH-margin-8 {
			// 封闭当前页组框，换页后重开框并重复组头
			b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
			b.gp.SetLineWidth(0.3)
			b.gp.Line(left, frameTop, left, b.y)
			b.gp.Line(right, frameTop, right, b.y)
			b.gp.AddPage()
			b.y = margin
			frameTop = b.y
			b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
			b.gp.SetLineWidth(0.25)
			b.gp.Line(left, b.y, right, b.y)
			drawHead()
		}
		b.drawDimRow(left, right, nameW, valX, valW, &g.Dimensions[i])
	}
	// 闭合组框：左右竖线
	b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
	b.gp.SetLineWidth(0.3)
	b.gp.Line(left, frameTop, left, b.y)
	b.gp.Line(right, frameTop, right, b.y)
	b.y += 3.0
}

// dimRowHeight 维度行高（文本按实际折行数、图片行按缩略图高）
func (b *evalPainter) dimRowHeight(dim *EvalDetailDim, valW float64) float64 {
	switch dim.FieldType {
	case "image":
		if len(stringSlice(dim.Value)) > 0 {
			return 15.5
		}
	case "file":
		return 6.0
	default:
		n := len(b.wrapText(dim.Display, valW, 11))
		if n > 3 {
			n = 3
		}
		if n > 1 {
			return float64(n)*4.4 + 1.6
		}
	}
	return 6.0
}

// drawDimRow 维度行（两列表格）：名称单元格灰底，值单元格左对齐；分数右对齐；图片缩略图、文件显示文件名
func (b *evalPainter) drawDimRow(left, right, nameW, valX, valW float64, dim *EvalDetailDim) {
	h := b.dimRowHeight(dim, valW)

	// 名称单元格（浅灰底，对齐打印模板 td 背景 #f5f7fa）
	b.fillRect(left, b.y, nameW, h, colInfoBG)
	b.text(left+3, baseLine(b.y, h, 10), dim.Name, 10, colText)

	// 名称/值分隔竖线 + 行底横线
	b.gp.SetStrokeColor(colBorder[0], colBorder[1], colBorder[2])
	b.gp.SetLineWidth(0.25)
	b.gp.Line(valX, b.y, valX, b.y+h)
	b.gp.Line(left, b.y+h, right, b.y+h)

	switch dim.FieldType {
	case "image":
		b.drawImages(valX+2, b.y+1, dim.Value)
	case "file":
		b.drawFiles(valX+2, baseLine(b.y, h, 10), dim.Value)
	case "score":
		if dim.MaxScore > 0 {
			b.textRight(right-2, baseLine(b.y, h, 11), trimFloat(dim.Score)+"/"+trimFloat(dim.MaxScore)+" 分", 11, colText)
		} else {
			b.textRight(right-2, baseLine(b.y, h, 11), dim.Display, 11, colText)
		}
	default:
		lines := b.wrapText(dim.Display, valW-2, 11)
		n := len(lines)
		if n > 3 {
			n = 3
		}
		for i := 0; i < n; i++ {
			b.text(valX+2, baseLine(b.y, h, 11)+float64(i-(n-1))/2*4.4, lines[i], 11, colText)
		}
		if len(lines) > 3 { // 超出截断（对齐旧端 50 字截断）
			b.text(valX+2+measureWidth(b, "…", 11), baseLine(b.y, h, 11)+4.4, "…", 11, colText)
		}
	}
	b.y += h
}

// measureWidth 文本显示宽度（mm）
func measureWidth(b *evalPainter, s string, pt float64) float64 {
	b.setFont(pt)
	w, _ := b.gp.MeasureTextWidth(s)
	return w
}

// drawImages 图片缩略图（最多 4 张，14mm，对齐 v2 dimension-images）
func (b *evalPainter) drawImages(x, y float64, v interface{}) {
	urls := stringSlice(v)
	size, gap := 14.0, 1.4
	cx := x
	for _, u := range urls {
		if cx+size > pageW-margin {
			break
		}
		holder, err := loadImageHolder(u, b.uploadDir)
		if err != nil {
			continue
		}
		_ = b.gp.ImageByHolder(holder, cx, y, &gopdf.Rect{W: size, H: size})
		cx += size + gap
	}
	if cx == x { // 无可用图片
		b.textRight(pageW-margin-3, y+1.5, "-", 9, colMuted)
	}
}

// drawFiles 附件文件名列表（最多 3 个，对齐 v2 file-item）
func (b *evalPainter) drawFiles(x, y float64, v interface{}) {
	urls := stringSlice(v)
	if len(urls) == 0 {
		b.textRight(pageW-margin-3, y, "无附件", 9, colMuted)
		return
	}
	names := make([]string, 0, 3)
	for _, u := range urls {
		if len(names) >= 3 {
			break
		}
		names = append(names, fileNameFromURL(u))
	}
	b.text(x, y, "附件："+strings.Join(names, "、"), 10, colText)
}

// drawFooter 页脚生成时间（固定页底）
func (b *evalPainter) drawFooter() {
	b.textCenter(pageH-margin-4, "生成时间："+time.Now().Format("2006-01-02 15:04"), 9, colMuted)
}

// wrapText 按像素宽折行（CJK 逐字断行）
func (b *evalPainter) wrapText(s string, w float64, pt float64) []string {
	if s == "" {
		return []string{"-"}
	}
	b.setFont(pt)
	lines, err := b.gp.SplitText(s, w)
	if err != nil || len(lines) == 0 {
		return []string{s}
	}
	return lines
}

// ---------- 工具 ----------

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// stringSlice interface{} 转 []string（JSON 数组场景）
func stringSlice(v interface{}) []string {
	switch n := v.(type) {
	case []string:
		return n
	case []interface{}:
		out := make([]string, 0, len(n))
		for _, item := range n {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// fileNameFromURL 从 URL 提取文件名（对齐 v2 _get_filename_from_url）
func fileNameFromURL(u string) string {
	name := u
	if i := strings.LastIndexAny(u, "/\\"); i >= 0 {
		name = u[i+1:]
	}
	if name == "" {
		return "未知文件"
	}
	return name
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// loadImageHolder 加载图片：data: 内联 / http(s) 下载 / 相对路径映射 uploads 目录
func loadImageHolder(u, uploadDir string) (gopdf.ImageHolder, error) {
	if strings.HasPrefix(u, "data:image/") {
		if i := strings.Index(u, ","); i > 0 {
			if raw, err := base64.StdEncoding.DecodeString(u[i+1:]); err == nil {
				return gopdf.ImageHolderByBytes(raw)
			}
		}
		return nil, fmt.Errorf("无效的 data URL")
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		resp, err := httpClient.Get(u)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		if err != nil {
			return nil, err
		}
		return gopdf.ImageHolderByBytes(data)
	}
	// /api/v1/files/xxx 或 /files/xxx → uploads/xxx
	rel := strings.TrimPrefix(u, "/api/v1")
	rel = strings.TrimPrefix(rel, "/files")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return nil, fmt.Errorf("无效路径")
	}
	data, err := os.ReadFile(filepath.Join(uploadDir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	return gopdf.ImageHolderByBytes(data)
}
