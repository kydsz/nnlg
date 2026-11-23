package service

import (
	"errors"
	"sort"
	"strings"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// 本文件集中「人事学院 / 授课学院」口径，避免任务、评教、统计各自拼接 SQL。
//
// 口径约定：
//   - 人事学院：user.college_id（历史字段，含义不变）；
//   - 授课学院：teacher_teaching_college 中的多对多关系；
//   - 评教任务的学院：course_college_id 快照优先，其次 teacher_college_id，最后回退教师人事学院；
//   - user_college 仍只表示督导「负责学院」，不参与教师授课归属。

// quoteIdent 简单引用标识符（仅用于内部固定的表别名/表名）
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "") + "`"
}

// TaskCollegeExpr 任务学院表达式（alias 为 evaluation_task 的表别名）
func TaskCollegeExpr(alias string) string {
	a := quoteIdent(alias)
	return "COALESCE(" + a + ".course_college_id, " + a + ".teacher_college_id, " +
		"(SELECT tu.college_id FROM `user` tu WHERE tu.id = " + a + ".teacher_id))"
}

// TaskCollegeID 单个任务的学院：授课学院快照 > 人事学院快照 > 教师当前主学院
func TaskCollegeID(db *gorm.DB, task *model.EvaluationTask) *int {
	if task == nil {
		return nil
	}
	if task.CourseCollegeID != nil {
		return task.CourseCollegeID
	}
	if task.TeacherCollegeID != nil {
		return task.TeacherCollegeID
	}
	if task.TeacherID == 0 {
		return nil
	}
	var u model.User
	if err := db.Select("id, college_id").First(&u, task.TeacherID).Error; err != nil {
		return nil
	}
	return u.CollegeID
}

// TaskCollegeInfo 任务的两个人事/授课学院展示信息
type TaskCollegeInfo struct {
	TeacherCollege *model.College
	CourseCollege  *model.College
}

// CollegeDisplay 学院展示信息：ID 指针与名称，无学院时均为 nil（JSON null）
func CollegeDisplay(c *model.College) (*int, interface{}) {
	if c == nil {
		return nil, nil
	}
	id := c.ID
	return &id, c.Name
}

// TaskCollegeMap 批量读取任务学院（按任务 ID 键）。
// 无快照的历史任务回退被评教师当前人事学院，与 TaskCollegeExpr 的回退口径一致。
func TaskCollegeMap(db *gorm.DB, tasks []model.EvaluationTask) map[int]TaskCollegeInfo {
	out := map[int]TaskCollegeInfo{}
	if len(tasks) == 0 {
		return out
	}
	collegeIDs := map[int]bool{}
	needTeacher := map[int]bool{}
	for _, t := range tasks {
		if t.CourseCollegeID != nil {
			collegeIDs[*t.CourseCollegeID] = true
		}
		if t.TeacherCollegeID != nil {
			collegeIDs[*t.TeacherCollegeID] = true
		}
		if t.CourseCollegeID == nil && t.TeacherCollegeID == nil && t.TeacherID != 0 {
			needTeacher[t.TeacherID] = true
		}
	}
	teacherColleges := map[int]*int{}
	if len(needTeacher) > 0 {
		var users []model.User
		db.Select("id, college_id").Where("id IN ?", keysOf(needTeacher)).Find(&users)
		for i := range users {
			teacherColleges[users[i].ID] = users[i].CollegeID
			if users[i].CollegeID != nil {
				collegeIDs[*users[i].CollegeID] = true
			}
		}
	}
	colleges := map[int]model.College{}
	if len(collegeIDs) > 0 {
		var list []model.College
		db.Where("id IN ?", keysOf(collegeIDs)).Find(&list)
		for _, c := range list {
			colleges[c.ID] = c
		}
	}
	collegeOf := func(id *int) *model.College {
		if id == nil {
			return nil
		}
		if c, ok := colleges[*id]; ok {
			cc := c
			return &cc
		}
		return nil
	}
	for _, t := range tasks {
		teacherCollegeID := t.TeacherCollegeID
		if teacherCollegeID == nil {
			teacherCollegeID = teacherColleges[t.TeacherID]
		}
		courseCollegeID := t.CourseCollegeID
		if courseCollegeID == nil {
			courseCollegeID = teacherCollegeID
		}
		out[t.ID] = TaskCollegeInfo{
			TeacherCollege: collegeOf(teacherCollegeID),
			CourseCollege:  collegeOf(courseCollegeID),
		}
	}
	return out
}

// TeacherSelectableColleges 教师可选学院 = 人事学院 + 全部授课学院（去重，按 ID 排序）
func TeacherSelectableColleges(db *gorm.DB, teacher *model.User) []model.College {
	if teacher == nil {
		return nil
	}
	ids := []int{}
	if teacher.CollegeID != nil {
		ids = append(ids, *teacher.CollegeID)
	}
	for _, id := range TeacherTeachingCollegeIDs(db, teacher.ID) {
		if !containsInt(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var list []model.College
	db.Where("id IN ?", ids).Order("id ASC").Find(&list)
	return list
}

// TeacherSelectableCollegeMap 批量读取教师可选学院（人事学院 + 授课学院，按教师 ID 键）。
func TeacherSelectableCollegeMap(db *gorm.DB, teacherIDs []int) map[int][]model.College {
	out := map[int][]model.College{}
	if len(teacherIDs) == 0 {
		return out
	}
	var users []model.User
	db.Select("id, college_id").Where("id IN ?", teacherIDs).Find(&users)
	var ttcs []model.TeacherTeachingCollege
	db.Where("user_id IN ?", teacherIDs).Find(&ttcs)
	idsSet := map[int]bool{}
	for _, u := range users {
		if u.CollegeID != nil {
			idsSet[*u.CollegeID] = true
		}
	}
	for _, ttc := range ttcs {
		idsSet[ttc.CollegeID] = true
	}
	colleges := map[int]model.College{}
	if len(idsSet) > 0 {
		var list []model.College
		db.Where("id IN ?", keysOf(idsSet)).Find(&list)
		for _, c := range list {
			colleges[c.ID] = c
		}
	}
	ttcByUser := map[int][]int{}
	for _, ttc := range ttcs {
		ttcByUser[ttc.UserID] = append(ttcByUser[ttc.UserID], ttc.CollegeID)
	}
	for _, u := range users {
		seen := map[int]bool{}
		ids := []int{}
		if u.CollegeID != nil {
			ids = append(ids, *u.CollegeID)
			seen[*u.CollegeID] = true
		}
		for _, id := range ttcByUser[u.ID] {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		sort.Ints(ids)
		for _, id := range ids {
			if c, ok := colleges[id]; ok {
				out[u.ID] = append(out[u.ID], c)
			}
		}
	}
	return out
}

// TeacherTeachingCollegeIDs 教师全部授课学院 ID（去重、稳定排序）
func TeacherTeachingCollegeIDs(db *gorm.DB, userID int) []int {
	if userID == 0 {
		return nil
	}
	var ids []int
	db.Model(&model.TeacherTeachingCollege{}).
		Where("user_id = ?", userID).Order("college_id ASC").Pluck("college_id", &ids)
	return ids
}

// whereTeacherCollegeIn 教师归属条件：主学院命中 或 授课学院命中。
// ids == nil 表示全校（不加条件）；空切片表示无范围（恒假）。
func whereTeacherCollegeIn(q *gorm.DB, alias string, ids []int) *gorm.DB {
	if ids == nil {
		return q
	}
	if len(ids) == 0 {
		return q.Where("1 = 0")
	}
	a := quoteIdent(alias)
	return q.Where("("+a+".college_id IN ? OR EXISTS (SELECT 1 FROM `teacher_teaching_college` ttc WHERE ttc.user_id = "+a+".id AND ttc.college_id IN ?))", ids, ids)
}

// TeacherInScope 调用者是否可操作该教师：教师人事学院或任一授课学院在调用者范围内。
func TeacherInScope(db *gorm.DB, caller *model.User, teacher *model.User) bool {
	if caller == nil || teacher == nil {
		return false
	}
	scope := AccessibleCollegeIDs(caller)
	if scope == nil {
		return true
	}
	if teacher.CollegeID != nil && containsInt(scope, *teacher.CollegeID) {
		return true
	}
	var cnt int64
	db.Model(&model.TeacherTeachingCollege{}).
		Where("user_id = ? AND college_id IN ?", teacher.ID, scope).Count(&cnt)
	return cnt > 0
}

// taskTargetScopeError 沿用各角色的既有错误文案
func taskTargetScopeError(caller *model.User) error {
	switch {
	case IsAdminRole(caller):
		return errors.New("您只能操作本学院教师的任务")
	case IsSupervisor(caller):
		return errors.New("您只能操作自己负责学院的教师任务")
	default:
		return errors.New("您只能为同学院教师创建任务")
	}
}

// ResolveTaskCollege 解析任务的「人事学院快照 + 授课学院快照」。
//
// 优先级：
//  1. 显式 college_id（必须是教师主学院或授课学院，且非全校管理员时在调用者范围内）；
//  2. 「教师可选学院 ∩ 调用者范围」：调用者主学院 > 教师主学院 > 最小 ID；
//  3. 系统管理员 + 教师无任何学院关系：允许为空（显示 "-"）。
//
// 交集为空时按角色返回原有错误文案（学院/督导/教师）。
func ResolveTaskCollege(db *gorm.DB, caller *model.User, teacher *model.User, explicit *int) (*int, *int, error) {
	allowed := []int{}
	if teacher.CollegeID != nil {
		allowed = append(allowed, *teacher.CollegeID)
	}
	for _, id := range TeacherTeachingCollegeIDs(db, teacher.ID) {
		if !containsInt(allowed, id) {
			allowed = append(allowed, id)
		}
	}
	sort.Ints(allowed)

	scope := AccessibleCollegeIDs(caller)
	inScope := func(id int) bool {
		return scope == nil || containsInt(scope, id)
	}

	if explicit != nil {
		if !containsInt(allowed, *explicit) {
			return nil, nil, errors.New("该教师不属于所选授课学院")
		}
		if !inScope(*explicit) {
			return nil, nil, taskTargetScopeError(caller)
		}
		course := *explicit
		return teacher.CollegeID, &course, nil
	}

	candidates := make([]int, 0, len(allowed))
	for _, id := range allowed {
		if inScope(id) {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		if scope == nil {
			// 系统管理员可为无学院的教师建任务（历史行为）
			return teacher.CollegeID, nil, nil
		}
		return nil, nil, taskTargetScopeError(caller)
	}

	chosen := candidates[0]
	if caller.CollegeID != nil && containsInt(candidates, *caller.CollegeID) {
		chosen = *caller.CollegeID
	} else if teacher.CollegeID != nil && containsInt(candidates, *teacher.CollegeID) {
		chosen = *teacher.CollegeID
	}
	return teacher.CollegeID, &chosen, nil
}
