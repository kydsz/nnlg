package service

import (
	"testing"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// userListTestDB 复用了 teacherScopeTestDB 的用户/交叉记录样例，
// 并补充学院记录供 User.List 预加载 UserColleges.College 使用：
// 学院 5=文理学院，6=马克思主义学院。
func userListTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := teacherScopeTestDB(t)
	colleges := []model.College{
		{Model: model.Model{ID: 5}, Code: "W", Name: "文理学院", Status: 1},
		{Model: model.Model{ID: 6}, Code: "M", Name: "马克思主义学院", Status: 1},
	}
	if err := db.Create(&colleges).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
	return db
}

func TestUserListCollegeFilter(t *testing.T) {
	db := userListTestDB(t)
	svc := &User{}

	t.Run("按学院筛选只匹配主学院，忽略 user_college 交叉记录", func(t *testing.T) {
		users, total, err := svc.List(db, UserParams{Page: 1, PageSize: 50, CollegeID: "5"})
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		got := teacherIDsOf(t, users)
		if total != 1 || len(got) != 1 || got[0] != "文理教师" {
			t.Fatalf("college_id=5 应只返回文理教师, 得到 total=%d %v", total, got)
		}
	})
}
