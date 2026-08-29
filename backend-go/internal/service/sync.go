package service

import (
	"encoding/json"
	"sync"
	"time"

	"backend-go/internal/model"

	"gorm.io/gorm"
)

// Sync 数据同步（教务系统）业务框架
//
// 教务系统爬虫（登录/抓取）依赖学校内网 JWXT 接口，Go 版仅实现
// 同步调度框架（互斥锁 + operation_log 状态机 + 状态查询），
// 抓取器未配置 JWXT_BASE_URL_* 时同步接口返回明确错误。
type Sync struct{}

func NewSync() *Sync { return &Sync{} }

var (
	syncMu       sync.Mutex
	syncRunning  = map[string]bool{}
	syncProgress = map[string]map[string]interface{}{} // module -> progress
)

// Acquire 获取模块同步锁
func (s *Sync) Acquire(module string) bool {
	syncMu.Lock()
	defer syncMu.Unlock()
	if syncRunning[module] {
		return false
	}
	syncRunning[module] = true
	return true
}

// Release 释放模块同步锁
func (s *Sync) Release(module string) {
	syncMu.Lock()
	defer syncMu.Unlock()
	delete(syncRunning, module)
}

// SetProgress 记录模块进度（内存态，进度查询用）
func (s *Sync) SetProgress(module string, progress map[string]interface{}) {
	syncMu.Lock()
	defer syncMu.Unlock()
	syncProgress[module] = progress
}

// GetProgress 读取模块进度
func (s *Sync) GetProgress(module string) map[string]interface{} {
	syncMu.Lock()
	defer syncMu.Unlock()
	p, ok := syncProgress[module]
	if !ok {
		return map[string]interface{}{}
	}
	return p
}

// RecordStart 记录同步开始（写 operation_log，content.status=running）
func (s *Sync) RecordStart(db *gorm.DB, userID *int, username, module string) (int, error) {
	content, _ := json.Marshal(map[string]interface{}{"status": "running", "started_at": time.Now().Format("2006-01-02 15:04:05")})
	// CreateTime/UpdateTime 必须显式写入：nil 时 GORM 会向列写 NULL，绕过
	// MySQL server_default=CURRENT_TIMESTAMP，导致 LastStatus 的 last_sync 为 null
	now := model.LocalTimePtr(time.Now())
	logRow := model.OperationLog{
		UserID: userID, UserName: username, OperationType: "sync", Module: module, Content: content,
		Model: model.Model{CreateTime: now, UpdateTime: now},
	}
	if err := db.Create(&logRow).Error; err != nil {
		return 0, err
	}
	return logRow.ID, nil
}

// RecordEnd 记录同步结束（success/failed + 说明与结果数据）
func (s *Sync) RecordEnd(db *gorm.DB, logID int, status, message string, result map[string]interface{}) {
	content := map[string]interface{}{"status": status, "message": message}
	for k, v := range result {
		content[k] = v
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return
	}
	db.Model(&model.OperationLog{}).Where("id = ?", logID).Update("content", raw)
}

// LastStatus 最近一次同步状态（对齐旧端 GET /teachers/sync-status：{last_sync, status}）
func (s *Sync) LastStatus(db *gorm.DB, module string) map[string]interface{} {
	var logRow model.OperationLog
	err := db.Where("module = ? AND operation_type = ?", module, "sync").
		Order("create_time DESC, id DESC").First(&logRow).Error

	status := "idle"
	var lastSync interface{}
	var content map[string]interface{}
	if err == nil {
		if logRow.CreateTime != nil {
			// Python isoformat()：2006-01-02T15:04:05
			lastSync = logRow.CreateTime.ToTime().Format("2006-01-02T15:04:05")
		}
		if len(logRow.Content) > 0 {
			_ = json.Unmarshal(logRow.Content, &content)
		}
		switch content["status"] {
		case "running":
			status = "running"
		case "success":
			status = "success"
		case "failed":
			status = "failed"
		}
	}
	return map[string]interface{}{
		"last_sync": lastSync, "status": status,
	}
}
