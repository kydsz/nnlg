package jwxt

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	xls "github.com/shakinm/xlsReader/xls"
)

// DebugReadSheetRows 调试用：导出解析行
func DebugReadSheetRows(data []byte, headerRow int) ([]map[string]string, error) {
	return readSheetRows(data, headerRow)
}

// readSheetRows 读取首个工作表，以 headerRow（0 基）为表头，返回按列名映射的数据行。
// 教务系统导出格式可能是 .xls（BIFF）也可能是 .xlsx（OOXML），按文件头自动选择解析器：
// 两者都产出同一张矩阵，再复用同一套表头映射逻辑。
func readSheetRows(data []byte, headerRow int) ([]map[string]string, error) {
	if isZipArchive(data) {
		return readXlsxRows(data, headerRow)
	}
	return readXlsRows(data, headerRow)
}

// isZipArchive 判断是否为 zip 容器（xlsx/ods 等 OOXML 文件都是 zip）
func isZipArchive(data []byte) bool {
	if len(data) < 4 || data[0] != 'P' || data[1] != 'K' {
		return false
	}
	switch {
	case data[2] == 3 && data[3] == 4: // 常规
		return true
	case data[2] == 5 && data[3] == 6: // 空档案
		return true
	case data[2] == 7 && data[3] == 8: // 分卷
		return true
	}
	return false
}

// readXlsxRows 读取 xlsx 首个工作表
func readXlsxRows(data []byte, headerRow int) ([]map[string]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("打开 Excel 失败: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel 无工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("读取工作表失败: %w", err)
	}
	return rowsFromMatrix(rows, headerRow)
}

// readXlsRows 读取 xls 首个工作表（xlsReader 只在单元格范围内可读，越界即视为该行结束）
func readXlsRows(data []byte, headerRow int) ([]map[string]string, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("打开 Excel 失败: %w", err)
	}
	sheet, err := wb.GetSheet(0)
	if err != nil || sheet == nil {
		return nil, fmt.Errorf("Excel 无工作表")
	}
	header, err := sheet.GetRow(headerRow)
	if err != nil || header == nil {
		return nil, fmt.Errorf("表头行不存在")
	}

	// 表头扫描宽度：空表头最多连续容忍一小段，遇到长尾即止
	width := 0
	for j := 0; ; j++ {
		cell, ce := header.GetCol(j)
		if ce != nil {
			break
		}
		name := ""
		if cell != nil {
			name = strings.TrimSpace(cell.GetString())
		}
		if name == "" && j > 40 {
			break
		}
		width = j + 1
	}
	if width == 0 {
		return nil, fmt.Errorf("表头为空")
	}

	total := sheet.GetNumberRows()
	matrix := make([][]string, 0, total)
	for i := 0; i < total; i++ {
		row, re := sheet.GetRow(i)
		if re != nil || row == nil {
			matrix = append(matrix, nil)
			continue
		}
		cells := make([]string, width)
		for j := 0; j < width; j++ {
			cell, ce := row.GetCol(j)
			if ce != nil || cell == nil {
				continue
			}
			cells[j] = cell.GetString()
		}
		matrix = append(matrix, cells)
	}
	return rowsFromMatrix(matrix, headerRow)
}

// rowsFromMatrix 表头映射 + 数据行解析（xls/xlsx 共用）
func rowsFromMatrix(matrix [][]string, headerRow int) ([]map[string]string, error) {
	if headerRow < 0 || headerRow >= len(matrix) {
		return nil, fmt.Errorf("表头行不存在")
	}
	colNames := map[int]string{}
	for j, raw := range matrix[headerRow] {
		name := strings.TrimSpace(raw)
		if name != "" {
			colNames[j] = name
		}
		if name == "" && j > 40 {
			break
		}
	}
	if len(colNames) == 0 {
		return nil, fmt.Errorf("表头为空")
	}

	var rows []map[string]string
	for i := headerRow + 1; i < len(matrix); i++ {
		m := map[string]string{}
		nonEmpty := 0
		for j, name := range colNames {
			if j >= len(matrix[i]) {
				continue
			}
			v := strings.TrimSpace(matrix[i][j])
			m[name] = v
			if v != "" {
				nonEmpty++
			}
		}
		if nonEmpty > 0 {
			rows = append(rows, m)
		}
	}
	return rows, nil
}
