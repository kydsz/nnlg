package service

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

// isDupKeyErr 是「唯一索引兜底」的判定入口：识别失败会把并发冲突当成 500 暴露给用户，
// 识别过宽会把普通 DB 故障误判为重复而静默放过，故两侧都要锁住。
func TestIsDupKeyErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"gorm 翻译后的重复键", gorm.ErrDuplicatedKey, true},
		{"MySQL 1062", errors.New("Error 1062 (23000): Duplicate entry 'T001' for key 'uk_user_no'"), true},
		{"SQLite 唯一约束", errors.New("UNIQUE constraint failed: evaluation_task.dedupe_key"), true},
		{"普通 DB 错误", errors.New("connection refused"), false},
		{"业务错误", ErrDuplicateTask, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDupKeyErr(c.err); got != c.want {
				t.Fatalf("isDupKeyErr(%v) = %v, 期望 %v", c.err, got, c.want)
			}
		})
	}
}
