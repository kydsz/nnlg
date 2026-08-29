package jwxt

import (
	"bytes"
	"fmt"
	"strings"

	xls "github.com/shakinm/xlsReader/xls"
)

// DebugReadSheetRows 调试用：导出解析行
func DebugReadSheetRows(data []byte, headerRow int) ([]map[string]string, error) {
	return readSheetRows(data, headerRow)
}

// readSheetRows 读取 xls 第 0 个工作表，以 headerRow（0 基）为表头，
// 返回按列名映射的数据行。
func readSheetRows(data []byte, headerRow int) ([]map[string]string, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("打开 Excel 失败: %w", err)
	}
	sheet, err := wb.GetSheet(0)
	if err != nil || sheet == nil {
		return nil, fmt.Errorf("Excel 无工作表")
	}

	// 表头行 -> 列名映射
	header, err := sheet.GetRow(headerRow)
	if err != nil || header == nil {
		return nil, fmt.Errorf("表头行不存在")
	}
	colNames := map[int]string{}
	for j := 0; ; j++ {
		cell, e := header.GetCol(j)
		if e != nil {
			break
		}
		name := strings.TrimSpace(cell.GetString())
		if name != "" {
			colNames[j] = name
		}
		// 空表头最多连续容忍一小段，遇到长尾即止
		if name == "" && j > 40 {
			break
		}
	}
	if len(colNames) == 0 {
		return nil, fmt.Errorf("表头为空")
	}

	var rows []map[string]string
	total := sheet.GetNumberRows()
	for i := headerRow + 1; i < total; i++ {
		row, e := sheet.GetRow(i)
		if e != nil || row == nil {
			continue
		}
		m := map[string]string{}
		nonEmpty := 0
		for j, name := range colNames {
			cell, ce := row.GetCol(j)
			if ce != nil || cell == nil {
				continue
			}
			v := strings.TrimSpace(cell.GetString())
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
