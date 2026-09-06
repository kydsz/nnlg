package service

import (
	"testing"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// 评教记录搜索语义（工单 #10）：
// keyword 命中「课程名/教师名/评教人名/作答文本」超集；
// 评教人姓名匹配（keyword 与 evaluator_name 参数）对匿名记录按 canSeeIdentity 口径放行——
// 评教人本人 / 系统管理员 / 数据范围内持 view_all 权限者可搜到，其余不可（匿名全程脱敏）。

func searchTestDB(t *testing.T) (*gorm.DB, *model.User, *model.User, *model.User) {
	t.Helper()
	db := evaluationTestDB(t)

	users := []model.User{
		{UserNo: "T001", Username: "被评教师张三", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
		{UserNo: "T002", Username: "评教人李四", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
		{UserNo: "T003", Username: "评教人王五", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}
	teacher, lisi, wangwu := &users[0], &users[1], &users[2]

	task := model.EvaluationTask{
		TeacherID: teacher.ID, TeacherName: "张三", CourseName: "高等数学",
		Status: model.TaskStatusPending,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("建任务失败: %v", err)
	}
	recs := []model.EvaluationRecord{
		{TaskID: task.ID, EvaluatorID: &lisi.ID, EvaluatorName: "李四", EvaluatorRole: model.RoleTeacher,
			DimensionValues: []byte(`{"listening_content":"板书清晰"}`)},
		{TaskID: task.ID, EvaluatorID: &wangwu.ID, EvaluatorName: "王五", EvaluatorRole: model.RoleTeacher,
			IsAnonymous: true, DimensionValues: []byte(`{"listening_content":"节奏好"}`)},
	}
	if err := db.Create(&recs).Error; err != nil {
		t.Fatalf("建评教记录失败: %v", err)
	}
	return db, teacher, lisi, wangwu
}

func searchIDs(items []map[string]interface{}) []int {
	ids := make([]int, 0, len(items))
	for _, it := range items {
		ids = append(ids, it["id"].(int))
	}
	return ids
}

func TestListKeywordHits(t *testing.T) {
	db, _, _, _ := searchTestDB(t)
	admin := &model.User{Role: model.RoleSystemAdmin, Status: 1, Perms: []string{}}

	cases := []struct {
		name    string
		viewer  *model.User
		keyword string
		want    []int // 期望命中的记录下标（1=李四实名, 2=王五匿名）
	}{
		{"课程名", admin, "高等数学", []int{1, 2}},
		{"教师名", admin, "张三", []int{1, 2}},
		{"评教人名（实名记录）", admin, "李四", []int{1}},
		{"作答文本（超集保留）", admin, "板书", []int{1}},
		{"部分字词", admin, "等数", []int{1, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, _, err := NewEvaluation().List(db, tc.viewer, EvaluationFilters{Keyword: tc.keyword, Page: 1, PageSize: 20})
			if err != nil {
				t.Fatalf("List 失败: %v", err)
			}
			if got := len(searchIDs(items)); got != len(tc.want) {
				t.Fatalf("keyword=%q 期望命中 %d 条, 得到 %d 条: %v", tc.keyword, len(tc.want), got, searchIDs(items))
			}
		})
	}
}

// 匿名记录的姓名可搜索性按查看者区分
func TestListAnonymousNameSearch(t *testing.T) {
	db, teacher, _, wangwu := searchTestDB(t)

	inScopeViewer := &model.User{Role: model.RoleCollegeAdmin, CollegeID: intPtr(5), Status: 1,
		UserRoles: []model.UserRole{{Role: model.RoleCollegeAdmin}}, Perms: []string{"evaluation:view_all"}}

	cases := []struct {
		name   string
		viewer *model.User
		filter EvaluationFilters
		want   int
	}{
		// 教师（被评人）按真名搜自己的匿名评教：不可见身份 → 搜不到
		{"被评教师搜匿名评教人 keyword", teacher, EvaluationFilters{Keyword: "王五", Page: 1, PageSize: 20}, 0},
		{"被评教师搜匿名评教人 evaluator_name", teacher, EvaluationFilters{EvaluatorName: "王五", Page: 1, PageSize: 20}, 0},
		// 匿名记录仍可按课程/作答搜到，展示保持脱敏
		{"被评教师按课程搜到匿名记录", teacher, EvaluationFilters{Keyword: "高等数学", Page: 1, PageSize: 20}, 2},
		// 系统管理员可搜到
		{"系统管理员", &model.User{Role: model.RoleSystemAdmin, Status: 1, Perms: []string{}},
			EvaluationFilters{EvaluatorName: "王五", Page: 1, PageSize: 20}, 1},
		// 数据范围内持 view_all 权限者可搜到
		{"范围内 viewAll 管理员", inScopeViewer, EvaluationFilters{EvaluatorName: "王五", Page: 1, PageSize: 20}, 1},
		// 评教人本人（type=sent）可搜到自己
		{"评教人本人", wangwu, EvaluationFilters{Type: "sent", Keyword: "王五", Page: 1, PageSize: 20}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, _, err := NewEvaluation().List(db, tc.viewer, tc.filter)
			if err != nil {
				t.Fatalf("List 失败: %v", err)
			}
			if got := len(items); got != tc.want {
				t.Fatalf("期望 %d 条, 得到 %d 条", tc.want, got)
			}
		})
	}

	// 教师按课程搜出的匿名记录：展示仍脱敏
	items, _, err := NewEvaluation().List(db, teacher, EvaluationFilters{Keyword: "节奏", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(items) != 1 || items[0]["evaluator_name"] != "匿名" {
		t.Fatalf("匿名记录经作答文本搜出后应脱敏展示, 得到 %v", items)
	}
}
