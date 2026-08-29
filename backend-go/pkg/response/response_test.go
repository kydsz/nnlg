package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupRouter(handler gin.HandlerFunc) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/t", handler)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	r.ServeHTTP(w, req)
	return r, w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) Body {
	t.Helper()
	var b Body
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("响应体不是合法 JSON: %v, body=%s", err, w.Body.String())
	}
	return b
}

func TestOK(t *testing.T) {
	_, w := setupRouter(func(c *gin.Context) {
		OK(c, map[string]string{"k": "v"})
	})
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200, 实际 %d", w.Code)
	}
	b := decodeBody(t, w)
	if b.Code != http.StatusOK || b.Message != "success" {
		t.Fatalf("响应不符合约定: %+v", b)
	}
	m, ok := b.Data.(map[string]any)
	if !ok || m["k"] != "v" {
		t.Fatalf("data 不符: %+v", b.Data)
	}
}

func TestOKMsg(t *testing.T) {
	_, w := setupRouter(func(c *gin.Context) {
		OKMsg(c, "创建成功", nil)
	})
	b := decodeBody(t, w)
	if b.Code != http.StatusOK || b.Message != "创建成功" || b.Data != nil {
		t.Fatalf("响应不符合约定: %+v", b)
	}
}

func TestFail(t *testing.T) {
	_, w := setupRouter(func(c *gin.Context) {
		Fail(c, http.StatusForbidden, "无权限")
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("期望 403, 实际 %d", w.Code)
	}
	b := decodeBody(t, w)
	// 约定：HTTP 状态码 = body.code，data 固定 null
	if b.Code != http.StatusForbidden || b.Message != "无权限" || b.Data != nil {
		t.Fatalf("响应不符合约定: %+v", b)
	}
}

func TestFailValidation(t *testing.T) {
	_, w := setupRouter(func(c *gin.Context) {
		FailValidation(c, []map[string]interface{}{
			{"loc": []string{"body", "username"}, "msg": "field required"},
		})
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("期望 422, 实际 %d", w.Code)
	}
	// data 必须是数组（对齐旧端 pydantic 错误明细），不能是 null 或对象
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("响应体不是合法 JSON: %v", err)
	}
	var details []map[string]any
	if err := json.Unmarshal(raw["data"], &details); err != nil {
		t.Fatalf("data 应为数组: %v, raw=%s", err, raw["data"])
	}
	if len(details) != 1 || details[0]["msg"] != "field required" {
		t.Fatalf("错误明细不符: %+v", details)
	}
	b := decodeBody(t, w)
	if b.Message != "请求参数错误" {
		t.Fatalf("message 不符: %+v", b)
	}
}
