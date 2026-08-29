package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/pdfgen"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// xlsxCol 导出列定义（数据键/显示标题）
type xlsxCol struct{ Key, Label string }

// displayWidth 估算显示宽度（中文按 2 字符，对齐旧端 _display_width）
func displayWidth(v interface{}) int {
	s := ""
	switch n := v.(type) {
	case nil:
		s = ""
	case float64:
		s = strconv.FormatFloat(n, 'f', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(n), 'f', -1, 64)
	default:
		s = fmt.Sprint(n)
	}
	w := 0
	for _, ch := range s {
		if (ch >= 0x4e00 && ch <= 0x9fff) || (ch >= 0x3000 && ch <= 0x303f) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// cellValue 单元格值转换：数字字符串存为数字（对齐旧端 generate_xlsx 的存储策略）
func cellValue(v interface{}) interface{} {
	switch n := v.(type) {
	case nil:
		return ""
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return n
	case string:
		if n == "" {
			return ""
		}
		if strings.Contains(n, ".") {
			if fv, err := strconv.ParseFloat(n, 64); err == nil {
				return fv
			}
			return n
		}
		if iv, err := strconv.ParseInt(n, 10, 64); err == nil {
			return iv
		}
		return n
	default:
		return fmt.Sprint(n)
	}
}

// generateExport 按格式分发导出（对齐旧端：xlsx 走 openpyxl 样式、pdf 走 reportlab 表格）
func generateExport(c *gin.Context, format, filename, title string, cols []xlsxCol, rows []map[string]interface{}) {
	if format == "pdf" {
		tcols := make([]pdfgen.TableCol, 0, len(cols))
		for _, col := range cols {
			tcols = append(tcols, pdfgen.TableCol{Key: col.Key, Label: col.Label})
		}
		pdfBytes, err := pdfgen.RenderTablePDF(title, tcols, rows)
		if err != nil {
			serverErr(c, "PDF 生成失败: "+err.Error())
			return
		}
		c.Header("Content-Disposition",
			"attachment; filename*=UTF-8''"+url.PathEscape(filename+".pdf"))
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
		return
	}
	generateXLSX(c, filename, title, cols, rows)
}

// generateXLSX 生成 xlsx 下载（对齐旧端 generate_xlsx：Sheet1、可选标题行、表头样式、自适应列宽、冻结表头）
func generateXLSX(c *gin.Context, filename, titleText string, cols []xlsxCol, rows []map[string]interface{}) {
	f := excelize.NewFile()
	sheet := "Sheet1"

	thinBorders := []excelize.Border{
		{Type: "left", Style: 1, Color: "000000"}, {Type: "right", Style: 1, Color: "000000"},
		{Type: "top", Style: 1, Color: "000000"}, {Type: "bottom", Style: 1, Color: "000000"},
	}
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 12, Bold: true, Color: "333333"},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 11, Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"366092"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorders,
	})
	dataStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "微软雅黑", Size: 10},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorders,
	})

	row := 1
	if titleText != "" {
		cell, _ := excelize.CoordinatesToCellName(1, row)
		_ = f.SetCellValue(sheet, cell, titleText)
		_ = f.SetCellStyle(sheet, cell, cell, titleStyle)
		if len(cols) > 1 {
			end, _ := excelize.CoordinatesToCellName(len(cols), row)
			_ = f.MergeCell(sheet, cell, end)
		}
		row++
	}
	headerRow := row
	for i, col := range cols {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		_ = f.SetCellValue(sheet, cell, col.Label)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}
	row++
	for _, r := range rows {
		for i, col := range cols {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			_ = f.SetCellValue(sheet, cell, cellValue(r[col.Key]))
			_ = f.SetCellStyle(sheet, cell, cell, dataStyle)
		}
		row++
	}

	// 自适应列宽：min(最大显示宽度 + 4, 50)
	for i, col := range cols {
		maxW := displayWidth(col.Label)
		for _, r := range rows {
			if w := displayWidth(r[col.Key]); w > maxW {
				maxW = w
			}
		}
		w := float64(maxW + 4)
		if w > 50 {
			w = 50
		}
		name, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, name, name, w)
	}

	// 冻结表头行
	_ = f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: headerRow,
		TopLeftCell: fmt.Sprintf("A%d", headerRow+1), ActivePane: "bottomLeft",
	})

	buf, err := f.WriteToBuffer()
	if err != nil {
		serverErr(c, "导出失败")
		return
	}
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename+".xlsx"))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

// splitRoles 逗号分隔角色参数
func splitRoles(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// dateRange 通用日期区间解析
func dateRange(c *gin.Context) (*time.Time, *time.Time, bool) {
	start, err := parseDatePtr(c.Query("start_date"))
	if err != nil {
		badReq(c, "start_date 格式错误")
		return nil, nil, false
	}
	end, err := parseDatePtr(c.Query("end_date"))
	if err != nil {
		badReq(c, "end_date 格式错误")
		return nil, nil, false
	}
	return start, end, true
}

// exportFormat 校验并返回 format 参数（对齐旧端：仅 xlsx/pdf）
func exportFormat(c *gin.Context) (string, bool) {
	format := c.DefaultQuery("format", "xlsx")
	if format != "xlsx" && format != "pdf" {
		badReq(c, "format 参数必须是 xlsx 或 pdf")
		return "", false
	}
	return format, true
}

// exportFields 按 fields 参数过滤列；空或全不匹配回退全列（对齐旧端 GET 导出）
func exportFields(c *gin.Context, all []xlsxCol) []xlsxCol {
	fields := c.QueryArray("fields")
	if len(fields) == 0 {
		return all
	}
	set := map[string]bool{}
	for _, s := range fields {
		set[strings.TrimSpace(s)] = true
	}
	var out []xlsxCol
	for _, col := range all {
		if set[col.Key] {
			out = append(out, col)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// colsByFields 按 fields 顺序组列（对齐旧端 POST /tasks/export：未知字段取自身为标题）
func colsByFields(fields []string, mapping map[string]string, def []string) []xlsxCol {
	if len(fields) == 0 {
		fields = def
	}
	out := make([]xlsxCol, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if lb, ok := mapping[f]; ok {
			out = append(out, xlsxCol{f, lb})
		} else {
			out = append(out, xlsxCol{f, f})
		}
	}
	return out
}

// exportDateSuffix 日期范围文件名后缀（对齐旧端 _build_date_range_suffix）
func exportDateSuffix(start, end string) string {
	s, e := strings.ReplaceAll(start, "-", ""), strings.ReplaceAll(end, "-", "")
	switch {
	case start != "" && end != "":
		return "_" + s + "-" + e
	case start != "":
		return "_" + s + "起"
	case end != "":
		return "_截至" + e
	}
	return ""
}

// exportDateLabel 日期范围标签（对齐旧端 _build_date_range_label）
func exportDateLabel(start, end string) string {
	switch {
	case start != "" && end != "":
		return start + " 至 " + end
	case start != "":
		return start + " 起"
	case end != "":
		return "截至 " + end
	}
	return ""
}

// exportTitle 报表标题（对齐旧端标题拼接规则）
func exportTitle(base, dateLabel string) string {
	out := base
	if dateLabel != "" {
		out += "  |  时间段: " + dateLabel
	}
	out += "  |  导出日期: " + time.Now().Format("2006-01-02")
	return out
}

// strList 兼容字符串 "1,2" 与数组两种 JSON 形态（对齐旧端 pydantic str/list 混合场景）
type strList []string

func (s *strList) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "null" {
		*s = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = splitRoles(v)
	return nil
}

func intSliceOf(list []string) []int {
	var out []int
	for _, s := range list {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ---------- POST /tasks/export ----------

func (h *Task) Export(c *gin.Context) {
	var p struct {
		Status    *int16   `json:"status"`
		CollegeID *int     `json:"college_id"`
		Keyword   string   `json:"keyword"`
		Fields    []string `json:"fields"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		response.FailValidation(c, []map[string]interface{}{{
			"type": "value_error", "loc": []string{"body"}, "msg": err.Error(), "input": nil,
		}})
		return
	}
	u := middleware.CurrentUser(c)

	// 权限范围（对齐旧端：教师=本学院；督导非管理=全校；其余非系统管理员=可管理学院）
	var scope []int // nil 表示全校
	switch {
	case u.HasRole(model.RoleTeacher):
		scope = service.AccessibleCollegeIDs(u)
	case !u.HasAnyRole(model.RoleSystemAdmin) && !service.IsSupervisor(u):
		scope = service.AccessibleCollegeIDs(u)
	}
	if p.CollegeID != nil {
		if scope != nil {
			allowed := false
			for _, id := range scope {
				if id == *p.CollegeID {
					allowed = true
					break
				}
			}
			if !allowed {
				forbidden(c, "无权访问此学院数据")
				return
			}
		}
		scope = []int{*p.CollegeID}
	}

	f := service.TaskFilters{Status: p.Status, Keyword: p.Keyword, CollegeIDs: scope}
	f.Page, f.PageSize = 1, 10000
	tasks, _, err := h.svc.List(h.db, u, f)
	if err != nil {
		serverErr(c, "导出失败")
		return
	}

	// 教师所属学院名称
	tids := make([]int, 0, len(tasks))
	seen := map[int]bool{}
	for _, t := range tasks {
		if !seen[t.TeacherID] {
			seen[t.TeacherID] = true
			tids = append(tids, t.TeacherID)
		}
	}
	collegeNames := map[int]string{}
	if len(tids) > 0 {
		var rows []struct {
			ID          int
			CollegeName *string
		}
		h.db.Table("`user` u").Select("u.id, c.name AS college_name").
			Joins("LEFT JOIN college c ON c.id = u.college_id").
			Where("u.id IN ?", tids).Scan(&rows)
		for _, r := range rows {
			if r.CollegeName != nil {
				collegeNames[r.ID] = *r.CollegeName
			}
		}
	}

	rows := make([]map[string]interface{}, 0, len(tasks))
	for _, t := range tasks {
		classTime, createTime := "", ""
		if t.ClassTime != nil {
			classTime = t.ClassTime.ToTime().Format(timeLayoutFull)
		}
		if t.CreateTime != nil {
			createTime = t.CreateTime.ToTime().Format(timeLayoutFull)
		}
		rows = append(rows, map[string]interface{}{
			"task_id": t.ID, "teacher_name": t.TeacherName, "course_name": t.CourseName,
			"class_time": classTime, "classroom": t.Classroom,
			"college_name":        collegeNames[t.TeacherID],
			"status_name":         model.TaskStatusNames[t.Status],
			"evaluation_count":    t.EvaluationCount,
			"has_supervisor_eval": map[bool]string{true: "是", false: "否"}[t.HasSupervisorEval],
			"create_time":         createTime,
		})
	}

	cols := colsByFields(p.Fields, map[string]string{
		"task_id": "任务ID", "teacher_name": "教师姓名", "course_name": "课程名称",
		"class_time": "上课时间", "classroom": "教室", "college_name": "学院",
		"status_name": "状态", "evaluation_count": "评教次数",
		"has_supervisor_eval": "督导已评", "create_time": "创建时间",
	}, []string{"teacher_name", "course_name", "class_time", "classroom", "college_name", "status_name", "evaluation_count"})

	generateXLSX(c, fmt.Sprintf("评教任务_%s", time.Now().Format("20060102")), "", cols, rows)
}

// ---------- GET /stats/export/teachers ----------

func (h *Stats) ExportTeachers(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	f := service.TeacherStatsFilters{
		CollegeIDs:     splitIntsHandler(c.Query("college_ids")),
		Keyword:        c.Query("keyword"),
		EvaluatorRoles: splitRoles(c.Query("evaluator_roles")),
		Page:           1, PageSize: 10000,
	}
	start, end, ok := dateRange(c)
	if !ok {
		return
	}
	f.Start, f.End = start, end
	u := middleware.CurrentUser(c)
	list, _, err := h.svc.TeacherStats(h.db, u, f)
	if err != nil {
		serverErr(c, "导出失败")
		return
	}
	rows := make([]map[string]interface{}, 0, len(list))
	for _, t := range list {
		college := ""
		if t.CollegeName != nil {
			college = *t.CollegeName
		}
		rows = append(rows, map[string]interface{}{
			"teacher_name": t.TeacherName, "user_no": t.UserNo, "college_name": college,
			"total_tasks": t.TotalTasks, "evaluated_tasks": t.EvaluatedTasks,
			"pending_tasks": t.PendingTasks, "total_evaluations": t.TotalEvaluations,
			"average_score": t.AverageScore, "evaluation_rate": t.EvaluationRate,
		})
	}
	cols := exportFields(c, []xlsxCol{
		{"teacher_name", "教师姓名"}, {"user_no", "工号"}, {"college_name", "学院"},
		{"total_tasks", "总任务数"}, {"evaluated_tasks", "已评任务数"}, {"pending_tasks", "待评任务数"},
		{"total_evaluations", "被评教次数"}, {"average_score", "平均分"}, {"evaluation_rate", "评教完成率(%)"},
	})
	sq, eq := c.Query("start_date"), c.Query("end_date")
	generateExport(c, format, "教师统计"+exportDateSuffix(sq, eq), exportTitle("教师评教统计报表", exportDateLabel(sq, eq)), cols, rows)
}

// ---------- GET /stats/export/colleges ----------

func (h *Stats) ExportColleges(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	f := service.CollegeStatsFilters{
		CollegeIDs: splitIntsHandler(c.Query("college_ids")),
		Semester:   c.Query("semester"),
		Page:       1, PageSize: 1000,
	}
	f.EvaluatorRoles = splitRoles(c.Query("evaluator_roles"))
	start, end, ok := dateRange(c)
	if !ok {
		return
	}
	f.Start, f.End = start, end
	u := middleware.CurrentUser(c)
	list, _, semester, err := h.svc.CollegeStatsList(h.db, u, f)
	if err != nil {
		serverErr(c, "导出失败")
		return
	}
	cols := exportFields(c, []xlsxCol{
		{"college_name", "学院名称"}, {"total_teacher_count", "全部教师数"}, {"teacher_count", "有课教师数"},
		{"total_tasks", "总任务数"}, {"evaluated_tasks", "已评任务数"}, {"pending_tasks", "待评任务数"},
		{"total_evaluations", "评教次数"}, {"average_score", "平均分"}, {"coverage_rate", "评教覆盖率(%)"},
		{"teachers_with_tasks", "被听课教师数"}, {"evaluation_rate", "评教完成率(%)"},
		{"evaluated_teacher_names", "已评教师"}, {"unevaluated_teacher_names", "未评教师"},
	})

	sq, eq := c.Query("start_date"), c.Query("end_date")
	var filename string
	if sq != "" || eq != "" {
		filename = "学院统计" + exportDateSuffix(sq, eq) + "_" + time.Now().Format("20060102")
	} else {
		sem := strings.ReplaceAll(semester, "-", "")
		if sem == "" {
			sem = "unknown"
		}
		filename = "学院统计_" + sem + "_" + time.Now().Format("20060102")
	}
	semesterLabel := semester
	if sq != "" || eq != "" {
		semesterLabel = exportDateLabel(sq, eq)
	}
	if semesterLabel == "" {
		semesterLabel = "未知学期"
	}
	title := "学院评教统计报表  |  时间段: " + semesterLabel + "  |  导出日期: " + time.Now().Format("2006-01-02")
	generateExport(c, format, filename, title, cols, list)
}

// ---------- GET /stats/export/supervisors ----------

func (h *Stats) ExportSupervisors(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	f := service.PersonStatsFilters{
		CollegeIDs: splitIntsHandler(c.Query("college_ids")),
		Keyword:    c.Query("keyword"),
		Page:       1, PageSize: 10000,
	}
	start, end, ok := dateRange(c)
	if !ok {
		return
	}
	f.Start, f.End = start, end
	u := middleware.CurrentUser(c)
	list, _, err := h.svc.SupervisorStats(h.db, u, f)
	if err != nil {
		serverErr(c, "导出失败")
		return
	}
	cols := exportFields(c, []xlsxCol{
		{"supervisor_name", "督导姓名"}, {"user_no", "工号"}, {"college_name", "学院"},
		{"total_evaluations", "评教次数"}, {"average_score", "平均给分"},
		{"evaluated_teacher_count", "评教教师数"}, {"last_evaluation_time", "最近评教时间"},
	})
	sq, eq := c.Query("start_date"), c.Query("end_date")
	generateExport(c, format, "督导评教统计"+exportDateSuffix(sq, eq), exportTitle("督导评教统计报表", exportDateLabel(sq, eq)), cols, list)
}

// ---------- POST /stats/evaluation-records/export ----------

func (h *Stats) ExportEvaluationRecords(c *gin.Context) {
	var p struct {
		CollegeIDs     strList  `json:"college_ids"`
		TeacherID      *int     `json:"teacher_id"`
		Keyword        string   `json:"keyword"`
		EvaluatorRoles strList  `json:"evaluator_roles"`
		StartDate      string   `json:"start_date"`
		EndDate        string   `json:"end_date"`
		Fields         []string `json:"fields"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		response.FailValidation(c, []map[string]interface{}{{
			"type": "value_error", "loc": []string{"body"}, "msg": err.Error(), "input": nil,
		}})
		return
	}
	f := service.RecordStatsFilters{
		CollegeIDs: intSliceOf(p.CollegeIDs), TeacherID: p.TeacherID, Keyword: p.Keyword,
		EvaluatorRoles: []string(p.EvaluatorRoles), Page: 1, PageSize: 10000,
	}
	if p.StartDate != "" {
		t, err := parseDatePtr(p.StartDate)
		if err != nil {
			badReq(c, "start_date 格式错误")
			return
		}
		f.Start = t
	}
	if p.EndDate != "" {
		t, err := parseDatePtr(p.EndDate)
		if err != nil {
			badReq(c, "end_date 格式错误")
			return
		}
		f.End = t
	}
	u := middleware.CurrentUser(c)
	list, _, err := h.svc.EvaluationRecordsStats(h.db, u, f)
	if err != nil {
		serverErr(c, "导出失败")
		return
	}

	fields := p.Fields
	if len(fields) == 0 {
		fields = []string{"teacher_name", "course_name", "class_time", "classroom", "evaluator_name", "submit_time"}
	}
	labelOf := map[string]string{
		"record_id": "记录ID", "teacher_name": "被评教师", "teacher_user_no": "工号",
		"course_name": "课程名称", "class_time": "上课时间", "classroom": "教室",
		"college_name": "所属学院", "campus_name": "校区", "student_count": "上课人数",
		"attendance_count": "出勤人数", "evaluator_name": "评教教师", "evaluator_role": "评教角色",
		"listening_content": "听课内容", "attendance_rate": "出勤率(%)", "total_score": "总评分",
		"max_total_score": "满分", "submit_time": "提交时间",
	}
	cols := make([]xlsxCol, 0, len(fields))
	for _, fd := range fields {
		if lb, ok := labelOf[fd]; ok {
			cols = append(cols, xlsxCol{fd, lb})
		}
	}

	filename := "评教记录" + exportDateSuffix(p.StartDate, p.EndDate) + "_" + time.Now().Format("20060102")
	generateXLSX(c, filename, exportTitle("评教记录报表", exportDateLabel(p.StartDate, p.EndDate)), cols, list)
}

// ---------- GET /stats/export/teacher-evaluation-summary ----------

func (h *Stats) ExportTeacherSummary(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	f := service.SummaryFilters{
		CollegeIDs:      splitIntsHandler(c.Query("college_ids")),
		CampusID:        qInt(c, "campus_id"),
		ResearchRoomIDs: splitIntsHandler(c.Query("research_room_ids")),
		Keyword:         c.Query("keyword"),
		SortBy:          c.Query("sort_by"), SortOrder: c.DefaultQuery("sort_order", "desc"),
		Page: 1, PageSize: 10000,
	}
	f.EvaluatorRoles = splitRoles(c.Query("evaluator_roles"))
	if v := c.Query("has_courses"); v == "true" || v == "1" {
		b := true
		f.HasCourses = &b
	} else if v == "false" || v == "0" {
		b := false
		f.HasCourses = &b
	}
	start, end, ok := dateRange(c)
	if !ok {
		return
	}
	f.Start, f.End = start, end
	u := middleware.CurrentUser(c)
	list, _, err := h.svc.TeacherEvaluationSummary(h.db, u, f)
	if err != nil {
		serverErr(c, "导出失败")
		return
	}
	rows := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		roleNames, _ := item["role_names"].([]string)
		row := map[string]interface{}{}
		for k, v := range item {
			row[k] = v
		}
		row["role_names"] = strings.Join(roleNames, ",")
		rows = append(rows, row)
	}
	cols := exportFields(c, []xlsxCol{
		{"teacher_name", "教师姓名"}, {"user_no", "工号"}, {"college_name", "学院"},
		{"role_names", "角色"}, {"given_count", "评教次数"}, {"given_avg_score", "评教平均给分"},
		{"given_teacher_count", "评教教师数"}, {"received_count", "被评教次数"},
		{"received_avg_score", "被评教平均得分"}, {"received_evaluator_count", "评教人数"},
		{"total_tasks", "总任务数"}, {"evaluated_tasks", "已评任务数"},
		{"pending_tasks", "待评任务数"}, {"evaluation_rate", "评教完成率(%)"},
	})
	sq, eq := c.Query("start_date"), c.Query("end_date")
	generateExport(c, format, "教师评教汇总"+exportDateSuffix(sq, eq), exportTitle("教师评教汇总报表", exportDateLabel(sq, eq)), cols, rows)
}
