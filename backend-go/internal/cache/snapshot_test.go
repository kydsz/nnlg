package cache

import (
	"context"
	"testing"

	"backend-go/internal/config"
	"backend-go/internal/model"
)

func snapshotTestUser() *model.User {
	collegeID, roomID := 10, 100
	u := &model.User{
		Model:  model.Model{ID: 1},
		UserNo: "T001", Username: "张老师", Role: model.RoleTeacher,
		CollegeID: &collegeID, ResearchRoomID: &roomID,
		Status: 1, MustChangePassword: false,
		UserRoles: []model.UserRole{{Model: model.Model{ID: 1}, UserID: 1, Role: model.RoleTeacher}},
		UserColleges: []model.UserCollege{{
			Model: model.Model{ID: 1}, UserID: 1, CollegeID: collegeID,
			College: &model.College{Model: model.Model{ID: collegeID}, Name: "计算机学院", Code: "CS"},
		}},
		UserRooms: []model.UserRoom{{
			Model: model.Model{ID: 1}, UserID: 1, ResearchRoomID: roomID,
			ResearchRoom: &model.ResearchRoom{Model: model.Model{ID: roomID}, Name: "软件教研室", CollegeID: collegeID,
				College: &model.College{Model: model.Model{ID: collegeID}, Name: "计算机学院", Code: "CS"}},
		}},
		College:      &model.College{Model: model.Model{ID: collegeID}, Name: "计算机学院", Code: "CS"},
		ResearchRoom: &model.ResearchRoom{Model: model.Model{ID: roomID}, Name: "软件教研室", CollegeID: collegeID},
	}
	u.Perms = []string{"user:view", "evaluation:create"}
	return u
}

func TestAuthUserSnapshotRoundTrip(t *testing.T) {
	u := snapshotTestUser()
	snap := SnapshotFromUser(u)
	back := snap.ToUser()

	if back.ID != 1 || back.Username != "张老师" || back.Role != model.RoleTeacher || back.Status != 1 {
		t.Fatalf("基础字段不符: %+v", back)
	}
	if !back.HasRole(model.RoleTeacher) {
		t.Fatal("角色应被重建")
	}
	if !back.HasCollegeID(10) {
		t.Fatal("学院关联应被重建")
	}
	if back.College == nil || back.College.Name != "计算机学院" {
		t.Fatalf("主学院指针缺失: %+v", back.College)
	}
	if back.ResearchRoom == nil || back.ResearchRoom.Name != "软件教研室" {
		t.Fatalf("主教研室指针缺失: %+v", back.ResearchRoom)
	}
	if len(back.UserRooms) != 1 || back.UserRooms[0].ResearchRoom == nil || back.UserRooms[0].ResearchRoom.College == nil {
		t.Fatalf("教研室关联或学院指针缺失: %+v", back.UserRooms)
	}
	// Perms 非空时应直接返回，不再依赖 db（传 nil 验证）
	perms := back.Permissions(nil)
	if len(perms) != 2 || perms[0] != "user:view" {
		t.Fatalf("权限未从快照恢复: %v", perms)
	}
}

func TestSnapshotHandlesEmptyCollections(t *testing.T) {
	u := &model.User{Model: model.Model{ID: 7}, Role: "teacher", Status: 1}
	back := SnapshotFromUser(u).ToUser()
	if back.Permissions(nil) == nil {
		t.Fatal("空权限应保留为空切片而非 nil，避免触发回源查询")
	}
	if len(back.RoleCodes()) == 0 {
		t.Fatal("应回退 legacy role 字段")
	}
}

func TestRedisDisabledClientIsSafe(t *testing.T) {
	c := New(&config.Config{}) // RedisAddr 为空 → 禁用
	if c.Enabled {
		t.Fatal("空 addr 应返回禁用状态")
	}
	ctx := context.Background()
	if _, ok := c.GetAuthUser(ctx, 1); ok {
		t.Fatal("禁用客户端不应命中缓存")
	}
	c.SetAuthUser(ctx, 1, SnapshotFromUser(snapshotTestUser())) // 不应 panic
	c.DelAuthUser(ctx, 1)
	// 会话 epoch key（禁用时仅作 key 生成，不应 panic）
	_ = c.SessionEpochKey(1)
}
