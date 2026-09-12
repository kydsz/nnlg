package handler

import (
	"testing"
	"time"

	"backend-go/internal/model"
)

func TestProgressIntAndFailedIDs(t *testing.T) {
	rec := map[string]interface{}{
		"total": 3, "completed": int64(2), "percent": float64(66.6), "status": "running",
	}
	cases := []struct {
		key  string
		want int
	}{
		{"total", 3},
		{"completed", 2},
		{"percent", 66},
		{"missing", 0},
		{"status", 0}, // 类型不符时按 0 处理而不是 panic
	}
	for _, c := range cases {
		if got := progressInt(rec, c.key); got != c.want {
			t.Fatalf("progressInt(%q) = %d, 期望 %d", c.key, got, c.want)
		}
	}

	if ids := progressFailedIDs(map[string]interface{}{"failed_ids": "bad-value"}); len(ids) != 0 {
		t.Fatalf("非法 failed_ids 应返回空切片: %+v", ids)
	}
	if ids := progressFailedIDs(map[string]interface{}{}); len(ids) != 0 {
		t.Fatalf("缺失 failed_ids 应返回空切片: %+v", ids)
	}
	legacy := []interface{}{
		map[string]interface{}{"id": "2023001", "name": "张三", "reason": "timeout"},
		"invalid",
	}
	ids := progressFailedIDs(map[string]interface{}{"failed_ids": legacy})
	if len(ids) != 1 || ids[0]["id"] != "2023001" {
		t.Fatalf("[]interface{} 兼容解析不符: %+v", ids)
	}
}

func TestCanViewProgress(t *testing.T) {
	rec := map[string]interface{}{"owner_id": 7}
	owner := &model.User{Model: model.Model{ID: 7}}
	other := &model.User{Model: model.Model{ID: 8}}
	systemAdmin := &model.User{Model: model.Model{ID: 99}, Role: "system_admin"}
	schoolAdmin := &model.User{
		Model:     model.Model{ID: 100},
		UserRoles: []model.UserRole{{Role: "school_admin"}},
	}

	if !canViewProgress(rec, owner) {
		t.Fatal("发起人应可查看自己的任务进度")
	}
	if canViewProgress(rec, other) {
		t.Fatal("其他用户不应查看他人任务进度")
	}
	if !canViewProgress(rec, systemAdmin) || !canViewProgress(rec, schoolAdmin) {
		t.Fatal("管理员应可查看任意任务进度")
	}
	if canViewProgress(rec, nil) {
		t.Fatal("未登录用户不应可查看")
	}
	// 无归属信息的记录（owner_id=0）仅管理员可见
	if canViewProgress(map[string]interface{}{}, owner) {
		t.Fatal("owner_id=0 的记录不应对普通用户可见")
	}
}

func TestInitProgressOwnerRoundtripAndCleanup(t *testing.T) {
	taskID := "test-progress-" + t.Name()
	initProgress(taskID, 5, "2025-2026-1", "all", 42)
	rec := getProgress(taskID)
	if rec == nil {
		t.Fatal("进度记录应存在")
	}
	if progressInt(rec, "owner_id") != 42 || progressInt(rec, "total") != 5 {
		t.Fatalf("进度初始字段不符: %+v", rec)
	}

	finished := time.Now().Add(-2 * batchTTL)
	updateProgress(taskID, map[string]interface{}{"status": "completed", "finished_at": &finished})
	cleanupExpiredProgress()
	if getProgress(taskID) != nil {
		t.Fatal("超过 TTL 的进度记录应被清理")
	}
}
