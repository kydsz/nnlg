package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"backend-go/internal/model"

	"github.com/xuri/excelize/v2"
)

// teiLongText 足以触发列宽封顶（displayWidth > 50）的长文本
var teiLongText = strings.Repeat("意见内容足够长", 12)

// seedTEIExportEnv 构建评教记录导出的最小数据环境：1 个任务 + 2 条评教记录
// （一条填写了 TEI 短文本，一条未填写 TEI）。
func seedTEIExportEnv(t *testing.T) (*testEnv, int64) {
	t.Helper()
	env := newTestServer(t)
	if err := env.db.AutoMigrate(
		&model.EvaluationTask{}, &model.EvaluationRecord{}, &model.EvaluationDimension{},
	); err != nil {
		t.Fatalf("补齐评教相关表失败: %v", err)
	}
	admin := systemAdminUser()
	env.seedUser(t, admin)
	teacher := &model.User{UserNo: "T001", Username: "李老师", Role: model.RoleTeacher, Status: 1}
	env.seedUser(t, teacher)
	classTime := model.LocalTime(time.Now()) // 落在默认"当前学期"区间内，避免被日期过滤
	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: "李老师", CourseName: "高等数学",
		ClassTime: &classTime, Status: model.TaskStatusPending,
	}
	if err := env.db.Create(&task).Error; err != nil {
		t.Fatalf("灌入任务失败: %v", err)
	}
	withTEI := model.EvaluationRecord{
		TaskID: task.ID, EvaluatorName: "王督导", EvaluatorRole: model.RoleSupervisor,
		DimensionValues: json.RawMessage(`{"TEI":"` + teiLongText + `"}`),
	}
	if err := env.db.Create(&withTEI).Error; err != nil {
		t.Fatalf("灌入评教记录失败: %v", err)
	}
	withoutTEI := model.EvaluationRecord{
		TaskID: task.ID, EvaluatorName: "钱教师", EvaluatorRole: model.RoleTeacher,
		DimensionValues: json.RawMessage(`{"listening_content":"板书清晰"}`),
	}
	if err := env.db.Create(&withoutTEI).Error; err != nil {
		t.Fatalf("灌入评教记录失败: %v", err)
	}
	return env, int64(admin.ID)
}

// parseXlsxRows 解析导出响应为行数据（第 1 行报表标题、第 2 行表头、其余数据行）。
func parseXlsxRows(t *testing.T, body []byte) [][]string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("解析 XLSX 失败: %v", err)
	}
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("读取 Sheet1 失败: %v", err)
	}
	if len(rows) < 3 {
		t.Fatalf("应有标题+表头+数据行, 实际 %d 行", len(rows))
	}
	return rows
}

// xlsxCell 取单元格文本（GetRows 会裁掉行尾空单元格，越界视为空串）。
func xlsxCell(row []string, idx int) string {
	if idx < len(row) {
		return row[idx]
	}
	return ""
}

// TestExportEvaluationRecordsTEIColumn 评教记录导出应支持 TEI（意见与建议）列：
// 表头用中文名、按 fields 顺序排在最末，取值为维度作答文本，未填写为空；
// 长文本列宽度封顶 50 并自动换行，短文本列不受影响。
func TestExportEvaluationRecordsTEIColumn(t *testing.T) {
	env, adminID := seedTEIExportEnv(t)

	body := `{"fields":["teacher_name","TEI"]}`
	w := env.do(t, http.MethodPost, "/api/v1/stats/evaluation-records/export", body, env.authHeader(t, adminID))
	if w.Code != http.StatusOK {
		t.Fatalf("导出状态码=%d, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("Content-Type=%q, 期望 xlsx", ct)
	}

	rows := parseXlsxRows(t, w.Body.Bytes())
	headers := rows[1]
	if len(headers) != 2 || headers[0] != "被评教师" || headers[1] != "意见与建议" {
		t.Fatalf("表头应为 [被评教师 意见与建议], 得到 %v", headers)
	}

	dataRows := rows[2:]
	if len(dataRows) != 2 {
		t.Fatalf("应有 2 条数据行, 得到 %d 行: %v", len(dataRows), dataRows)
	}
	var filled, empty []string
	for _, r := range dataRows {
		if xlsxCell(r, 1) != "" {
			filled = r
		} else {
			empty = r
		}
	}
	if xlsxCell(filled, 0) != "李老师" || xlsxCell(filled, 1) != teiLongText {
		t.Fatalf("已填写行的 TEI 值错误: %v", filled)
	}
	if xlsxCell(empty, 0) != "李老师" || xlsxCell(empty, 1) != "" {
		t.Fatalf("未填写行的 TEI 值应为空: %v", empty)
	}

	// 列宽与换行样式：TEI 列被长文本撑爆后封顶 50 并自动换行；短文本列保持默认
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("重新解析 XLSX 失败: %v", err)
	}
	// 表头"被评教师"显示宽度 8 + 4 = 12
	if width, _ := f.GetColWidth("Sheet1", "A"); width != 12 {
		t.Fatalf("teacher_name 列宽 = %v, 期望 12（不受换行机制影响）", width)
	}
	if width, _ := f.GetColWidth("Sheet1", "B"); width != 50 {
		t.Fatalf("TEI 列宽 = %v, 期望封顶 50", width)
	}
	// B3 = 首个数据行（标题占第 1 行、表头第 2 行，数据从第 3 行开始）
	cellID, _ := f.GetCellStyle("Sheet1", "B3")
	style, _ := f.GetStyle(cellID)
	if style == nil || !style.Alignment.WrapText {
		t.Fatalf("TEI 数据单元格应自动换行")
	}
	shortID, _ := f.GetCellStyle("Sheet1", "A3")
	shortStyle, _ := f.GetStyle(shortID)
	if shortStyle != nil && shortStyle.Alignment.WrapText {
		t.Fatalf("短文本列不应启用换行")
	}
}

// TestExportEvaluationRecordsWithoutTEI 不勾选 TEI 时，导出列集合与改动前一致（不含意见与建议）。
func TestExportEvaluationRecordsWithoutTEI(t *testing.T) {
	env, adminID := seedTEIExportEnv(t)

	w := env.do(t, http.MethodPost, "/api/v1/stats/evaluation-records/export", `{"fields":["teacher_name"]}`, env.authHeader(t, adminID))
	if w.Code != http.StatusOK {
		t.Fatalf("导出状态码=%d, body=%s", w.Code, w.Body.String())
	}
	rows := parseXlsxRows(t, w.Body.Bytes())
	headers := rows[1]
	if len(headers) != 1 || headers[0] != "被评教师" {
		t.Fatalf("表头应仅含 [被评教师], 得到 %v", headers)
	}
}
