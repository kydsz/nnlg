package handler_test

import (
	"net/http"
	"testing"

	"backend-go/internal/model"
	"backend-go/pkg/pwd"
)

// seedStatsAdmin 灌入一位带 stats:view 权限的教师，返回其用户结构。
func seedStatsAdmin(t *testing.T, env *testEnv) *model.User {
	t.Helper()
	env.seedRole(t, "stats_viewer", "统计查看员", 1, []string{"stats:view"})
	hash, _ := pwd.Hash("secret123")
	u := &model.User{UserNo: "S001", Username: "统计员", Role: "stats_viewer", Password: hash, Status: 1}
	env.seedUser(t, u)
	env.seedUserRole(t, u.ID, "stats_viewer")
	return u
}

func TestUnteachedTeachersPairwiseValidation(t *testing.T) {
	env := newTestServer(t)
	u := seedStatsAdmin(t, env)
	// newTestServer 未建 evaluation_task 表，成对传参查询需要
	if err := env.db.AutoMigrate(&model.EvaluationTask{}); err != nil {
		t.Fatalf("补建 evaluation_task 表失败: %v", err)
	}

	t.Run("只传 start_date 返回 400 而非 panic/500", func(t *testing.T) {
		w := env.do(t, http.MethodGet,
			"/api/v1/stats/unteached-teachers?start_date=2026-09-01",
			"", env.authHeader(t, int64(u.ID)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("只传 start_date 应 400, 实际 %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("只传 end_date 返回 400", func(t *testing.T) {
		w := env.do(t, http.MethodGet,
			"/api/v1/stats/unteached-teachers?end_date=2026-12-31",
			"", env.authHeader(t, int64(u.ID)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("只传 end_date 应 400, 实际 %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("非法日期格式返回 400", func(t *testing.T) {
		w := env.do(t, http.MethodGet,
			"/api/v1/stats/unteached-teachers?start_date=abc&end_date=2026-12-31",
			"", env.authHeader(t, int64(u.ID)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("非法日期格式应 400, 实际 %d", w.Code)
		}
	})

	t.Run("成对传参且合法返回 200", func(t *testing.T) {
		w := env.do(t, http.MethodGet,
			"/api/v1/stats/unteached-teachers?start_date=2026-09-01&end_date=2026-12-31",
			"", env.authHeader(t, int64(u.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("成对合法参数应 200, 实际 %d body=%s", w.Code, w.Body.String())
		}
	})
}
