package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend-go/internal/model"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	// 旧端 FastAPI 输出 datetime 为 isoformat（T 分隔、含秒）
	timeLayoutFull = "2006-01-02T15:04:05"
	timeLayoutMin  = "2006-01-02T15:04"
	timeLayoutDay  = "2006-01-02"
)

// FTime 全格式时间输出
func FTime(t *model.LocalTime) interface{} {
	if t == nil {
		return nil
	}
	return t.ToTime().Format(timeLayoutFull)
}

// FTimeMin 分钟精度输出（兼容调用点；旧端 isoformat 含秒，故同样输出全格式）
func FTimeMin(t *model.LocalTime) interface{} {
	if t == nil {
		return nil
	}
	return t.ToTime().Format(timeLayoutFull)
}

// parseTimePtr 多格式时间解析
func parseTimePtr(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	for _, l := range []string{timeLayoutFull, timeLayoutMin, timeLayoutDay, time.RFC3339} {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, errors.New("时间格式错误: " + s)
}

// pageOf 解析分页参数
func pageOf(c *gin.Context) (int, int) {
	return pageOfDefault(c, 20)
}

// pageOfDefault 解析分页参数（自定义默认每页数量，与旧端对齐用）
func pageOfDefault(c *gin.Context, def int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(def)))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 1000 {
		size = def
	}
	return page, size
}

// pageData 统一分页响应结构（对齐旧端 {list,total,page,page_size}）
func pageData(c *gin.Context, list interface{}, total int64, pageSize int) gin.H {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	return gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}
}

func qInt(c *gin.Context, key string) *int {
	if v, ok := c.GetQuery(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return &n
		}
	}
	return nil
}

func qInt16(c *gin.Context, key string) *int16 {
	if n := qInt(c, key); n != nil {
		v := int16(*n)
		return &v
	}
	return nil
}

func qBool(c *gin.Context, key string) *bool {
	if v, ok := c.GetQuery(key); ok && v != "" {
		b := v == "true" || v == "1"
		return &b
	}
	return nil
}

func collegeName(c *model.College) interface{} {
	if c == nil {
		return nil
	}
	return c.Name
}

func roomName(r *model.ResearchRoom) interface{} {
	if r == nil {
		return nil
	}
	return r.Name
}

func badReq(c *gin.Context, msg string) {
	response.Fail(c, http.StatusBadRequest, msg)
}

// missingQuery 缺失必填 query 参数：422 + pydantic 风格明细（对齐旧端）
func missingQuery(c *gin.Context, name string) {
	response.FailValidation(c, []map[string]interface{}{{
		"type":  "missing",
		"loc":   []string{"query", name},
		"msg":   "Field required",
		"input": nil,
	}})
}

// invalidParam query/path 参数解析失败：422 + pydantic 风格明细
func invalidParam(c *gin.Context, loc, name, input, msg string) {
	response.FailValidation(c, []map[string]interface{}{{
		"type":  typeOfParseError(msg),
		"loc":   []string{loc, name},
		"msg":   msg,
		"input": input,
	}})
}

func typeOfParseError(msg string) string {
	if strings.Contains(msg, "integer") {
		return "int_parsing"
	}
	return "value_error"
}

// svcErr 按业务错误携带的 HTTP 状态码返回（默认 400）
func svcErr(c *gin.Context, err error) {
	if code, ok := service.AsHTTPError(err); ok {
		response.Fail(c, code, err.Error())
		return
	}
	badReq(c, err.Error())
}

func forbidden(c *gin.Context, msg string) {
	response.Fail(c, http.StatusForbidden, msg)
}

func serverErr(c *gin.Context, msg string) {
	response.Fail(c, http.StatusInternalServerError, msg)
}

// keysOf 取 map 的 key 列表
func keysOf[T comparable, V any](m map[T]V) []T {
	out := make([]T, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
