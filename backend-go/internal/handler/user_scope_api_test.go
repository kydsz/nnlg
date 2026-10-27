package handler_test

import (
	"net/http"
	"strconv"
	"testing"

	"backend-go/internal/model"
)

// scopedUserItem 用户列表项（只取断言需要的字段）
type scopedUserItem struct {
	ID        int    `json:"id"`
	UserNo    string `json:"user_no"`
	CollegeID *int   `json:"college_id"`
}

// scopedUserListResp 用户列表响应
type scopedUserListResp struct {
	Code int `json:"code"`
	Data struct {
		List  []scopedUserItem `json:"list"`
		Total int64            `json:"total"`
	} `json:"data"`
}

func hpInt(v int) *int { return &v }

// seedColleges 灌入两所学院（5=文理学院，6=马克思主义学院）
func seedColleges(t *testing.T, env *testEnv) {
	t.Helper()
	colleges := []model.College{
		{Model: model.Model{ID: 5}, Code: "W", Name: "文理学院", Status: 1},
		{Model: model.Model{ID: 6}, Code: "M", Name: "马克思主义学院", Status: 1},
	}
	if err := env.db.Create(&colleges).Error; err != nil {
		t.Fatalf("灌入学院失败: %v", err)
	}
}

// TestUserListScopedToCollegeAPI 端到端校验用户管理的学院收敛：
// 学院管理员列表只回本学院、传外院参数拿不到数据、跨学院详情 403；系统管理员不受限。
func TestUserListScopedToCollegeAPI(t *testing.T) {
	env := newTestServer(t)
	env.seedRole(t, model.RoleCollegeAdmin, "学院管理员", 1, []string{"user:view", "user:create", "user:update"})
	env.seedRole(t, model.RoleTeacher, "教师", 1, []string{"evaluation:view"})
	seedColleges(t, env)

	admin := &model.User{UserNo: "admin", Username: "系统管理员", Role: model.RoleSystemAdmin, Status: 1}
	collegeAdmin := &model.User{UserNo: "ca", Username: "文理管理员", Role: model.RoleCollegeAdmin, CollegeID: hpInt(5), Status: 1}
	teacher5 := &model.User{UserNo: "t5", Username: "文理教师", Role: model.RoleTeacher, CollegeID: hpInt(5), Status: 1}
	teacher6 := &model.User{UserNo: "t6", Username: "马院教师", Role: model.RoleTeacher, CollegeID: hpInt(6), Status: 1}
	for _, u := range []*model.User{admin, collegeAdmin, teacher5, teacher6} {
		env.seedUser(t, u)
	}

	t.Run("学院管理员列表只返回本学院用户", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/users?page=1&page_size=50", "", env.authHeader(t, int64(collegeAdmin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /users 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp scopedUserListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total != 2 || len(resp.Data.List) != 2 {
			t.Fatalf("学院管理员应只见本学院 2 个用户, 得到 total=%d list=%+v", resp.Data.Total, resp.Data.List)
		}
		for _, it := range resp.Data.List {
			if it.CollegeID == nil || *it.CollegeID != 5 {
				t.Fatalf("出现非本学院用户: %+v", it)
			}
		}
	})

	t.Run("学院管理员传外院筛选拿不到数据", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/users?page=1&page_size=50&college_id=6", "", env.authHeader(t, int64(collegeAdmin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /users?college_id=6 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp scopedUserListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total != 0 || len(resp.Data.List) != 0 {
			t.Fatalf("外院筛选应为空, 得到 total=%d list=%+v", resp.Data.Total, resp.Data.List)
		}
	})

	t.Run("学院管理员读跨学院用户详情 403", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/users/"+strconv.Itoa(teacher6.ID), "", env.authHeader(t, int64(collegeAdmin.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("跨学院详情应为 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员读本学院用户详情成功", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/users/"+strconv.Itoa(teacher5.ID), "", env.authHeader(t, int64(collegeAdmin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("本学院详情应为 200, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("系统管理员列表不受限", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/users?page=1&page_size=50", "", env.authHeader(t, int64(admin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /users 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp scopedUserListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total != 4 {
			t.Fatalf("系统管理员应见全部 4 个用户, 得到 %d", resp.Data.Total)
		}
	})
}

// TestStatsSupervisorsRequiresPermissionAPI 督导统计的双权限门：
// 只有 stats:view 的学院管理员被 403；持 stats:view_supervisors 的角色与系统管理员不被拦截。
func TestStatsSupervisorsRequiresPermissionAPI(t *testing.T) {
	env := newTestServer(t)
	env.seedRole(t, model.RoleCollegeAdmin, "学院管理员", 1, []string{"stats:view"})
	env.seedRole(t, model.RoleSchoolAdmin, "学院管理员（兼容）", 1, []string{"stats:view", "stats:view_supervisors"})

	admin := &model.User{UserNo: "admin", Username: "系统管理员", Role: model.RoleSystemAdmin, Status: 1}
	collegeAdmin := &model.User{UserNo: "ca", Username: "文理管理员", Role: model.RoleCollegeAdmin, CollegeID: hpInt(5), Status: 1}
	// school_admin 是 college_admin 的兼容别名，这里借它验证「持码即放行」
	granted := &model.User{UserNo: "sa", Username: "兼容管理员", Role: model.RoleSchoolAdmin, Status: 1}
	for _, u := range []*model.User{admin, collegeAdmin, granted} {
		env.seedUser(t, u)
	}

	t.Run("学院管理员未持 stats:view_supervisors 被拒", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/stats/supervisors?page=1&page_size=10", "", env.authHeader(t, int64(collegeAdmin.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("督导统计应对学院管理员返回 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("持 stats:view_supervisors 的角色不被权限中间件拦截", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/stats/supervisors?page=1&page_size=10", "", env.authHeader(t, int64(granted.ID)))
		// 测试库未迁移统计相关表，业务可能报错；此处只断言权限门放行（不是 403）
		if w.Code == http.StatusForbidden {
			t.Fatalf("持码角色不应被 403 拦截, body=%s", w.Body.String())
		}
	})

	t.Run("系统管理员直通", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/stats/supervisors?page=1&page_size=10", "", env.authHeader(t, int64(admin.ID)))
		if w.Code == http.StatusForbidden {
			t.Fatalf("系统管理员不应被 403 拦截, body=%s", w.Body.String())
		}
	})
}
