package handler

import (
	"backend-go/internal/middleware"
	"backend-go/internal/service"
	"backend-go/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Setting 系统设置接口
type Setting struct {
	db *gorm.DB
}

func NewSetting(db *gorm.DB) *Setting { return &Setting{db: db} }

// Get 读取系统设置
func (h *Setting) Get(c *gin.Context) {
	response.OK(c, service.TaskSettings(h.db))
}

// Update 保存系统设置（当前仅评教任务人数上限）
func (h *Setting) Update(c *gin.Context) {
	var p struct {
		TaskMaxParticipants *int `json:"task_max_participants"`
	}
	if err := c.ShouldBindJSON(&p); err != nil || p.TaskMaxParticipants == nil {
		badReq(c, "请求参数错误")
		return
	}
	u := middleware.CurrentUser(c)
	if err := service.SetTaskMaxParticipants(h.db, u.ID, *p.TaskMaxParticipants); err != nil {
		badReq(c, err.Error())
		return
	}
	service.LogRecord(h.db, &u.ID, u.Username, "update", "setting", nil, "system_setting",
		map[string]interface{}{"task_max_participants": *p.TaskMaxParticipants})
	response.OKMsg(c, "保存成功", service.TaskSettings(h.db))
}
