package handler

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/cache"
	"backend-go/internal/config"
	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/pdfgen"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Evaluation 评教记录接口
type Evaluation struct {
	db        *gorm.DB
	svc       *service.Evaluation
	uploadDir string
}

func NewEvaluationHandler(cfg *config.Config, db *gorm.DB) *Evaluation {
	return &Evaluation{db: db, svc: service.NewEvaluation(), uploadDir: cfg.UploadDir}
}

// dupKeyOf 提交幂等键（key=task:user）
func dupKeyOf(taskID, userID int) string {
	return fmt.Sprintf("tev:dup:%d:%d", taskID, userID)
}

// releaseDupKey 释放提交幂等键（Redis 未启用时静默）。提交未成功落库时调用，
// 允许用户立即重试，而不是被 30 分钟 TTL 锁定。
func releaseDupKey(c *gin.Context, taskID, userID int) {
	if rdb := cache.GetClient(); rdb != nil && rdb.Enabled {
		rdb.Del(c.Request.Context(), dupKeyOf(taskID, userID))
	}
}

// Submit 提交评教
// 高并发场景：先做同步校验，再入 Redis Stream 异步落库，秒级返回成功。
// Redis 不可用（未配置或异常）时自动降级为原同步落库，保证功能不中断。
func (h *Evaluation) Submit(c *gin.Context) {
	var p service.SubmitParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)

	rdb := cache.GetClient()
	if rdb != nil && rdb.Enabled {
		// 异步路径：同步校验 + Redis 幂等 + 入队秒回
		if _, _, err := h.svc.ValidateSubmit(h.db, u, p); err != nil {
			badReq(c, err.Error())
			return
		}
		// 幂等判重：Redis SETNX（key=task:user），替代 DB Count，防并发重复提交
		dupKey := fmt.Sprintf("tev:dup:%d:%d", p.TaskID, u.ID)
		ok, err := rdb.SetNX(c.Request.Context(), dupKey, 30*time.Minute)
		if err != nil {
			// Redis 异常：删除可能的残留并降级同步
			log.Printf("[evaluation] Redis 幂等异常，降级同步提交: %v", err)
			rec, serr := h.svc.Submit(h.db, u, p)
			if serr != nil {
				badReq(c, serr.Error())
				return
			}
			h.logSubmit(c, u, rec, false)
			response.OKMsg(c, "提交成功", gin.H{
				"id": rec.ID, "task_id": rec.TaskID, "submit_time": FTime(rec.SubmitTime),
				"total_score": h.svc.TotalScoreOf(h.db, rec),
			})
			return
		}
		if !ok {
			badReq(c, "您已提交过本次评教")
			return
		}
		// 入队
		raw, _ := json.Marshal(p.DimensionValues)
		_, err = rdb.Publish(c.Request.Context(), rdb.StreamName, map[string]interface{}{
			"task_id":          p.TaskID,
			"user_id":          u.ID,
			"is_anonymous":     p.IsAnonymous,
			"dimension_values": string(raw),
		})
		if err != nil {
			// 入队失败：释放幂等键并降级同步提交
			rdb.Del(c.Request.Context(), dupKey)
			log.Printf("[evaluation] 入队失败，降级同步提交: %v", err)
			rec, serr := h.svc.Submit(h.db, u, p)
			if serr != nil {
				badReq(c, serr.Error())
				return
			}
			h.logSubmit(c, u, rec, false)
			response.OKMsg(c, "提交成功", gin.H{
				"id": rec.ID, "task_id": rec.TaskID, "submit_time": FTime(rec.SubmitTime),
				"total_score": h.svc.TotalScoreOf(h.db, rec),
			})
			return
		}
		// 秒回成功（异步落库，数据稍后可见；id/total_score 待落库后由列表接口回填，此处占位对齐结构）
		response.OKMsg(c, "提交成功", gin.H{
			"id": nil, "task_id": p.TaskID, "submit_time": FTime(model.LocalTimePtr(time.Now())),
			"total_score": nil, "queued": true,
		})
		return
	}

	// 同步路径（Redis 未配置）：原逻辑
	rec, err := h.svc.Submit(h.db, u, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	h.logSubmit(c, u, rec, false)
	response.OKMsg(c, "提交成功", gin.H{
		"id": rec.ID, "task_id": rec.TaskID, "submit_time": FTime(rec.SubmitTime),
		"total_score": h.svc.TotalScoreOf(h.db, rec),
	})
}

// logSubmit 记录提交操作日志（同步/异步共用）
func (h *Evaluation) logSubmit(c *gin.Context, u *model.User, rec *model.EvaluationRecord, withFiles bool) {
	content := map[string]interface{}{
		"task_id": rec.TaskID, "is_anonymous": rec.IsAnonymous,
		"total_score": h.svc.TotalScoreOf(h.db, rec),
	}
	if withFiles {
		content["with_files"] = true
	}
	service.LogRecord(h.db, &u.ID, u.Username, "evaluation_submit", "evaluation", &rec.ID, "evaluation_record", content)
}

// List 记录分页
func (h *Evaluation) List(c *gin.Context) {
	page, pageSize := pageOf(c)
	f := service.EvaluationFilters{
		TaskID: qInt(c, "task_id"), TeacherID: qInt(c, "teacher_id"), EvaluatorID: qInt(c, "evaluator_id"),
		Keyword: c.Query("keyword"), EvaluatorName: c.Query("evaluator_name"),
		EvaluatorRole: c.Query("evaluator_role"), CollegeID: c.Query("college_id"),
		TeacherName: c.Query("teacher_name"), Type: c.Query("type"),
		OrderBy: c.Query("order_by"), OrderDir: c.Query("order"),
		Page: page, PageSize: pageSize,
	}
	var err error
	if f.Start, err = parseDatePtr(c.Query("start_date")); err != nil {
		badReq(c, "start_date 格式错误")
		return
	}
	if f.End, err = parseDatePtr(c.Query("end_date")); err != nil {
		badReq(c, "end_date 格式错误")
		return
	}
	if f.Start != nil && f.End != nil && f.Start.After(*f.End) {
		badReq(c, "开始日期不能晚于结束日期")
		return
	}
	u := middleware.CurrentUser(c)
	list, total, err := h.svc.List(h.db, u, f)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// GetMyDraft 查询我的评教草稿（暂存进度恢复用）
func (h *Evaluation) GetMyDraft(c *gin.Context) {
	taskID := qInt(c, "task_id")
	if taskID == nil {
		badReq(c, "task_id 为必填")
		return
	}
	u := middleware.CurrentUser(c)
	draft, err := h.svc.GetMyDraft(h.db, u, *taskID)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	var draftOrNil interface{}
	if draft != nil {
		values := map[string]interface{}{}
		if len(draft.DimensionValues) > 0 {
			_ = json.Unmarshal(draft.DimensionValues, &values)
		}
		draftOrNil = gin.H{
			"id": draft.ID, "task_id": draft.TaskID, "is_anonymous": draft.IsAnonymous,
			"evaluator_id": draft.EvaluatorID, "evaluator_name": draft.EvaluatorName,
			"update_time": FTime(draft.UpdateTime), "dimension_values": values,
		}
	}
	response.OK(c, gin.H{"draft": draftOrNil})
}

// SaveMyDraft upsert 保存评教草稿
func (h *Evaluation) SaveMyDraft(c *gin.Context) {
	var p service.SaveDraftParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	if p.TaskID == 0 {
		badReq(c, "task_id 为必填")
		return
	}
	u := middleware.CurrentUser(c)
	draft, err := h.svc.SaveDraft(h.db, u, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	response.OKMsg(c, "暂存成功", gin.H{
		"id": draft.ID, "task_id": draft.TaskID, "update_time": FTime(draft.UpdateTime),
	})
}

// DeleteMyDraft 丢弃我的评教草稿
func (h *Evaluation) DeleteMyDraft(c *gin.Context) {
	taskID := qInt(c, "task_id")
	if taskID == nil {
		badReq(c, "task_id 为必填")
		return
	}
	u := middleware.CurrentUser(c)
	if err := h.svc.DeleteMyDraft(h.db, u, *taskID); err != nil {
		badReq(c, err.Error())
		return
	}
	response.OKMsg(c, "已删除", nil)
}

// Detail 记录详情
func (h *Evaluation) Detail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	u := middleware.CurrentUser(c)
	item, err := h.svc.GetDetail(h.db, u, id)
	if err != nil {
		svcErr(c, err) // 404（不存在）/ 403（无权限）
		return
	}
	response.OK(c, item)
}

// SubmitWithFiles 提交评教（multipart，支持文件类维度附件）
func (h *Evaluation) SubmitWithFiles(c *gin.Context) {
	taskID, err := strconv.Atoi(c.PostForm("task_id"))
	if err != nil || taskID == 0 {
		badReq(c, "task_id 为必填")
		return
	}
	var values map[string]interface{}
	if err := json.Unmarshal([]byte(c.PostForm("dimension_values")), &values); err != nil {
		badReq(c, "dimension_values 格式错误，必须是有效的JSON字符串")
		return
	}
	isAnonymous := c.PostForm("is_anonymous") == "true"

	form, err := c.MultipartForm()
	if err != nil {
		badReq(c, "multipart 解析失败")
		return
	}
	files := form.File["files"]

	// 按文件名前缀 {dim_code}_{original} 分组到文件类维度
	fileDims := h.svc.FileDimMap(h.db)
	byDim := map[string][]*multipart.FileHeader{}
	for _, fh := range files {
		parts := strings.SplitN(fh.Filename, "_", 2)
		if len(parts) < 2 {
			continue
		}
		if d, ok := fileDims[parts[0]]; ok {
			isImage := d.FieldType == model.FieldImage
			if err := validateUpload(fh, isImage); err != nil {
				badReq(c, err.Error())
				return
			}
			byDim[parts[0]] = append(byDim[parts[0]], fh)
		}
	}
	// 数量上限校验（先于建记录）
	for code, list := range byDim {
		d := fileDims[code]
		var cfg struct {
			MaxCount int `json:"max_count"`
		}
		_ = json.Unmarshal(d.FieldConfig, &cfg)
		maxCount := cfg.MaxCount
		if maxCount == 0 {
			if d.FieldType == model.FieldImage {
				maxCount = 9
			} else {
				maxCount = 5
			}
		}
		if len(list) > maxCount {
			badReq(c, fmt.Sprintf("维度 '%s' 的文件数量超过限制: %d > %d", d.Name, len(list), maxCount))
			return
		}
	}

	u := middleware.CurrentUser(c)
	// Redis 幂等防重复（带文件提交，key=task:user）；Redis 不可用时跳过，靠 DB 判重兜底
	if rdb := cache.GetClient(); rdb != nil && rdb.Enabled {
		dupKey := fmt.Sprintf("tev:dup:%d:%d", taskID, u.ID)
		ok, err := rdb.SetNX(c.Request.Context(), dupKey, 30*time.Minute)
		if err != nil {
			log.Printf("[evaluation] 带文件提交 Redis 幂等异常: %v", err)
		} else if !ok {
			badReq(c, "您已提交过本次评教")
			return
		}
	}
	rec, err := h.svc.Submit(h.db, u, service.SubmitParams{
		TaskID: taskID, DimensionValues: values, IsAnonymous: isAnonymous,
	})
	if err != nil {
		// 落库失败：释放幂等键，允许用户立即重试（而不是被锁 30 分钟）
		releaseDupKey(c, taskID, u.ID)
		badReq(c, err.Error())
		return
	}

	// 保存文件并回写 dimension_values
	baseDir := h.uploadDir
	uploaded := map[string]interface{}{}
	for code, list := range byDim {
		dir := filepath.Join(baseDir, "evaluations", strconv.Itoa(rec.ID), code)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		urls := []string{}
		for _, fh := range list {
			name := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), sanitizeName(fh.Filename))
			if err := saveUploaded(fh, filepath.Join(dir, name)); err != nil {
				continue
			}
			rel := "evaluations/" + strconv.Itoa(rec.ID) + "/" + code + "/" + name
			urls = append(urls, "/api/v1/files/"+rel)
		}
		if len(urls) > 0 {
			uploaded[code] = urls
		}
	}
	if len(uploaded) > 0 {
		for code, urls := range uploaded {
			values[code] = urls
		}
		if err := h.svc.UpdateDimValues(h.db, rec.ID, values); err != nil {
			// 附件信息回写失败：释放幂等键，允许用户删除记录后重新提交（而非锁 30 分钟）
			releaseDupKey(c, taskID, u.ID)
			serverErr(c, "附件信息保存失败")
			return
		}
	}

	service.LogRecord(h.db, &u.ID, u.Username, "evaluation_submit", "evaluation", &rec.ID, "evaluation_record",
		map[string]interface{}{"task_id": rec.TaskID, "with_files": true, "file_count": len(files)})
	response.OKMsg(c, "提交成功", gin.H{
		"id": rec.ID, "task_id": rec.TaskID, "submit_time": FTime(rec.SubmitTime),
		"total_score": h.svc.TotalScoreOf(h.db, rec), "files": uploaded,
	})
}

// Delete 删除评教记录
func (h *Evaluation) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的记录 ID")
		return
	}
	u := middleware.CurrentUser(c)
	rec, err := h.svc.Delete(h.db, u, id)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	// 操作日志：记录被删评教的教师/课程/评教人等上下文，便于审计追查
	content := map[string]interface{}{
		"task_id":        rec.TaskID,
		"evaluator_name": rec.EvaluatorName,
		"is_anonymous":   rec.IsAnonymous,
	}
	if rec.SubmitTime != nil {
		content["submit_time"] = rec.SubmitTime
	}
	var t model.EvaluationTask
	if err := h.db.Where("id = ?", rec.TaskID).First(&t).Error; err == nil {
		content["teacher_id"] = t.TeacherID
		content["teacher_name"] = t.TeacherName
		content["course_name"] = t.CourseName
	}
	service.LogRecord(h.db, &u.ID, u.Username, "delete", "evaluation", &rec.ID, "evaluation_record", content)
	response.OKMsg(c, "删除成功", nil)
}

// Update 修改评教记录（仅系统管理员或被分配 evaluation:delete 权限的角色；仅维度值与匿名标记可改）
func (h *Evaluation) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的记录 ID")
		return
	}
	var p service.UpdateParams
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)
	rec, err := h.svc.Update(h.db, u, id, p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	var t model.EvaluationTask
	content := map[string]interface{}{"is_anonymous": rec.IsAnonymous}
	if err := h.db.Where("id = ?", rec.TaskID).First(&t).Error; err == nil {
		content["teacher_id"] = t.TeacherID
		content["teacher_name"] = t.TeacherName
		content["course_name"] = t.CourseName
	}
	service.LogRecord(h.db, &u.ID, u.Username, "update", "evaluation", &rec.ID, "evaluation_record", content)
	response.OKMsg(c, "更新成功", gin.H{
		"id":           rec.ID,
		"is_anonymous": rec.IsAnonymous,
		"total_score":  h.svc.TotalScoreOf(h.db, rec),
	})
}

// Export 单条记录导出：format=pdf 返回 PDF 文件；默认返回可打印 HTML
func (h *Evaluation) Export(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		badReq(c, "无效的记录 ID")
		return
	}
	u := middleware.CurrentUser(c)
	data, err := h.svc.ExportData(h.db, u, id)
	if err != nil {
		response.Fail(c, 403, err.Error())
		return
	}

	// format=pdf 生成真 PDF（对齐旧端 Playwright HTML→PDF 导出）；
	// 不传 format 返回可打印 HTML（移动端打印窗口链路）
	if c.Query("format") == "pdf" {
		pdfBytes, err := renderEvaluationPDF(data, h.uploadDir)
		if err != nil {
			serverErr(c, "PDF 生成失败: "+err.Error())
			return
		}
		filename := fmt.Sprintf("evaluation_%v_%v.pdf", data["id"], data["course_name"])
		c.Header("Content-Disposition",
			"attachment; filename*=UTF-8''"+url.PathEscape(filename))
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
		return
	}

	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="evaluation_%d.html"`, id))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderEvaluationHTML(data)))
}

// renderEvaluationPDF 组装评教详情 PDF（布局对齐旧端 generate_evaluation_html 模板）
func renderEvaluationPDF(data map[string]interface{}, uploadDir string) ([]byte, error) {
	groups, _ := data["dimension_groups"].([]service.ExportGroup)
	detail := pdfgen.EvalDetail{
		CourseName:    strOrEmpty(data["course_name"]),
		TeacherName:   strOrEmpty(data["teacher_name"]),
		CollegeName:   strOrEmpty(data["college_name"]),
		EvaluatorName: strOrEmpty(data["evaluator_name"]),
		EvaluatorRole: strOrEmpty(data["evaluator_role_name"]),
		ClassTime:     strOrEmpty(data["class_time"]),
		Submit:        strOrEmpty(data["submit_time"]),
		TotalScore:    toFloat64Val(data["total_score"]),
		MaxTotalScore: toFloat64Val(data["max_total_score"]),
	}
	if v, ok := data["is_anonymous"].(bool); ok {
		detail.IsAnonymous = v
	}
	// 课表信息（与提交页一致）
	if sch, ok := data["schedule"].(map[string]interface{}); ok {
		detail.ScheduleRows = buildScheduleRows(sch)
	}
	for _, g := range groups {
		og := pdfgen.EvalDetailGroup{Name: g.Name, Score: g.Score, MaxScore: g.MaxScore}
		for _, dim := range g.Dimensions {
			og.Dimensions = append(og.Dimensions, pdfgen.EvalDetailDim{
				Name: dim.Name, FieldType: dim.FieldType,
				Value: dim.Value, Display: fmt.Sprint(dim.DisplayValue),
				Score: dim.Score, MaxScore: dim.MaxScore,
			})
		}
		detail.Groups = append(detail.Groups, og)
	}
	return pdfgen.RenderEvaluationPDF(detail, uploadDir)
}

// buildScheduleRows 课表信息 → PDF 行。解引用指针字段（如历史数据里的 *int 学生数），
// 避免 fmt.Sprint 把指针渲染成 0x... 内存地址（线上症状「应到人数0x...人」）
func buildScheduleRows(sch map[string]interface{}) []pdfgen.EvalInfoRow {
	rows := []struct {
		label string
		val   interface{}
		unit  string
	}{
		{"上课时间", sch["class_time_text"], ""},
		{"教室", sch["classroom"], ""},
		{"班级", sch["class_info"], ""},
		{"应到人数", sch["student_count"], "人"},
		{"周次", sch["week_pattern"], ""},
	}
	out := make([]pdfgen.EvalInfoRow, 0, len(rows))
	for _, row := range rows {
		val := row.val
		if u, ok := pdfgen.DerefValue(val); ok {
			val = u
		}
		if val == nil {
			out = append(out, pdfgen.EvalInfoRow{Label: row.label, Value: "-"})
			continue
		}
		out = append(out, pdfgen.EvalInfoRow{Label: row.label, Value: fmt.Sprint(val) + row.unit})
	}
	return out
}

func strOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func toFloat64Val(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// renderEvaluationHTML 生成可打印的评教记录 HTML
func renderEvaluationHTML(d map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8">
<title>评教记录导出</title><style>
body{font-family:"Microsoft YaHei",sans-serif;margin:24px;color:#222}
h2{color:#1a3c6e;border-bottom:2px solid #1a3c6e;padding-bottom:6px}
table{border-collapse:collapse;width:100%;margin:12px 0}
th,td{border:1px solid #bbb;padding:6px 10px;text-align:left;font-size:14px}
th{background:#eef3fa}.meta td:first-child{width:120px;background:#f7f9fc;font-weight:bold}
.total{font-weight:bold;color:#1a3c6e}
@media print{body{margin:0}}
</style></head><body>`)

	fmt.Fprintf(&b, "<h2>听课评教记录 #%v</h2>", d["id"])
	b.WriteString(`<table class="meta">`)
	for _, row := range []struct{ k, v string }{
		{"课程名称", fmt.Sprint(d["course_name"])},
		{"上课时间", fmt.Sprint(d["class_time"])},
		{"被评教师", fmt.Sprint(d["teacher_name"])},
		{"所属学院", fmt.Sprint(d["college_name"])},
		{"评教人", fmt.Sprint(d["evaluator_name"])},
		{"评教人角色", fmt.Sprint(d["evaluator_role_name"])},
		{"提交时间", fmt.Sprint(d["submit_time"])},
	} {
		if row.v == "<nil>" {
			row.v = "-"
		}
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td></tr>", row.k, html.EscapeString(row.v))
	}
	b.WriteString(`</table>`)

	// 课表信息（与提交页一致：上课时间/教室/班级/应到人数/周次）
	if sch, ok := d["schedule"].(map[string]interface{}); ok {
		b.WriteString(`<table class="meta">`)
		rows := []struct {
			k string
			v interface{}
		}{
			{"上课时间", sch["class_time_text"]},
			{"教室", sch["classroom"]},
			{"班级", sch["class_info"]},
			{"应到人数", sch["student_count"]},
			{"周次", sch["week_pattern"]},
		}
		for _, row := range rows {
			val := "-"
			if row.v != nil {
				val = fmt.Sprint(row.v)
				if row.k == "应到人数" {
					val = val + "人"
				}
			}
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td></tr>", row.k, html.EscapeString(val))
		}
		b.WriteString(`</table>`)
	}

	if groups, ok := d["dimension_groups"].([]service.ExportGroup); ok {
		for _, g := range groups {
			fmt.Fprintf(&b, "<h3>%s", html.EscapeString(g.Name))
			if g.MaxScore > 0 {
				fmt.Fprintf(&b, `（%g / %g 分）`, g.Score, g.MaxScore)
			}
			b.WriteString("</h3><table><tr><th>维度</th><th>评价</th><th>得分</th></tr>")
			for _, dim := range g.Dimensions {
				val := fmt.Sprint(dim.DisplayValue)
				if val == "<nil>" {
					val = "-"
				}
				score := "-"
				if dim.FieldType == model.FieldScore {
					score = fmt.Sprintf("%g / %g", dim.Score, dim.MaxScore)
				}
				fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>",
					html.EscapeString(dim.Name), html.EscapeString(val), score)
			}
			b.WriteString("</table>")
		}
	}
	fmt.Fprintf(&b, `<p class="total">总分：%v / %v</p></body></html>`,
		d["total_score"], d["max_total_score"])
	return b.String()
}
