package handler

import (
	"encoding/json"
	"fmt"
	"strconv"

	"backend-go/internal/middleware"
	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Task 评教任务接口
type Task struct {
	db      *gorm.DB
	svc     *service.Task
	evalSvc *service.Evaluation
}

func NewTask(db *gorm.DB) *Task {
	return &Task{db: db, svc: service.NewTask(), evalSvc: service.NewEvaluation()}
}

// List 任务分页列表
func (h *Task) List(c *gin.Context) {
	page, pageSize := pageOf(c)
	f := service.TaskFilters{
		Keyword: c.Query("keyword"), Status: qInt16(c, "status"), TeacherID: qInt(c, "teacher_id"),
		HasSupervisorEval: qBool(c, "has_supervisor_eval"), CreateBy: qInt(c, "create_by"),
		CreateByNot: qInt(c, "create_by_not"), Page: page, PageSize: pageSize,
		OrderBy: c.Query("order_by"), OrderDir: c.Query("order"),
	}
	if v := c.Query("college_id"); v != "" {
		f.CollegeIDs = splitIntsHandler(v)
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

	tasks, total, err := h.svc.List(h.db, middleware.CurrentUser(c), f)
	if err != nil {
		serverErr(c, "查询失败")
		return
	}

	u := middleware.CurrentUser(c)
	summaries, evaluated := h.evalSvc.SummariesForTasks(h.db, u, tasks)

	// 收集本页任务下当前用户是否有草稿，便于列表展示"暂存中"标记
	draftSet := map[int]bool{}
	{
		ids := make([]int, 0, len(tasks))
		for _, t := range tasks {
			ids = append(ids, t.ID)
		}
		if len(ids) > 0 {
			var draftTaskIDs []int
			h.db.Model(&model.EvaluationDraft{}).
				Where("task_id IN ? AND evaluator_id = ?", ids, u.ID).
				Pluck("task_id", &draftTaskIDs)
			for _, tid := range draftTaskIDs {
				draftSet[tid] = true
			}
		}
	}

	// 创建者姓名
	creatorNames := map[int]string{}
	ids := map[int]bool{}
	for _, t := range tasks {
		if t.CreateBy != nil {
			ids[*t.CreateBy] = true
		}
	}
	if len(ids) > 0 {
		var users []model.User
		// 注意：必须用 Find 而非 Scan——model.User 含指针字段，Scan + Select 组合会报 unsupported data type
		h.db.Select("id, username").Where("id IN ?", keysOf(ids)).Find(&users)
		for _, uu := range users {
			creatorNames[uu.ID] = uu.Username
		}
	}

	list := []gin.H{}
	for i := range tasks {
		t := &tasks[i]
		summary := summaries[t.ID]
		if summary == nil {
			summary = []map[string]interface{}{}
		}
		var createByName interface{}
		if t.CreateBy != nil {
			if n, ok := creatorNames[*t.CreateBy]; ok {
				createByName = n
			}
		}
		list = append(list, gin.H{
			"id": t.ID, "teacher_id": t.TeacherID, "teacher_name": t.TeacherName,
			"course_name": t.CourseName, "class_time": FTimeMin(t.ClassTime),
			"classroom": t.Classroom, "status": t.Status,
			"status_name": model.TaskStatusNames[t.Status],
			"start_time":  FTime(t.StartTime), "end_time": FTime(t.EndTime),
			"evaluation_count": t.EvaluationCount, "has_supervisor_eval": t.HasSupervisorEval,
			"create_by": t.CreateBy, "create_by_name": createByName,
			"create_time":            FTime(t.CreateTime),
			"current_user_evaluated": evaluated[t.ID],
			"has_draft":              draftSet[t.ID],
			"evaluation_records":     summary,
		})
	}

	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// Get 任务详情（字段对齐旧端 GET /tasks/{id}）
func (h *Task) Get(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	t, err := h.svc.Get(h.db, id)
	if err != nil {
		response.Fail(c, 404, err.Error())
		return
	}
	u := middleware.CurrentUser(c)
	summaries, _ := h.evalSvc.SummariesForTasks(h.db, u, []model.EvaluationTask{*t})
	summary := summaries[t.ID]
	if summary == nil {
		summary = []map[string]interface{}{}
	}
	teacher, _ := service.LoadTeacherUser(h.db, t.TeacherID)
	var teacherCollegeID interface{}
	var teacherCollegeName interface{}
	if teacher != nil {
		teacherCollegeID = teacher.CollegeID
		if teacher.College != nil {
			teacherCollegeName = teacher.College.Name
		}
	}
	var createByName interface{}
	if t.CreateBy != nil {
		var creator model.User
		if err := h.db.Select("id, username").First(&creator, *t.CreateBy).Error; err == nil {
			createByName = creator.Username
		}
	}
	response.OK(c, gin.H{
		"id": t.ID, "teacher_id": t.TeacherID, "teacher_name": t.TeacherName,
		"teacher_college_id": teacherCollegeID, "teacher_college_name": teacherCollegeName,
		"course_name": t.CourseName, "class_time": FTimeMin(t.ClassTime),
		"classroom": t.Classroom, "schedule": taskScheduleSnapshot(t), "status": t.Status,
		"status_name":      model.TaskStatusNames[t.Status],
		"evaluation_count": t.EvaluationCount, "has_supervisor_eval": t.HasSupervisorEval,
		"create_by": t.CreateBy, "create_by_name": createByName,
		"create_time": FTime(t.CreateTime), "update_time": FTime(t.UpdateTime),
		"evaluation_records": summary,
	})
}

type taskReq struct {
	TeacherID   int              `json:"teacher_id"`
	TeacherName string           `json:"teacher_name"`
	CourseName  string           `json:"course_name"`
	ClassTime   *model.LocalTime `json:"class_time"`
	Classroom   *string          `json:"classroom"`
	StartTime   *model.LocalTime `json:"start_time"`
	EndTime     *model.LocalTime `json:"end_time"`
	Status      *int16           `json:"status"`
	Schedule    *service.ScheduleSnapshot `json:"schedule"`
}

func toTaskParams(p taskReq) (service.CreateTaskParams, error) {
	return service.CreateTaskParams{
		TeacherID: p.TeacherID, TeacherName: p.TeacherName,
		CourseName: p.CourseName, Classroom: p.Classroom,
		ClassTime: p.ClassTime, StartTime: p.StartTime, EndTime: p.EndTime,
		ScheduleSnapshot: p.Schedule,
	}, nil
}

// taskScheduleSnapshot 解析任务上的课表信息快照；无则返回 nil
func taskScheduleSnapshot(t *model.EvaluationTask) interface{} {
	if len(t.ScheduleSnapshot) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(t.ScheduleSnapshot, &m); err != nil {
		return nil
	}
	return m
}

// taskCreatePayload 创建响应体（对齐旧端 create/batch 返回结构）
func taskCreatePayload(t *model.EvaluationTask) gin.H {
	return gin.H{
		"id": t.ID, "teacher_id": t.TeacherID, "teacher_name": t.TeacherName,
		"course_name": t.CourseName, "class_time": FTimeMin(t.ClassTime),
		"classroom": t.Classroom, "schedule": taskScheduleSnapshot(t), "status": t.Status,
		"status_name":      model.TaskStatusNames[t.Status],
		"evaluation_count": t.EvaluationCount, "has_supervisor_eval": t.HasSupervisorEval,
		"create_by": t.CreateBy, "create_time": FTime(t.CreateTime),
	}
}

// taskUpdatePayload 更新响应体（对齐旧端 update 返回结构）
func taskUpdatePayload(t *model.EvaluationTask, teacher *model.User) gin.H {
	teacherName := t.TeacherName
	if teacher != nil {
		teacherName = teacher.Username
	}
	return gin.H{
		"id": t.ID, "teacher_id": t.TeacherID, "teacher_name": teacherName,
		"course_name": t.CourseName, "class_time": FTimeMin(t.ClassTime),
		"classroom": t.Classroom, "schedule": taskScheduleSnapshot(t), "status": t.Status,
		"status_name":      model.TaskStatusNames[t.Status],
		"evaluation_count": t.EvaluationCount, "has_supervisor_eval": t.HasSupervisorEval,
		"update_time": FTime(t.UpdateTime),
	}
}

// Create 创建任务
func (h *Task) Create(c *gin.Context) {
	var p taskReq
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	cp, err := toTaskParams(p)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	u := middleware.CurrentUser(c)
	t, err := h.svc.Create(h.db, u, cp)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &u.ID, u.Username, "create", "task", &t.ID, "evaluation_task",
		map[string]interface{}{"teacher_id": t.TeacherID, "course_name": t.CourseName})
	response.OKMsg(c, "创建成功", taskCreatePayload(t))
}

// BatchCreate 批量创建
func (h *Task) BatchCreate(c *gin.Context) {
	var req struct {
		Tasks []taskReq `json:"tasks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Tasks) == 0 {
		badReq(c, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)
	items := make([]service.CreateTaskParams, 0, len(req.Tasks))
	for _, tr := range req.Tasks {
		cp, err := toTaskParams(tr)
		if err != nil {
			badReq(c, err.Error())
			return
		}
		items = append(items, cp)
	}
	res, err := h.svc.BatchCreate(h.db, u, items)
	if err != nil {
		serverErr(c, "批量创建失败")
		return
	}
	created := []gin.H{}
	for i := range res.Created {
		created = append(created, taskCreatePayload(&res.Created[i]))
	}
	service.LogRecord(h.db, &u.ID, u.Username, "create", "task", nil, "evaluation_task",
		map[string]interface{}{"batch": true, "created": len(created), "skipped": len(res.Skipped)})
	response.OKMsg(c, fmt.Sprintf("批量创建完成：成功 %d 条，跳过 %d 条", len(created), len(res.Skipped)), gin.H{
		"created":       created,
		"skipped":       res.Skipped,
		"created_count": len(created),
		"skipped_count": len(res.Skipped),
	})
}

// Update 编辑任务
func (h *Task) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p taskReq
	if err := c.ShouldBindJSON(&p); err != nil {
		badReq(c, "请求参数错误")
		return
	}
	up := service.UpdateTaskParams{
		CourseName: &p.CourseName, Classroom: p.Classroom, Status: p.Status,
		ClassTime: p.ClassTime, StartTime: p.StartTime, EndTime: p.EndTime,
		ScheduleSnapshot: p.Schedule,
	}
	// 空字符串表示不修改
	if p.CourseName == "" {
		up.CourseName = nil
	}

	u := middleware.CurrentUser(c)
	t, err := h.svc.Update(h.db, u, id, up)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	teacher, _ := service.LoadTeacherUser(h.db, t.TeacherID)
	service.LogRecord(h.db, &u.ID, u.Username, "update", "task", &t.ID, "evaluation_task", nil)
	response.OKMsg(c, "更新成功", taskUpdatePayload(t, teacher))
}

// Delete 软删除任务
func (h *Task) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	u := middleware.CurrentUser(c)
	if err := h.svc.Delete(h.db, u, id); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &u.ID, u.Username, "delete", "task", &id, "evaluation_task", nil)
	response.OKMsg(c, "删除成功", nil)
}

// splitIntsHandler 逗号分隔 ID
func splitIntsHandler(s string) []int {
	var ids []int
	cur, has := 0, false
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if has {
				ids = append(ids, cur)
			}
			cur, has = 0, false
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			cur = cur*10 + int(s[i]-'0')
			has = true
		}
	}
	return ids
}
