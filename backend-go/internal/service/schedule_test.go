package service

import (
	"testing"

	"backend-go/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSemesterDurationDays(t *testing.T) {
	cases := []struct {
		name  string
		weeks int
		want  int
	}{
		{"默认20周", 20, 140},
		{"未配置取默认", 0, 140},
		{"负数取默认", -5, 140},
		{"自定义16周", 16, 112},
		{"自定义25周", 25, 175},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := semesterDurationDays(c.weeks); got != c.want {
				t.Fatalf("semesterDurationDays(%d) = %d, 期望 %d", c.weeks, got, c.want)
			}
		})
	}
}

func TestParseStudentCount(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		wantN int
		want  bool
	}{
		{"取最后一个括号数字", "2404计算机班,2403计算机(125)", 125, true},
		{"无括号", "2404计算机班", 0, false},
		{"多个括号取最后", "a(10)b(20)", 20, true},
		{"括号内非数字", "a(x)", 0, false},
		{"全角括号不匹配", "（30）", 0, false},
		{"空字符串", "", 0, false},
		{"括号内含空格不匹配", "a( 30 )", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, ok := ParseStudentCount(c.in)
			if ok != c.want || n != c.wantN {
				t.Fatalf("ParseStudentCount(%q) = (%d, %v), 期望 (%d, %v)", c.in, n, ok, c.wantN, c.want)
			}
		})
	}
}

func TestScheduleDetailPayload(t *testing.T) {
	t.Run("取明细最大学生数", func(t *testing.T) {
		sc := &model.CourseSchedule{
			Model:       model.Model{ID: 1},
			TeacherID:   intPtr(7),
			TeacherName: "张三",
			Semester:    "2026-2027-1",
			Details: []model.CourseScheduleDetail{
				{Model: model.Model{ID: 1}, CourseName: "高等数学", ClassInfo: "2401(125)"},
				{Model: model.Model{ID: 2}, CourseName: "大学物理", ClassInfo: "2402(50)"},
				{Model: model.Model{ID: 3}, CourseName: "无人数课程", ClassInfo: "2403"},
			},
		}
		out := scheduleDetailPayload(sc)
		cnt, ok := out["student_count"].(*int)
		if !ok || cnt == nil || *cnt != 125 {
			t.Fatalf("student_count 应为 125, 得到 %v", out["student_count"])
		}
		details, ok := out["details"].([]map[string]interface{})
		if !ok || len(details) != 3 {
			t.Fatalf("details 应有 3 条, 得到 %d 条", len(details))
		}
		// 无人数明细的 student_count 应为 nil
		if got := details[2]["student_count"]; got != nil {
			t.Fatalf("无人数明细 student_count 应为 nil, 得到 %v", got)
		}
	})

	t.Run("无明细返回空壳", func(t *testing.T) {
		sc := &model.CourseSchedule{Model: model.Model{ID: 2}}
		out := scheduleDetailPayload(sc)
		// student_count 是 *int 类型的 typed-nil，需断言后判空
		cnt, ok := out["student_count"].(*int)
		if !ok || cnt != nil {
			t.Fatalf("无明细 student_count 应为 nil 的 *int, 得到 %v", out["student_count"])
		}
		if d, ok := out["details"].([]map[string]interface{}); !ok || len(d) != 0 {
			t.Fatalf("无明细 details 应为空, 得到 %v", out["details"])
		}
	})
}

func intPtr(v int) *int { return &v }

func TestTeacherScopeOf(t *testing.T) {
	cases := []struct {
		name string
		user *model.User
		want *TeacherScope
	}{
		{"system_admin 全校", &model.User{Role: model.RoleSystemAdmin}, nil},
		{"school_supervisor 全校", &model.User{Role: model.RoleSchoolSupervisor}, nil},
		{"college_admin 无学院全校", &model.User{Role: model.RoleCollegeAdmin}, nil},
		{
			"督导负责学院+教研室",
			&model.User{
				Role: model.RoleSupervisor,
				UserColleges: []model.UserCollege{{CollegeID: 2}, {CollegeID: 3}},
				UserRooms:    []model.UserRoom{{ResearchRoomID: 5}},
			},
			&TeacherScope{CollegeIDs: []int{2, 3}, RoomIDs: []int{5}},
		},
		{
			"督导含主学院与主教研室",
			&model.User{
				Role:           model.RoleCollegeSupervisor,
				CollegeID:      intPtr(1),
				ResearchRoomID: intPtr(9),
				UserColleges:   []model.UserCollege{{CollegeID: 1}},
			},
			&TeacherScope{CollegeIDs: []int{1}, RoomIDs: []int{9}},
		},
		{
			"督导无任何负责范围返回空范围",
			&model.User{Role: model.RoleSupervisor},
			&TeacherScope{CollegeIDs: []int{}, RoomIDs: []int{}},
		},
		{
			"教师仅主学院",
			&model.User{Role: model.RoleTeacher, CollegeID: intPtr(4)},
			&TeacherScope{CollegeIDs: []int{4}, RoomIDs: []int{}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TeacherScopeOf(c.user)
			if c.want == nil {
				if got != nil {
					t.Fatalf("期望全校(nil)，得到 %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("期望 %+v，得到 nil", c.want)
			}
			if !sameInts(got.CollegeIDs, c.want.CollegeIDs) {
				t.Fatalf("CollegeIDs = %v，期望 %v", got.CollegeIDs, c.want.CollegeIDs)
			}
			if !sameInts(got.RoomIDs, c.want.RoomIDs) {
				t.Fatalf("RoomIDs = %v，期望 %v", got.RoomIDs, c.want.RoomIDs)
			}
		})
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[int]bool{}
	for _, v := range b {
		set[v] = true
	}
	for _, v := range a {
		if !set[v] {
			return false
		}
	}
	return true
}

// teacherScopeTestDB 构造内存库并灌入教师/督导样例：
// 学院 5=文理学院，6=马克思主义学院（模拟 user_college 交叉记录的真实场景）
func teacherScopeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.UserRole{}, &model.UserCollege{},
		&model.UserRoom{}, &model.College{}, &model.ResearchRoom{},
	); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	users := []model.User{
		{UserNo: "T001", Username: "文理教师", Role: model.RoleTeacher, CollegeID: intPtr(5), Status: 1},
		// 主学院马院，但 user_college 记了文理学院（交叉记录）
		{UserNo: "T002", Username: "马院教师A", Role: model.RoleTeacher, CollegeID: intPtr(6), Status: 1},
		{UserNo: "T003", Username: "马院教师B", Role: model.RoleTeacher, CollegeID: intPtr(6), Status: 1},
		// 主学院马院的督导，负责文理学院（user_college 语义来源）
		{UserNo: "T004", Username: "马院督导", Role: model.RoleSupervisor, CollegeID: intPtr(6), Status: 1},
		// 主学院马院但兼职文理学院教研室的教师
		{UserNo: "T005", Username: "马院兼职教师", Role: model.RoleTeacher, CollegeID: intPtr(6), Status: 1},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("灌入用户失败: %v", err)
	}
	seed := []struct {
		ucUser  int
		ucColl  int
		roomUser int
		roomID  int
	}{
		{ucUser: int(users[1].ID), ucColl: 5},
		{ucUser: int(users[3].ID), ucColl: 5},
		{roomUser: int(users[4].ID), roomID: 1},
	}
	for _, s := range seed {
		if s.ucUser != 0 {
			if err := db.Create(&model.UserCollege{UserID: s.ucUser, CollegeID: s.ucColl}).Error; err != nil {
				t.Fatalf("灌入 user_college 失败: %v", err)
			}
		}
		if s.roomUser != 0 {
			if err := db.Create(&model.UserRoom{UserID: s.roomUser, ResearchRoomID: s.roomID}).Error; err != nil {
				t.Fatalf("灌入 user_research_room 失败: %v", err)
			}
		}
	}
	if err := db.Create(&model.ResearchRoom{Code: "R1", Name: "文理教研室", CollegeID: 5, Status: 1}).Error; err != nil {
		t.Fatalf("灌入教研室失败: %v", err)
	}
	return db
}

func teacherIDsOf(t *testing.T, users []model.User) []string {
	t.Helper()
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.Username)
	}
	return out
}

func TestListTeachersCollegeFilter(t *testing.T) {
	db := teacherScopeTestDB(t)
	svc := &Schedule{}

	t.Run("按学院筛选只匹配教师主学院，忽略 user_college 交叉记录", func(t *testing.T) {
		users, total, err := svc.ListTeachers(db, TeacherFilter{Page: 1, PageSize: 50, CollegeID: "5"})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if total != 1 || len(got) != 1 || got[0] != "文理教师" {
			t.Fatalf("college_id=5 应只返回文理教师, 得到 total=%d %v", total, got)
		}
	})

	t.Run("督导范围过滤只匹配教师主学院（回归）", func(t *testing.T) {
		users, total, err := svc.ListTeachers(db, TeacherFilter{
			Page: 1, PageSize: 50,
			Scope: &TeacherScope{CollegeIDs: []int{5}, RoomIDs: []int{}},
		})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if total != 1 || len(got) != 1 || got[0] != "文理教师" {
			t.Fatalf("范围=文理学院 应只返回文理教师, 得到 total=%d %v", total, got)
		}
	})

	t.Run("督导负责教研室可见其中兼职教师", func(t *testing.T) {
		users, total, err := svc.ListTeachers(db, TeacherFilter{
			Page: 1, PageSize: 50,
			Scope: &TeacherScope{CollegeIDs: []int{5}, RoomIDs: []int{1}},
		})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if len(got) != 2 || !containsStr(got, "文理教师") || !containsStr(got, "马院兼职教师") {
			t.Fatalf("范围=文理学院+教研室1 应含文理教师与马院兼职教师, 得到 total=%d %v", total, got)
		}
	})

	t.Run("教师列表不含督导等非教师角色", func(t *testing.T) {
		users, _, err := svc.ListTeachers(db, TeacherFilter{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if got := teacherIDsOf(t, users); containsStr(got, "马院督导") {
			t.Fatalf("教师列表不应含督导角色, 得到 %v", got)
		}
	})
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
