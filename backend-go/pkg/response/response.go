package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 统一响应格式，与前端现有约定一致：{code, message, data}
type Body struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{Code: http.StatusOK, Message: "success", Data: data})
}

func OKMsg(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Body{Code: http.StatusOK, Message: message, Data: data})
}

// Fail data 固定为 null
func Fail(c *gin.Context, httpCode int, message string) {
	c.JSON(httpCode, Body{Code: httpCode, Message: message, Data: nil})
}

// FailValidation 422 验证错误，data 为 pydantic 风格错误明细数组（对齐旧端 RequestValidationError）
func FailValidation(c *gin.Context, details []map[string]interface{}) {
	c.JSON(http.StatusUnprocessableEntity, Body{
		Code: http.StatusUnprocessableEntity, Message: "请求参数错误", Data: details,
	})
}
