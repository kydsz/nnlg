package handler_test

import (
	"net/http"
	"testing"

	"backend-go/internal/model"
)

// TestSemesterConfigWriteRequiresSystemAdminAPI 学期配置是校级配置：
// 仅系统管理员可维护，学院管理员（即便持 org:view / schedule:view）只读查询。
func TestSemesterConfigWriteRequiresSystemAdminAPI(t *testing.T) {
	env := newTestServer(t)
	if err := env.db.AutoMigrate(&model.SemesterConfig{}); err != nil {
		t.Fatalf("建学期配置表失败: %v", err)
	}
	// 旧门禁是 org:view：这里特意授给学院管理员，验证其已不足以写学期配置
	env.seedRole(t, model.RoleCollegeAdmin, "学院管理员", 1, []string{"schedule:view", "org:view"})
	seedColleges(t, env)

	admin := systemAdminUser()
	ca := &model.User{UserNo: "ca", Username: "文理管理员", Role: model.RoleCollegeAdmin, CollegeID: hpInt(5), Status: 1}
	env.seedUser(t, admin)
	env.seedUser(t, ca)

	body := `{"semester":"2026-2027-1","start_date":"2026-09-01","weeks":20,"is_current":true}`

	t.Run("学院管理员新建学期配置 403", func(t *testing.T) {
		w := env.do(t, http.MethodPost, "/api/v1/course-schedules/semester-configs", body, env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("学院管理员新建学期配置应为 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员改删学期配置 403", func(t *testing.T) {
		w := env.do(t, http.MethodPut, "/api/v1/course-schedules/semester-configs/2026-2027-1",
			`{"weeks":18}`, env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("学院管理员改学期配置应为 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
		w = env.do(t, http.MethodDelete, "/api/v1/course-schedules/semester-configs/2026-2027-1", "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("学院管理员删学期配置应为 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员只读查询放行", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/course-schedules/semester-configs", "", env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusForbidden {
			t.Fatalf("查询学期配置不应被 403 拦截, body=%s", w.Body.String())
		}
	})

	t.Run("系统管理员可维护", func(t *testing.T) {
		w := env.do(t, http.MethodPost, "/api/v1/course-schedules/semester-configs", body, env.authHeader(t, int64(admin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("系统管理员新建学期配置应成功, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})
}
