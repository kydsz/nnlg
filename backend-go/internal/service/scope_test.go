package service

import (
	"testing"

	"backend-go/internal/model"
)

// TestAccessibleCollegeIDsAdminScope 学院管理员的数据范围都以「本学院」为界：
// 仅主学院，不认督导「负责学院」，未绑定学院为空集（fail-closed）。
// 评教任务、评教记录、统计报表、教师评教汇总等统一走该口径。
func TestAccessibleCollegeIDsAdminScope(t *testing.T) {
	cases := []struct {
		name string
		user *model.User
		want []int // nil 表示全校
	}{
		{"system_admin 全校", &model.User{Role: model.RoleSystemAdmin}, nil},
		{"school_supervisor 全校", &model.User{Role: model.RoleSchoolSupervisor}, nil},
		{
			"college_admin 仅主学院不认负责学院",
			&model.User{
				Role:         model.RoleCollegeAdmin,
				CollegeID:    intPtr(1),
				UserColleges: []model.UserCollege{{CollegeID: 2}, {CollegeID: 3}},
			},
			[]int{1},
		},
		{"college_admin 未绑定学院为空集", &model.User{Role: model.RoleCollegeAdmin}, []int{}},
		{"school_admin 仅主学院", &model.User{Role: model.RoleSchoolAdmin, CollegeID: intPtr(2)}, []int{2}},
		{
			"督导为主学院+负责学院",
			&model.User{
				Role:         model.RoleSupervisor,
				CollegeID:    intPtr(1),
				UserColleges: []model.UserCollege{{CollegeID: 1}, {CollegeID: 2}},
			},
			[]int{1, 2},
		},
		{"教师仅主学院", &model.User{Role: model.RoleTeacher, CollegeID: intPtr(4)}, []int{4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AccessibleCollegeIDs(c.user)
			if c.want == nil {
				if got != nil {
					t.Fatalf("期望全校(nil)，得到 %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("期望 %v，得到 nil(全校)", c.want)
			}
			if !sameInts(got, c.want) {
				t.Fatalf("得到 %v，期望 %v", got, c.want)
			}
		})
	}
}
