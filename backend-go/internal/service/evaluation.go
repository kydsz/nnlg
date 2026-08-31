package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// Evaluation 评教记录业务
type Evaluation struct{}

func NewEvaluation() *Evaluation { return &Evaluation{} }

// SubmitParams 提交评教参数
type SubmitParams struct {
	TaskID          int                    `json:"task_id"`
	DimensionValues map[string]interface{} `json:"dimension_values"`
	IsAnonymous     bool                   `json:"is_anonymous"`
}

// toFloat 数值转换（JSON 数字 / 字符串数字）
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%g", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// fieldConfigNum 从 field_config 取数值配置
func fieldConfigNum(raw json.RawMessage, key string) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0, false
	}
	return toFloat(m[key])
}

// totalScoreOfValues 按启用 score 维度计算记录总分（表无 total_score 列，动态计算）
func totalScoreOfValues(raw json.RawMessage, scoreCodes map[string]bool) *float64 {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]interface{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	total := 0.0
	for code, v := range values {
		if scoreCodes[code] {
			if f, ok := toFloat(v); ok {
				total += f
			}
		}
	}
	return &total
}

// totalMaxScoreOfSchema 启用 score 维度的满分合计（列表/详情展示"满分"用）
func totalMaxScoreOfSchema(db *gorm.DB) float64 {
	var dims []model.EvaluationDimension
	db.Where("status = 1 AND field_type = ?", model.FieldScore).Find(&dims)
	total := 0.0
	for _, d := range dims {
		if max, ok := fieldConfigNum(d.FieldConfig, "max_score"); ok {
			total += max
		}
	}
	return total
}

// TotalScoreOf 单条记录总分（对外用）
func (s *Evaluation) TotalScoreOf(db *gorm.DB, rec *model.EvaluationRecord) *float64 {
	return totalScoreOfValues(rec.DimensionValues, scoreDimCodeSet(db))
}

// Submit 提交评教
func (s *Evaluation) Submit(db *gorm.DB, viewer *model.User, p SubmitParams) (*model.EvaluationRecord, error) {
	if p.TaskID == 0 {
		return nil, errors.New("task_id 为必填")
	}
	if len(p.DimensionValues) == 0 {
		return nil, errors.New("dimension_values 不能为空")
	}

	var task model.EvaluationTask
	if err := db.Where("id = ? AND is_deleted = 0", p.TaskID).First(&task).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if task.Status == model.TaskStatusCancelled {
		return nil, errors.New("任务已取消，不能提交评教")
	}

	// 唯一约束：同一任务同一评教人只能一条（已软删除不再参与重复判断，允许重新提交）
	var dup int64
	db.Model(&model.EvaluationRecord{}).Where("task_id = ? AND evaluator_id = ? AND is_deleted = 0", task.ID, viewer.ID).Count(&dup)
	if dup > 0 {
		return nil, errors.New("您已提交过本次评教")
	}

	// 学院数据范围校验
	teacher, err := LoadTeacherUser(db, task.TeacherID)
	if err != nil {
		return nil, err
	}
	switch {
	case IsAdminRole(viewer) || IsSupervisor(viewer):
		if !CollegeInScope(viewer, teacher.CollegeID) {
			if IsSupervisor(viewer) && !IsAdminRole(viewer) {
				return nil, errors.New("您只能评自己负责学院的教师")
			}
			return nil, errors.New("您没有该学院的评教权限")
		}
	default: // 教师：同行评教（先判自评，再判同学院；对齐旧端）
		if task.TeacherID == viewer.ID {
			return nil, errors.New("不能评自己")
		}
		if teacher.CollegeID == nil || viewer.CollegeID == nil || *teacher.CollegeID != *viewer.CollegeID {
			return nil, errForbidden("只能评同学院的教师")
		}
	}

	// 维度校验
	if _, err := validateDimensionValues(db, p.DimensionValues); err != nil {
		return nil, err
	}

	now := time.Now()
	rec := model.EvaluationRecord{
		TaskID: task.ID, EvaluatorID: IPtr(viewer.ID), EvaluatorName: viewer.Username,
		EvaluatorRole: viewer.Role, IsAnonymous: p.IsAnonymous,
		SubmitTime: model.LocalTimePtr(now),
	}
	raw, err := marshalJSON(p.DimensionValues)
	if err != nil {
		return nil, err
	}
	rec.DimensionValues = raw

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"evaluation_count": gorm.Expr("evaluation_count + 1"),
		}
		if model.IsSupervisorRole(viewer.Role) {
			updates["has_supervisor_eval"] = true
		}
		if task.Status == model.TaskStatusPending {
			updates["status"] = model.TaskStatusEvaluated
		}
		return tx.Model(&model.EvaluationTask{}).Where("id = ?", task.ID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// validateDimensionValues 校验维度值（必填/类型/分值范围），Submit 与 Update 共用
// 返回 score 维度累计分值
func validateDimensionValues(db *gorm.DB, values map[string]interface{}) (float64, error) {
	var dims []model.EvaluationDimension
	if err := db.Where("status = 1").Find(&dims).Error; err != nil {
		return 0, err
	}
	total := 0.0
	for _, d := range dims {
		v, ok := values[d.Code]
		if (!ok || v == nil) && d.IsRequired {
			return 0, fmt.Errorf("维度 %s 为必填项", d.Name)
		}
		if !ok || v == nil {
			continue
		}
		if d.FieldType == model.FieldScore {
			f, valid := toFloat(v)
			if !valid {
				return 0, fmt.Errorf("维度 %s 的分值必须为数字", d.Name)
			}
			max, hasMax := fieldConfigNum(d.FieldConfig, "max_score")
			min, hasMin := fieldConfigNum(d.FieldConfig, "min_score")
			if (hasMax && f > max) || (hasMin && f < min) {
				return 0, fmt.Errorf("维度 %s 分值越界（范围 %g-%g）", d.Name, min, max)
			}
			total += f
		}
	}
	return total, nil
}

// EvaluationFilters 记录查询筛选
type EvaluationFilters struct {
	TaskID         *int
	TeacherID      *int
	EvaluatorID    *int
	Keyword        string
	EvaluatorName  string
	EvaluatorRole  string
	CollegeID      string
	TeacherName    string
	Type           string // received / sent / 空
	Start, End     *time.Time
	Page, PageSize int
}

// canSeeIdentity 是否可查看评教人身份（匿名脱敏规则，需求文档 13.4）
// viewAll 表示拥有"查看他人评教详情"权限（evaluation:view_all），可在数据范围内查看评教人身份
func canSeeIdentity(viewer *model.User, rec *model.EvaluationRecord, task *model.EvaluationTask, teacherCollegeID *int, viewAll bool) bool {
	if rec.EvaluatorID != nil && *rec.EvaluatorID == viewer.ID {
		return true // 评教人本人
	}
	if viewer.HasRole(model.RoleSystemAdmin) {
		return true
	}
	if viewAll && CollegeInScope(viewer, teacherCollegeID) {
		return true
	}
	return false
}

// buildRecordQuery 构造记录查询（t = evaluation_task, r = evaluation_record）
func (s *Evaluation) buildRecordQuery(db *gorm.DB, viewer *model.User, f EvaluationFilters) (*gorm.DB, error) {
	// 记录可见性不随任务软删除消失（对齐旧端：列表/详情均不过滤任务 is_deleted）；
	// 但已软删除的记录本身不展示
	q := db.Table("evaluation_record AS r").
		Joins("JOIN evaluation_task AS t ON t.id = r.task_id").
		Where("r.is_deleted = 0")

	// 类型与数据范围
	isTeacherOnly := viewer.HasRole(model.RoleTeacher) && !IsAdminRole(viewer) && !IsSupervisor(viewer)
	switch {
	case f.Type == "received":
		q = q.Where("t.teacher_id = ?", viewer.ID)
	case f.Type == "sent":
		q = q.Where("r.evaluator_id = ?", viewer.ID)
	case isTeacherOnly:
		q = q.Where("t.teacher_id = ?", viewer.ID)
	default:
		switch {
		case viewer.HasRole(model.RoleSystemAdmin):
			// 系统管理员可查看全部
		case CanViewOthersEvaluation(db, viewer):
			// 被分配"查看他人评教详情"权限的角色：按学院数据范围
			if ids := AccessibleCollegeIDs(viewer); ids != nil {
				if len(ids) == 0 {
					return nil, errors.New("无数据权限")
				}
				q = q.Where("t.teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", ids)
			}
		default:
			// 其余角色（含督导）：仅自己评的 + 评给自己的，不能看别人的评教
			q = q.Where("(r.evaluator_id = ? OR t.teacher_id = ?)", viewer.ID, viewer.ID)
		}
	}

	if isTeacherOnly && f.EvaluatorID != nil && *f.EvaluatorID != viewer.ID {
		return nil, errors.New("教师只能查询自己的评教记录")
	}

	if f.TaskID != nil {
		q = q.Where("r.task_id = ?", *f.TaskID)
	}
	if f.TeacherID != nil {
		q = q.Where("t.teacher_id = ?", *f.TeacherID)
	}
	if f.EvaluatorID != nil {
		q = q.Where("r.evaluator_id = ?", *f.EvaluatorID)
	}
	if f.Keyword != "" {
		kw := "%" + f.Keyword + "%"
		q = q.Where("r.dimension_values LIKE ? OR r.evaluator_name LIKE ?", kw, kw)
	}
	if f.EvaluatorName != "" {
		q = q.Where("r.evaluator_name LIKE ?", "%"+f.EvaluatorName+"%")
	}
	if f.EvaluatorRole != "" {
		q = q.Where("r.evaluator_role = ?", f.EvaluatorRole)
	}
	if f.CollegeID != "" {
		q = q.Where("t.teacher_id IN (SELECT id FROM `user` WHERE college_id IN ?)", splitInts(f.CollegeID))
	}
	if f.TeacherName != "" {
		q = q.Where("t.teacher_name LIKE ?", "%"+f.TeacherName+"%")
	}
	if f.Start != nil {
		q = q.Where("t.class_time >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("t.class_time < ?", *f.End)
	}
	return q, nil
}

// List 记录分页（含匿名脱敏）
func (s *Evaluation) List(db *gorm.DB, viewer *model.User, f EvaluationFilters) ([]map[string]interface{}, int64, error) {
	q, err := s.buildRecordQuery(db, viewer, f)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var recs []model.EvaluationRecord
	err = q.Session(&gorm.Session{}).Select("r.*").
		Order("t.class_time DESC, r.id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Scan(&recs).Error
	if err != nil {
		return nil, 0, err
	}

	items, err := s.decorateRecords(db, viewer, recs, false)
	return items, total, err
}

// decorateRecords 组装记录列表项（含任务信息与脱敏）
func (s *Evaluation) decorateRecords(db *gorm.DB, viewer *model.User, recs []model.EvaluationRecord, withValues bool) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(recs))
	if len(recs) == 0 {
		return out, nil
	}

	taskIDs := map[int]bool{}
	for _, r := range recs {
		taskIDs[r.TaskID] = true
	}
	var tasks []model.EvaluationTask
	db.Where("id IN ?", keysOf(taskIDs)).Find(&tasks)
	taskMap := map[int]model.EvaluationTask{}
	teacherIDs := map[int]bool{}
	for _, t := range tasks {
		taskMap[t.ID] = t
		teacherIDs[t.TeacherID] = true
	}
	// 被评教师所在学院（注意：Scan 在指针字段 + Select 场景会报 unsupported data type，必须用 Find）
	collegeOf := map[int]*int{}
	if len(teacherIDs) > 0 {
		var users []model.User
		db.Select("id, college_id").Where("id IN ?", keysOf(teacherIDs)).Find(&users)
		for _, u := range users {
			c := u.CollegeID
			collegeOf[u.ID] = c
		}
	}
	scoreCodes := scoreDimCodeSet(db)
	maxTotal := totalMaxScoreOfSchema(db)
	viewAll := CanViewOthersEvaluation(db, viewer)

	for i := range recs {
		r := &recs[i]
		task, ok := taskMap[r.TaskID]
		item := map[string]interface{}{
			"id": r.ID, "task_id": r.TaskID, "is_anonymous": r.IsAnonymous,
			"evaluator_role": r.EvaluatorRole, "evaluator_role_name": model.RoleName(r.EvaluatorRole),
			"total_score": totalScoreOfValues(r.DimensionValues, scoreCodes),
			"max_total_score": maxTotal, "submit_time": r.SubmitTime,
		}
		if ok {
			item["teacher_id"] = task.TeacherID
			item["teacher_name"] = task.TeacherName
			item["course_name"] = task.CourseName
			item["class_time"] = task.ClassTime
		}
		if withValues {
			if r.DimensionValues == nil {
				item["dimension_values"] = map[string]interface{}{}
			} else {
				item["dimension_values"] = r.DimensionValues
			}
		}
		if canSeeIdentity(viewer, r, &task, collegeOf[task.TeacherID], viewAll) {
			item["evaluator_id"] = r.EvaluatorID
			item["evaluator_name"] = r.EvaluatorName
		} else {
			item["evaluator_id"] = nil
			item["evaluator_name"] = "匿名"
		}
		out = append(out, item)
	}
	return out, nil
}

// GetDetail 详情（含维度 schema）
func (s *Evaluation) GetDetail(db *gorm.DB, viewer *model.User, id int) (map[string]interface{}, error) {
	var rec model.EvaluationRecord
	if err := db.Where("id = ? AND is_deleted = 0", id).First(&rec).Error; err != nil {
		return nil, errNotFound("评教记录不存在")
	}
	var task model.EvaluationTask
	if err := db.First(&task, rec.TaskID).Error; err != nil {
		return nil, errNotFound("关联任务不存在")
	}

	// 查看权限：仅"自己评的"、"评给自己的"（被评教师）、系统管理员，
	// 或被分配 evaluation:view_all 权限的角色（按学院数据范围）
	isEvaluator := rec.EvaluatorID != nil && *rec.EvaluatorID == viewer.ID
	isSubject := task.TeacherID == viewer.ID
	teacher, _ := LoadTeacherUser(db, task.TeacherID)
	var collegeID *int
	if teacher != nil {
		collegeID = teacher.CollegeID
	}
	if !isEvaluator && !isSubject && !(CanViewOthersEvaluation(db, viewer) && CollegeInScope(viewer, collegeID)) {
		return nil, errors.New("您没有权限查看该评教记录")
	}

	items, err := s.decorateRecords(db, viewer, []model.EvaluationRecord{rec}, true)
	if err != nil || len(items) == 0 {
		return nil, errors.New("评教记录读取失败")
	}
	item := items[0]

	// 展示友好字段：满分合计、学院名、提交时间（空格分隔，对齐导出格式）
	item["max_total_score"] = totalMaxScoreOfSchema(db)
	if teacher != nil {
		if teacher.College != nil {
			item["college_name"] = teacher.College.Name
		} else {
			item["college_name"] = nil
		}
	}
	if rec.SubmitTime != nil {
		item["submit_time"] = rec.SubmitTime.ToTime().Format("2006-01-02 15:04")
	} else {
		item["submit_time"] = "-"
	}

	var dimSvc = NewDimension()
	schema, _ := dimSvc.SchemaForEvaluation(db)
	item["dimension_groups"] = schema
	item["class_time"] = task.ClassTime
	item["classroom"] = task.Classroom
	item["task_status"] = task.Status
	return item, nil
}

// SummariesForTasks 任务列表用的评教摘要（含脱敏与"我是否已评"）
func (s *Evaluation) SummariesForTasks(db *gorm.DB, viewer *model.User, tasks []model.EvaluationTask) (map[int][]map[string]interface{}, map[int]bool) {
	if len(tasks) == 0 {
		return map[int][]map[string]interface{}{}, map[int]bool{}
	}
	ids := make([]int, 0, len(tasks))
	taskMap := map[int]model.EvaluationTask{}
	for _, t := range tasks {
		ids = append(ids, t.ID)
		taskMap[t.ID] = t
	}

	var recs []model.EvaluationRecord
	db.Where("task_id IN ? AND is_deleted = 0", ids).Order("submit_time ASC").Find(&recs)

	teacherIDs := map[int]bool{}
	for _, t := range tasks {
		teacherIDs[t.TeacherID] = true
	}
	collegeOf := map[int]*int{}
	var users []model.User
	db.Select("id, college_id").Where("id IN ?", keysOf(teacherIDs)).Find(&users)
	for _, u := range users {
		c := u.CollegeID
		collegeOf[u.ID] = c
	}

	summaries := map[int][]map[string]interface{}{}
	evaluated := map[int]bool{}
	scoreCodes := scoreDimCodeSet(db)
	// 他人评教可见性：仅系统管理员或被分配 evaluation:view_all 的角色可看别人评的记录；
	// 其他督导/教师只能看到自己评的记录，其余通过任务状态（已评/评教次数）感知
	viewAll := CanViewOthersEvaluation(db, viewer)
	for i := range recs {
		r := &recs[i]
		isEvaluator := r.EvaluatorID != nil && *r.EvaluatorID == viewer.ID
		if isEvaluator {
			evaluated[r.TaskID] = true
		}
		task := taskMap[r.TaskID]
		if !isEvaluator && task.TeacherID != viewer.ID && !viewAll {
			continue // 不可见他人评教，仅保留任务状态
		}
		item := map[string]interface{}{
			"id": r.ID, "evaluator_role": r.EvaluatorRole,
			"evaluator_role_name": model.RoleName(r.EvaluatorRole),
			"total_score":         totalScoreOfValues(r.DimensionValues, scoreCodes), "submit_time": r.SubmitTime,
		}
		if canSeeIdentity(viewer, r, &task, collegeOf[task.TeacherID], viewAll) {
			item["evaluator_id"] = r.EvaluatorID
			item["evaluator_name"] = r.EvaluatorName
		} else {
			item["evaluator_id"] = nil
			item["evaluator_name"] = "匿名"
		}
		summaries[r.TaskID] = append(summaries[r.TaskID], item)
	}
	return summaries, evaluated
}

// keysOf map keys -> slice
func keysOf(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Delete 删除评教记录（软删除：标记 is_deleted，数据保留可恢复）
// 删除会影响接收人（被评教师）侧的列表/统计/汇总，故仅系统管理员或被分配 evaluation:delete 权限的角色可操作
// 返回被删记录快照，供调用方（如操作日志）使用
func (s *Evaluation) Delete(db *gorm.DB, viewer *model.User, id int) (*model.EvaluationRecord, error) {
	var rec model.EvaluationRecord
	if err := db.Where("id = ? AND is_deleted = 0", id).First(&rec).Error; err != nil {
		return nil, errors.New("评教记录不存在")
	}
	if !CanDeleteEvaluation(db, viewer) {
		if !CanDeleteOwnEvaluation(db, viewer) {
			return nil, errors.New("无权删除评教记录")
		}
		if rec.EvaluatorID == nil || *rec.EvaluatorID != viewer.ID {
			return nil, errors.New("只能删除自己提交的评教记录")
		}
	}
	if err := db.Model(&model.EvaluationRecord{}).Where("id = ?", id).Update("is_deleted", true).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateParams 修改评教记录参数（仅维度值与匿名标记可改；评教人/角色/提交时间/任务关联不可改）
type UpdateParams struct {
	DimensionValues map[string]interface{} `json:"dimension_values"`
	IsAnonymous     bool                   `json:"is_anonymous"`
}

// Update 修改评教记录（权限与删除一致：仅系统管理员或被分配 evaluation:delete 权限的角色可操作）
func (s *Evaluation) Update(db *gorm.DB, viewer *model.User, id int, p UpdateParams) (*model.EvaluationRecord, error) {
	var rec model.EvaluationRecord
	if err := db.Where("id = ? AND is_deleted = 0", id).First(&rec).Error; err != nil {
		return nil, errors.New("评教记录不存在")
	}
	if !CanDeleteEvaluation(db, viewer) {
		return nil, errors.New("无权修改评教记录")
	}
	if len(p.DimensionValues) == 0 {
		return nil, errors.New("dimension_values 不能为空")
	}
	if _, err := validateDimensionValues(db, p.DimensionValues); err != nil {
		return nil, err
	}
	raw, err := marshalJSON(p.DimensionValues)
	if err != nil {
		return nil, err
	}
	if err := db.Model(&model.EvaluationRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"dimension_values": raw,
		"is_anonymous":     p.IsAnonymous,
	}).Error; err != nil {
		return nil, err
	}
	rec.DimensionValues = raw
	rec.IsAnonymous = p.IsAnonymous
	return &rec, nil
}

// FileDimMap 文件类维度（image/file）编码 -> 维度
func (s *Evaluation) FileDimMap(db *gorm.DB) map[string]model.EvaluationDimension {
	var dims []model.EvaluationDimension
	db.Where("status = 1 AND field_type IN ?", []string{model.FieldImage, model.FieldFile}).Find(&dims)
	m := map[string]model.EvaluationDimension{}
	for _, d := range dims {
		m[d.Code] = d
	}
	return m
}

// UpdateDimValues 记录创建后回写文件类维度值
func (s *Evaluation) UpdateDimValues(db *gorm.DB, recordID int, values map[string]interface{}) error {
	raw, err := marshalJSON(values)
	if err != nil {
		return err
	}
	return db.Model(&model.EvaluationRecord{}).Where("id = ?", recordID).
		Update("dimension_values", raw).Error
}

// ExportDimension 单个维度导出明细
type ExportDimension struct {
	Code         string      `json:"code"`
	Name         string      `json:"name"`
	GroupName    string      `json:"group_name"`
	GroupCode    string      `json:"group_code"`
	FieldType    string      `json:"field_type"`
	Value        interface{} `json:"value"`
	DisplayValue interface{} `json:"display_value"`
	Score        float64     `json:"score"`
	MaxScore     float64     `json:"max_score"`
}

// ExportGroup 分组汇总
type ExportGroup struct {
	Name       string            `json:"name"`
	Code       string            `json:"code"`
	SortOrder  int               `json:"sort_order"`
	Score      float64           `json:"score"`
	MaxScore   float64           `json:"max_score"`
	Dimensions []ExportDimension `json:"dimensions"`
}

// ExportData 导出单条记录的结构化数据（含脱敏、分组、选项标签映射）
func (s *Evaluation) ExportData(db *gorm.DB, viewer *model.User, id int) (map[string]interface{}, error) {
	var rec model.EvaluationRecord
	if err := db.Where("id = ? AND is_deleted = 0", id).First(&rec).Error; err != nil {
		return nil, errNotFound("评教记录不存在")
	}
	var task model.EvaluationTask
	if err := db.First(&task, rec.TaskID).Error; err != nil {
		return nil, errNotFound("关联任务不存在")
	}

	// 访问权限：仅"自己评的"、"评给自己的"（被评教师）、系统管理员，
	// 或被分配 evaluation:view_all 权限的角色（按学院数据范围）
	teacher, _ := LoadTeacherUser(db, task.TeacherID)
	var teacherCollegeID *int
	if teacher != nil {
		teacherCollegeID = teacher.CollegeID
	}
	isEvaluator := rec.EvaluatorID != nil && *rec.EvaluatorID == viewer.ID
	isSubject := task.TeacherID == viewer.ID
	viewAll := CanViewOthersEvaluation(db, viewer)
	if !isEvaluator && !isSubject && !(viewAll && CollegeInScope(viewer, teacherCollegeID)) {
		return nil, errors.New("无权导出此评教记录")
	}

	// 维度与选项标签映射
	var dims []model.EvaluationDimension
	db.Preload("Group").Where("status = 1").Order("sort_order ASC").Find(&dims)
	dimMap := map[string]model.EvaluationDimension{}
	optionLabel := map[string]string{}
	for _, d := range dims {
		dimMap[d.Code] = d
		var cfg struct {
			Options []struct {
				Value interface{} `json:"value"`
				Label string      `json:"label"`
			} `json:"options"`
			MaxScore float64 `json:"max_score"`
		}
		if len(d.FieldConfig) > 0 {
			if err := json.Unmarshal(d.FieldConfig, &cfg); err == nil {
				for _, o := range cfg.Options {
					optionLabel[d.Code+":"+fmt.Sprintf("%v", o.Value)] = o.Label
				}
			}
		}
	}

	// 解析记录值
	var values map[string]interface{}
	if len(rec.DimensionValues) > 0 {
		_ = json.Unmarshal(rec.DimensionValues, &values)
	}

	groups := map[string]*ExportGroup{}
	var details []ExportDimension
	totalScore, totalMax := 0.0, 0.0
	for code, value := range values {
		d, ok := dimMap[code]
		if !ok {
			continue
		}
		gName, gCode := "其他", "other"
		if d.Group != nil {
			gName, gCode = d.Group.Name, d.Group.Code
		}
		detail := ExportDimension{
			Code: code, Name: d.Name, GroupName: gName, GroupCode: gCode,
			FieldType: d.FieldType, Value: value, DisplayValue: value,
		}
		if d.FieldType == model.FieldSingleChoice || d.FieldType == model.FieldMultipleChoce {
			if list, isList := value.([]interface{}); isList {
				labels := make([]string, 0, len(list))
				for _, v := range list {
					if l, has := optionLabel[code+":"+fmt.Sprintf("%v", v)]; has {
						labels = append(labels, l)
					} else {
						labels = append(labels, fmt.Sprintf("%v", v))
					}
				}
				detail.DisplayValue = strings.Join(labels, ", ")
			} else if l, has := optionLabel[code+":"+fmt.Sprintf("%v", value)]; has {
				detail.DisplayValue = l
			}
		}
		if d.FieldType == model.FieldScore {
			if f, ok := toFloat(value); ok {
				detail.Score = f
				var cfg struct {
					MaxScore float64 `json:"max_score"`
				}
				_ = json.Unmarshal(d.FieldConfig, &cfg)
				detail.MaxScore = cfg.MaxScore
				totalScore += f
				totalMax += cfg.MaxScore
			}
		}
		details = append(details, detail)
	}
	sort.Slice(details, func(i, j int) bool {
		gi, gj := groupOrderIndex(details[i].GroupCode), groupOrderIndex(details[j].GroupCode)
		if gi != gj {
			return gi < gj
		}
		return details[i].Code < details[j].Code
	})
	for _, d := range details {
		g, ok := groups[d.GroupCode]
		if !ok {
			g = &ExportGroup{Name: d.GroupName, Code: d.GroupCode, SortOrder: groupOrderIndex(d.GroupCode)}
			groups[d.GroupCode] = g
		}
		g.Dimensions = append(g.Dimensions, d)
		if d.FieldType == model.FieldScore {
			g.Score += d.Score
			g.MaxScore += d.MaxScore
		}
	}

	groupOrder := []string{"attendance", "evaluation_option", "attitude", "content", "method", "effect", "comment"}
	groupList := make([]ExportGroup, 0, len(groups))
	for _, code := range groupOrder {
		if g, ok := groups[code]; ok {
			groupList = append(groupList, *g)
		}
	}
	for code, g := range groups {
		if groupOrderIndex(code) == 999 {
			groupList = append(groupList, *g)
		}
	}

	// 评教人显示（脱敏）
	collegeName := interface{}(nil)
	if teacher != nil && teacher.College != nil {
		collegeName = teacher.College.Name
	}
	evaluatorName := interface{}(nil)
	if canSeeIdentity(viewer, &rec, &task, teacherCollegeID, viewAll) || (!rec.IsAnonymous && !viewer.HasRole(model.RoleTeacher)) || isEvaluator {
		evaluatorName = rec.EvaluatorName
	} else {
		evaluatorName = "匿名"
	}

	submitTime := "-"
	if rec.SubmitTime != nil {
		submitTime = rec.SubmitTime.ToTime().Format("2006-01-02 15:04")
	}
	classTime := "-"
	if task.ClassTime != nil {
		classTime = task.ClassTime.ToTime().Format("2006-01-02 15:04")
	}
	return map[string]interface{}{
		"id": rec.ID, "course_name": task.CourseName, "teacher_name": task.TeacherName,
		"college_name": collegeName, "semester": nil,
		"class_time": classTime,
		"evaluator_name":      evaluatorName,
		"evaluator_role_name": model.RoleName(rec.EvaluatorRole),
		"submit_time":         submitTime, "is_anonymous": rec.IsAnonymous,
		"total_score": totalScore, "max_total_score": totalMax,
		"dimension_groups": groupList,
	}, nil
}

// groupOrderIndex 导出分组的固定排序
func groupOrderIndex(code string) int {
	for i, c := range []string{"attendance", "evaluation_option", "attitude", "content", "method", "effect", "comment"} {
		if c == code {
			return i
		}
	}
	return 999
}
