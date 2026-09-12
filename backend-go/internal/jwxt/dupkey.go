package jwxt

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// isDupKeyErr 判断是否为唯一键冲突（工号唯一索引 uk_user_no 等）。
// 与 service.isDupKeyErr 同口径：优先 gorm.ErrDuplicatedKey（驱动已翻译），
// 兜底按错误文本匹配，兼容未被翻译的唯一约束报错原文。
// 导入场景下冲突表示「另一并发实例已插入同一工号」，按已存在处理。
func isDupKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint failed")
}
