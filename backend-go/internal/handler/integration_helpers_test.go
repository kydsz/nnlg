package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend-go/internal/config"
	"backend-go/internal/model"
	"backend-go/internal/router"
	"backend-go/pkg/jwtutil"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// itSecret 集成测试专用 JWT 密钥（长度需 >=16，防止误用 config.Load 校验之外的口径测试）
const itSecret = "integration-test-secret-0123456789"

// testEnv 集成测试环境：内存库 + 全量路由
type testEnv struct {
	r  *gin.Engine
	db *gorm.DB
}

// newTestServer 构建内存 SQLite 库并迁移角色业务所需的最小表集合，
// 再挂载真实路由（router.Setup），返回可直接发请求的测试环境。
func newTestServer(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.UserRole{}, &model.UserCollege{}, &model.UserRoom{},
		&model.College{}, &model.ResearchRoom{}, &model.Role{}, &model.OperationLog{},
	); err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	cfg := &config.Config{
		SecretKey:   itSecret,
		AppName:     "test",
		Version:     "test",
		Debug:       false,
		CORSOrigins: []string{"*"},
	}
	return &testEnv{r: router.Setup(cfg, db), db: db}
}

// seedUser 灌入用户（角色走 UserRole 关联或 legacy Role 字段，均被 Auth 预加载覆盖）
func (e *testEnv) seedUser(t *testing.T, u *model.User) {
	t.Helper()
	if err := e.db.Create(u).Error; err != nil {
		t.Fatalf("灌入用户失败: %v", err)
	}
}

// seedRole 灌入角色。
func (e *testEnv) seedRole(t *testing.T, code string, name string, status int16, perms []string) model.Role {
	t.Helper()
	raw, _ := marshalJSONForTest(perms)
	r := model.Role{
		Name:        name,
		Code:        code,
		Status:      status,
		Level:       100,
		Permissions: raw,
		DataScope:   "all",
	}
	if err := e.db.Create(&r).Error; err != nil {
		t.Fatalf("灌入角色失败: %v", err)
	}
	return r
}

// seedUserRole 灌入用户-角色关联（删除保护、权限合并依赖该表）
func (e *testEnv) seedUserRole(t *testing.T, userID int, role string) {
	t.Helper()
	if err := e.db.Create(&model.UserRole{UserID: userID, Role: role}).Error; err != nil {
		t.Fatalf("灌入用户角色失败: %v", err)
	}
}

// tokenOf 签发指定用户 ID 的 JWT（含 Bearer 前缀）。
func (e *testEnv) tokenOf(t *testing.T, userID int64) string {
	t.Helper()
	s, err := jwtutil.Sign(itSecret, userID, 60)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	return "Bearer " + s
}

// authHeader 构造带 Bearer token 的请求头。
func (e *testEnv) authHeader(t *testing.T, userID int64) http.Header {
	t.Helper()
	h := http.Header{}
	h.Set("Authorization", e.tokenOf(t, userID))
	return h
}

// do 以给定方法/路径/请求体/请求头发送请求，返回响应记录器。
func (e *testEnv) do(t *testing.T, method, path string, body string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w
}

func marshalJSONForTest(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
