package pdfgen

import (
	"fmt"

	"github.com/signintech/gopdf"
)

// v2 generate_pdf 表格配色（reportlab 版）
var (
	colTableHead = [3]uint8{54, 96, 146} // #366092
	colTableGrid = [3]uint8{128, 128, 128}
)

// TableCol 表格列（数据键 / 显示标题）
type TableCol struct{ Key, Label string }

// RenderTablePDF 生成表格类报表 PDF（对齐旧端 generate_pdf：
// A4 竖版 2cm 边距、居中标题、蓝底白字表头、灰网格线、均分列宽、跨页重复表头）。
func RenderTablePDF(title string, cols []TableCol, rows []map[string]interface{}) ([]byte, error) {
	font, err := loadChineseFont()
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("导出列不能为空")
	}
	gp := gopdf.GoPdf{}
	gp.Start(gopdf.Config{Unit: gopdf.UnitMM, PageSize: *gopdf.PageSizeA4})
	if err := gp.AddTTFFontData(FontFamily, font); err != nil {
		return nil, fmt.Errorf("中文字体加载失败: %w", err)
	}
	gp.AddPage()

	const m = 20.0 // 旧端 2cm 边距
	usableW := pageW - 2*m
	bottom := pageH - m
	y := m

	// 标题（居中 16pt，对齐旧端 ChineseTitle）
	if title != "" {
		b := &evalPainter{gp: &gp, y: y}
		b.textCenter(y, title, 16, colText)
		y += 12
	}

	colW := usableW / float64(len(cols))
	const headH, rowH = 8.0, 6.5

	cell := func(x, cy float64, s string, pt float64, c [3]uint8, center bool) {
		b := &evalPainter{gp: &gp}
		b.setFont(pt)
		b.setColor(c)
		w, _ := gp.MeasureTextWidth(s)
		if center {
			gp.SetXY(x+(colW-w)/2, cy)
		} else {
			gp.SetXY(x+1.5, cy)
		}
		_ = gp.Text(s)
	}

	drawHead := func() {
		gp.SetFillColor(colTableHead[0], colTableHead[1], colTableHead[2])
		gp.RectFromUpperLeftWithStyle(m, y, usableW, headH, "F")
		for i, col := range cols {
			cell(m+float64(i)*colW, y+2.5, col.Label, 9, colWhite, true)
		}
		y += headH
	}

	// 数据行（跨页时重画表头，对齐旧端 repeatRows=1）
	for _, r := range rows {
		// 行高按折行数取最大值
		nLines := 1
		for _, col := range cols {
			if n := len(wrapCellText(&gp, cellText(r[col.Key]), colW-3, 8.5)); n > nLines {
				nLines = n
			}
		}
		rh := float64(nLines)*4.0 + 2.5
		if y+rh > bottom {
			gp.AddPage()
			y = m
			drawHead()
		}
		for i, col := range cols {
			x := m + float64(i)*colW
			gp.SetStrokeColor(colTableGrid[0], colTableGrid[1], colTableGrid[2])
			gp.SetLineWidth(0.15)
			gp.RectFromUpperLeftWithStyle(x, y, colW, rh, "S")
			lines := wrapCellText(&gp, cellText(r[col.Key]), colW-3, 8.5)
			for j, ln := range lines {
				cell(x, y+1.2+float64(j)*4.0, ln, 8.5, colText, true)
			}
		}
		y += rh
	}

	return gp.GetBytesPdfReturnErr()
}

// cellText 单元格值转文本（对齐旧端 str(value)）
func cellText(v interface{}) string {
	switch n := v.(type) {
	case nil:
		return ""
	case float64:
		return trimFloat(n)
	case bool:
		if n {
			return "True"
		}
		return "False"
	case string:
		return n
	default:
		return fmt.Sprintf("%v", n)
	}
}

// wrapCellText 单元格文本折行（复用文档字体度量）
func wrapCellText(gp *gopdf.GoPdf, s string, w float64, pt float64) []string {
	if s == "" {
		return []string{""}
	}
	_ = gp.SetFont(FontFamily, "", pt)
	lines, err := gp.SplitText(s, w)
	if err != nil || len(lines) == 0 {
		return []string{s}
	}
	return lines
}
