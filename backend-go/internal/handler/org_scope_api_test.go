package handler_test

import (
	"net/http"
	"strconv"
	"testing"

	"backend-go/internal/model"
)

// orgItem 组织架构列表项（只取断言需要的字段）
type orgItem struct {
	ID        int `json:"id"`
	CollegeID int `json:"college_id"`
}

// orgListResp 组织架构列表响应
type orgListResp struct {
	Code int `json:"code"`
	Data struct {
		List  []orgItem `json:"list"`
		Total int64     `json:"total"`
	} `json:"data"`
}

// orgFailItem 失败响应（只看 code）
type orgFailItem struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// seedOrgScope 灌入学院管理员（主学院5，user_college 挂学院6）与两学院各一个教研室
func seedOrgScope(t *testing.T, env *testEnv) (*model.User, *model.User, *model.ResearchRoom, *model.ResearchRoom) {
	t.Helper()
	env.seedRole(t, model.RoleCollegeAdmin, "学院管理员", 1,
		[]string{"org:view", "college:manage", "research_room:manage"})
	env.seedRole(t, model.RoleTeacher, "教师", 1, nil)
	seedColleges(t, env)

	ca := &model.User{UserNo: "ca", Username: "文理管理员", Role: model.RoleCollegeAdmin, CollegeID: hpInt(5), Status: 1}
	teacher := &model.User{UserNo: "t5", Username: "文理教师", Role: model.RoleTeacher, CollegeID: hpInt(5), Status: 1}
	env.seedUser(t, ca)
	env.seedUser(t, teacher)
	// 督导「负责学院」挂在学院6：不得据此把可见/可管理范围扩大到院外
	if err := env.db.Create(&model.UserCollege{UserID: ca.ID, CollegeID: 6}).Error; err != nil {
		t.Fatalf("灌入负责学院失败: %v", err)
	}
	room5 := &model.ResearchRoom{Code: "R5", Name: "文理教研室", CollegeID: 5, Status: 1}
	room6 := &model.ResearchRoom{Code: "R6", Name: "马院教研室", CollegeID: 6, Status: 1}
	for _, r := range []*model.ResearchRoom{room5, room6} {
		if err := env.db.Create(r).Error; err != nil {
			t.Fatalf("灌入教研室失败: %v", err)
		}
	}
	return ca, teacher, room5, room6
}

// TestCollegeScopeAPI 学院读写同权：学院管理员只能查看/管理本学院，
// 督导「负责学院」不扩大范围，新建学院仅系统管理员。
func TestCollegeScopeAPI(t *testing.T) {
	env := newTestServer(t)
	ca, _, _, _ := seedOrgScope(t, env)
	admin := systemAdminUser()
	env.seedUser(t, admin)

	t.Run("学院管理员列表只返回本学院", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/colleges", "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /colleges 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp orgListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total != 1 || len(resp.Data.List) != 1 || resp.Data.List[0].ID != 5 {
			t.Fatalf("学院管理员应只见本学院, 得到 %+v", resp.Data)
		}
	})

	t.Run("学院管理员读外院详情 403", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/colleges/6", "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("外院详情应为 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员读本学院详情成功", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/colleges/5", "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("本学院详情应为 200, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员改外院被拒", func(t *testing.T) {
		w := env.do(t, http.MethodPut, "/api/v1/colleges/6", `{"name":"改名"}`, env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusOK {
			t.Fatalf("改外院应被拒绝, body=%s", w.Body.String())
		}
	})

	t.Run("学院管理员改本学院成功", func(t *testing.T) {
		w := env.do(t, http.MethodPut, "/api/v1/colleges/5", `{"name":"文理学院"}`, env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("改本学院应成功, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员删外院被拒", func(t *testing.T) {
		w := env.do(t, http.MethodDelete, "/api/v1/colleges/6", "", env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusOK {
			t.Fatalf("删外院应被拒绝, body=%s", w.Body.String())
		}
	})

	t.Run("学院管理员不可新建学院", func(t *testing.T) {
		w := env.do(t, http.MethodPost, "/api/v1/colleges",
			`{"code":"NEW","name":"新学院"}`, env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusOK {
			t.Fatalf("新建学院应被拒绝, body=%s", w.Body.String())
		}
		var resp orgFailItem
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Message != "仅系统管理员可新建学院" {
			t.Fatalf("错误信息不符: %s", resp.Message)
		}
	})

	t.Run("系统管理员不受限", func(t *testing.T) {
		w := env.do(t, http.MethodPost, "/api/v1/colleges",
			`{"code":"NEW","name":"新学院"}`, env.authHeader(t, int64(admin.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("系统管理员新建学院应成功, 得到 %d, body=%s", w.Code, w.Body.String())
		}
		w = env.do(t, http.MethodGet, "/api/v1/colleges", "", env.authHeader(t, int64(admin.ID)))
		var resp orgListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total != 3 {
			t.Fatalf("系统管理员应见全部 3 个学院, 得到 %d", resp.Data.Total)
		}
	})
}

// TestResearchRoomScopeAPI 教研室读写同权：学院管理员只能查看/管理本学院教研室；
// 教师自助改教研室的列表同样收敛到所属学院。
func TestResearchRoomScopeAPI(t *testing.T) {
	env := newTestServer(t)
	ca, teacher, room5, room6 := seedOrgScope(t, env)
	admin := systemAdminUser()
	env.seedUser(t, admin)

	t.Run("学院管理员列表只返回本学院教研室", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/research-rooms", "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /research-rooms 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp orgListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total != 1 || len(resp.Data.List) != 1 || resp.Data.List[0].ID != room5.ID {
			t.Fatalf("学院管理员应只见本学院教研室, 得到 %+v", resp.Data)
		}
	})

	t.Run("学院管理员读外院教研室详情 403", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/research-rooms/"+strconv.Itoa(room6.ID), "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("外院教研室详情应为 403, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员只能在本学院新建教研室", func(t *testing.T) {
		w := env.do(t, http.MethodPost, "/api/v1/research-rooms",
			`{"code":"R5B","name":"文理第二教研室","college_id":6}`, env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusOK {
			t.Fatalf("在外院新建教研室应被拒绝, body=%s", w.Body.String())
		}
		w = env.do(t, http.MethodPost, "/api/v1/research-rooms",
			`{"code":"R5B","name":"文理第二教研室","college_id":5}`, env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("在本学院新建教研室应成功, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("学院管理员改删外院教研室被拒", func(t *testing.T) {
		w := env.do(t, http.MethodPut, "/api/v1/research-rooms/"+strconv.Itoa(room6.ID),
			`{"name":"改名"}`, env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusOK {
			t.Fatalf("改外院教研室应被拒绝, body=%s", w.Body.String())
		}
		w = env.do(t, http.MethodDelete, "/api/v1/research-rooms/"+strconv.Itoa(room6.ID), "", env.authHeader(t, int64(ca.ID)))
		if w.Code == http.StatusOK {
			t.Fatalf("删外院教研室应被拒绝, body=%s", w.Body.String())
		}
	})

	t.Run("学院管理员改删本学院教研室成功", func(t *testing.T) {
		w := env.do(t, http.MethodPut, "/api/v1/research-rooms/"+strconv.Itoa(room5.ID),
			`{"name":"文理教研室"}`, env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("改本学院教研室应成功, 得到 %d, body=%s", w.Code, w.Body.String())
		}
		w = env.do(t, http.MethodDelete, "/api/v1/research-rooms/"+strconv.Itoa(room5.ID), "", env.authHeader(t, int64(ca.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("删本学院教研室应成功, 得到 %d, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("教师教研室列表收敛到所属学院", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/research-rooms", "", env.authHeader(t, int64(teacher.ID)))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /research-rooms 状态码=%d, body=%s", w.Code, w.Body.String())
		}
		var resp orgListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total == 0 {
			t.Fatalf("教师应能见到所属学院教研室, 得到 %+v", resp.Data)
		}
		for _, it := range resp.Data.List {
			if it.CollegeID != 5 || it.ID == room6.ID {
				t.Fatalf("教师只应见到所属学院的教研室, 得到 %+v", it)
			}
		}
	})

	t.Run("系统管理员不受限", func(t *testing.T) {
		w := env.do(t, http.MethodGet, "/api/v1/research-rooms", "", env.authHeader(t, int64(admin.ID)))
		var resp orgListResp
		decodeJSON(t, w.Body.Bytes(), &resp)
		if resp.Data.Total < 2 {
			t.Fatalf("系统管理员应见全部教研室, 得到 %+v", resp.Data)
		}
	})
}
