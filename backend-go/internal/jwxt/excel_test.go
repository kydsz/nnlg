package jwxt

import (
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildXlsx 构造测试用 xlsx：第 0/1 行为说明行，第 2 行为表头（对齐教务系统导出格式）
func buildXlsx(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	rows := [][]interface{}{
		{"教师课表", "", ""},
		{"导出时间", "2026-09-12", ""},
		{"工号", "姓名", "课程数"},
		{"2023001", "张三", "3"},
		{"", "", ""},
		{"2023002", "李四", "0"},
	}
	for i, row := range rows {
		for j, v := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("构造单元格失败: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				t.Fatalf("写入单元格失败: %v", err)
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("生成 xlsx 失败: %v", err)
	}
	return buf.Bytes()
}

func TestIsZipArchive(t *testing.T) {
	if isZipArchive([]byte("PK")) {
		t.Fatal("长度不足不应识别为 zip")
	}
	if !isZipArchive([]byte{'P', 'K', 3, 4, 0}) {
		t.Fatal("PK\\x03\\x04 应识别为 zip")
	}
	// 旧版 xls 是 OLE2 复合文档头，必须走 xlsReader 分支
	if isZipArchive([]byte{0xD0, 0xCF, 0x11, 0xE0}) {
		t.Fatal("xls (OLE2) 不应识别为 zip")
	}
}

func TestReadSheetRowsXlsx(t *testing.T) {
	data := buildXlsx(t)
	if !isZipArchive(data) {
		t.Fatal("xlsx 应识别为 zip 容器")
	}
	rows, err := DebugReadSheetRows(data, 2)
	if err != nil {
		t.Fatalf("解析 xlsx 失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应解析出 2 行有效数据, 实际 %d: %+v", len(rows), rows)
	}
	if rows[0]["工号"] != "2023001" || rows[0]["姓名"] != "张三" || rows[0]["课程数"] != "3" {
		t.Fatalf("首行解析不符: %+v", rows[0])
	}
	if rows[1]["工号"] != "2023002" || rows[1]["姓名"] != "李四" {
		t.Fatalf("次行解析不符: %+v", rows[1])
	}
}

func TestReadSheetRowsXlsxHeaderMissing(t *testing.T) {
	if _, err := DebugReadSheetRows(buildXlsx(t), 99); err == nil {
		t.Fatal("表头行越界应报错")
	}
}

func TestReadSheetRowsUnsupportedFormat(t *testing.T) {
	if _, err := DebugReadSheetRows([]byte("这不是 Excel 文件"), 2); err == nil {
		t.Fatal("非法格式应报错")
	}
	if _, err := DebugReadSheetRows(nil, 2); err == nil {
		t.Fatal("空内容应报错")
	}
}
