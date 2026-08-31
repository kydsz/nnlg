package handler_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"backend-go/internal/model"
)

// roleItem 角色列表/详情响应中的角色条目。
type roleItem struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Status    int16  `json:"status"`
	UserCount int    `json:"user_count"`
}

// roleListResp 角色列表响应。
type roleListResp struct {
	Code int `json:"code"`
	Data struct {
		List  []roleItem `json:"list"`
		Total int64      `json:"total"`
	} `json:"data"`
}

// roleDetailResp 角色详情/创建响应（data 直接为角色对象）。
type roleDetailResp struct {
	Code int      `json:"code"`
	Data roleItem `json:"data"`
}

// failResp 统一失败响应。
type failResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func decodeJSON(t *testing.T, body []byte, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, string(body))
	}
}

// systemAdminUser 建一个系统管理员用户（恒通过权限校验，聚焦 handler 逻辑）。
func systemAdminUser() *model.User {
	return &model.User{UserNo: "admin", Username: "admin", Role: model.RoleSystemAdmin, Status: 1}
}

func TestRoleListAsSystemAdmin(t *testing.T) {
	env := newTestServer(t)
	env.seedRole(t, "role_a", "角色A", 1, nil)
	env.seedRole(t, "role_b", "角色B", 0, nil)
	admin := systemAdminUser()
	env.seedUser(t, admin)

	w := env.do(t, http.MethodGet, "/api/v1/roles", "", env.authHeader(t, int64(admin.ID)))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /roles 状态码=%d, body=%s", w.Code, w.Body.String())
	}

	var resp roleListResp
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != http.StatusOK {
		t.Fatalf("响应 code=%d, body=%s", resp.Code, w.Body.String())
	}
	// 系统管理员可见全部角色（含禁用）
	if resp.Data.Total != 2 || len(resp.Data.List) != 2 {
		t.Fatalf("管理员应看到 2 个角色, 得到 total=%d len=%d", resp.Data.Total, len(resp.Data.List))
	}
}

func TestRoleListOnlyEnabledForNonAdmin(t *testing.T) {
	env := newTestServer(t)
	env.seedRole(t, "role_a", "角色A", 1, nil)
	env.seedRole(t, "role_b", "角色B", 0, nil)
	// 授信角色：持有 user:view，且状态启用
	env.seedRole(t, "granted", "受信角色", 1, []string{"user:view"})
	u := &model.User{UserNo: "U001", Username: "受信用户", Role: "granted", Status: 1}
	env.seedUser(t, u)

	w := env.do(t, http.MethodGet, "/api/v1/roles", "", env.authHeader(t, int64(u.ID)))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /roles 状态码=%d, body=%s", w.Code, w.Body.String())
	}

	var resp roleListResp
	decodeJSON(t, w.Body.Bytes(), &resp)
	// 非管理员只应看到启用的角色（role_a、granted），不含禁用的 role_b
	if resp.Data.Total != 2 || len(resp.Data.List) != 2 {
		t.Fatalf("非管理员应看到 2 个启用角色, 得到 total=%d len=%d list=%+v", resp.Data.Total, len(resp.Data.List), resp.Data.List)
	}
	codes := map[string]bool{}
	for _, it := range resp.Data.List {
		codes[it.Code] = true
	}
	if !codes["role_a"] || !codes["granted"] {
		t.Fatalf("应包含启用的 role_a/granted, 得到 %+v", resp.Data.List)
	}
	if codes["role_b"] {
		t.Fatalf("禁用角色 role_b 不应出现在列表中, 得到 %+v", resp.Data.List)
	}
}

func TestRolePermissionDenied(t *testing.T) {
	env := newTestServer(t)
	// teacher 无 user:view 权限（role 表无对应条目），应被权限中间件拒绝
	u := &model.User{UserNo: "T001", Username: "教师", Role: model.RoleTeacher, Status: 1}
	env.seedUser(t, u)

	w := env.do(t, http.MethodGet, "/api/v1/roles", "", env.authHeader(t, int64(u.ID)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("无权限应返回 403, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRoleUnauthorized(t *testing.T) {
	env := newTestServer(t)
	w := env.do(t, http.MethodGet, "/api/v1/roles", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应返回 401, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRoleCreateFlow(t *testing.T) {
	env := newTestServer(t)
	admin := systemAdminUser()
	env.seedUser(t, admin)
	h := env.authHeader(t, int64(admin.ID))

	// 正常创建
	w := env.do(t, http.MethodPost, "/api/v1/roles",
		`{"name":"督导员","code":"supervisor2","permissions":["user:view"],"data_scope":"college"}`, h)
	if w.Code != http.StatusOK {
		t.Fatalf("创建角色状态码=%d, body=%s", w.Code, w.Body.String())
	}
	var created roleDetailResp
	decodeJSON(t, w.Body.Bytes(), &created)
	if created.Data.Code != "supervisor2" || created.Data.Status != 1 {
		t.Fatalf("创建结果不符: %+v", created.Data)
	}

	// 重复编码应报错
	w = env.do(t, http.MethodPost, "/api/v1/roles", `{"name":"X","code":"supervisor2","permissions":[],"data_scope":"all"}`, h)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("重复编码应返回 400, 得到 %d, body=%s", w.Code, w.Body.String())
	}
	var dup failResp
	decodeJSON(t, w.Body.Bytes(), &dup)
	if dup.Message != "角色编码已存在" {
		t.Fatalf("重复编码错误信息不符: %s", dup.Message)
	}

	// 禁止创建系统管理员角色
	w = env.do(t, http.MethodPost, "/api/v1/roles",
		`{"name":"ADMIN","code":"system_admin","permissions":[],"data_scope":"all"}`, h)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("创建系统管理员应返回 400, 得到 %d, body=%s", w.Code, w.Body.String())
	}

	// 缺少必填字段
	w = env.do(t, http.MethodPost, "/api/v1/roles", `{"name":"只有名称"}`, h)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺必填字段应返回 400, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRoleUpdateAndDelete(t *testing.T) {
	env := newTestServer(t)
	admin := systemAdminUser()
	env.seedUser(t, admin)
	h := env.authHeader(t, int64(admin.ID))

	role := env.seedRole(t, "role_x", "旧名字", 1, []string{"user:view"})

	// 更新名称
	w := env.do(t, http.MethodPut, "/api/v1/roles/"+strconv.Itoa(role.ID), `{"name":"新名字"}`, h)
	if w.Code != http.StatusOK {
		t.Fatalf("更新角色状态码=%d, body=%s", w.Code, w.Body.String())
	}
	w = env.do(t, http.MethodGet, "/api/v1/roles/"+strconv.Itoa(role.ID), "", h)
	if w.Code != http.StatusOK {
		t.Fatalf("读取角色状态码=%d, body=%s", w.Code, w.Body.String())
	}
	var detail roleDetailResp
	decodeJSON(t, w.Body.Bytes(), &detail)
	if detail.Data.Name != "新名字" {
		t.Fatalf("更新后名称=%q, 应为 新名字", detail.Data.Name)
	}

	// 删除后应 404
	w = env.do(t, http.MethodDelete, "/api/v1/roles/"+strconv.Itoa(role.ID), "", h)
	if w.Code != http.StatusOK {
		t.Fatalf("删除角色状态码=%d, body=%s", w.Code, w.Body.String())
	}
	w = env.do(t, http.MethodGet, "/api/v1/roles/"+strconv.Itoa(role.ID), "", h)
	if w.Code != http.StatusNotFound {
		t.Fatalf("删除后应返回 404, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRoleDeleteInUseRejected(t *testing.T) {
	env := newTestServer(t)
	admin := systemAdminUser()
	env.seedUser(t, admin)
	h := env.authHeader(t, int64(admin.ID))

	role := env.seedRole(t, "role_in_use", "使用中", 1, nil)
	// 建一条用户-角色关联，使删除保护生效（服务按 UserRole 表计数）
	u := &model.User{UserNo: "U002", Username: "挂角色用户", Status: 1}
	env.seedUser(t, u)
	env.seedUserRole(t, u.ID, "role_in_use")

	w := env.do(t, http.MethodDelete, "/api/v1/roles/"+strconv.Itoa(role.ID), "", h)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("删除使用中角色应返回 400, 得到 %d, body=%s", w.Code, w.Body.String())
	}
	var resp failResp
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Message != "该角色仍被用户使用，禁止删除" {
		t.Fatalf("删除保护信息不符: %s", resp.Message)
	}
}

func TestRolePermissionGroups(t *testing.T) {
	env := newTestServer(t)
	admin := systemAdminUser()
	env.seedUser(t, admin)

	w := env.do(t, http.MethodGet, "/api/v1/roles/permissions", "", env.authHeader(t, int64(admin.ID)))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /roles/permissions 状态码=%d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			Group       string `json:"group"`
			Permissions []struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"permissions"`
		} `json:"data"`
	}
	decodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != http.StatusOK || len(resp.Data) == 0 {
		t.Fatalf("权限目录为空或 code 异常: %+v", resp)
	}
	found := false
	for _, g := range resp.Data {
		if g.Group == "用户管理" {
			found = true
			if len(g.Permissions) == 0 {
				t.Fatalf("用户管理组不应为空权限")
			}
		}
	}
	if !found {
		t.Fatalf("权限目录应包含「用户管理」分组, 得到 %+v", resp.Data)
	}
}

