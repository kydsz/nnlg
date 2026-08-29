package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// 时间序列化布局（与 Python 端输出对齐）
const (
	// LayoutISO 对齐 Python datetime.isoformat()（秒级，无时区后缀）
	LayoutISO = "2006-01-02T15:04:05"
	// LayoutISOUs 微秒级（Python isoformat 在微秒非零时输出 6 位小数）
	LayoutISOUs = "2006-01-02T15:04:05.000000"
	// LayoutSQL 对齐 Python str(datetime)（空格分隔）
	LayoutSQL = "2006-01-02 15:04:05"
	// LayoutDate 仅日期
	LayoutDate = "2006-01-02"
)

var timeParseLayouts = []string{
	LayoutISO, LayoutSQL, LayoutDate,
	time.RFC3339Nano, time.RFC3339,
	"2006-01-02T15:04:05.999999", "2006-01-02T15:04",
	"2006-01-02 15:04", "15:04", "2006/01/02",
}

// LocalTimePtr 将 time.Time 转为 *LocalTime（构造/赋值场景用）
func LocalTimePtr(t time.Time) *LocalTime { lt := LocalTime(t); return &lt }

// LocalDatePtr 将 time.Time 转为 *LocalDate
func LocalDatePtr(t time.Time) *LocalDate { ld := LocalDate(t); return &ld }

// LocalTime 数据库时间字段类型：序列化对齐 Python datetime 输出。
type LocalTime time.Time

func (t LocalTime) ToTime() time.Time { return time.Time(t) }

// MarshalJSON 输出 2006-01-02T15:04:05（微秒非零时带 6 位小数），零值输出 null
func (t LocalTime) MarshalJSON() ([]byte, error) {
	tt := time.Time(t)
	if tt.IsZero() {
		return []byte("null"), nil
	}
	layout := LayoutISO
	if tt.Nanosecond()/1000 != 0 {
		layout = LayoutISOUs
	}
	return json.Marshal(tt.Format(layout))
}

// UnmarshalJSON 兼容多种时间格式（无时区 ISO / SQL / RFC3339 等）
func (t *LocalTime) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*t = LocalTime(time.Time{})
		return nil
	}
	s = strings.Trim(s, `"`)
	return t.parseString(s)
}

// Value 实现 driver.Valuer，零值写 NULL
func (t LocalTime) Value() (driver.Value, error) {
	tt := time.Time(t)
	if tt.IsZero() {
		return nil, nil
	}
	return tt, nil
}

// Scan 实现 sql.Scanner
func (t *LocalTime) Scan(v any) error {
	if v == nil {
		*t = LocalTime(time.Time{})
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		*t = LocalTime(val)
	case []byte:
		return t.parseString(strings.TrimSpace(string(val)))
	case string:
		return t.parseString(strings.TrimSpace(val))
	default:
		return fmt.Errorf("LocalTime.Scan: 不支持的类型 %T", v)
	}
	return nil
}

func (t *LocalTime) parseString(s string) error {
	if s == "" {
		*t = LocalTime(time.Time{})
		return nil
	}
	for _, layout := range timeParseLayouts {
		if tt, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			*t = LocalTime(tt)
			return nil
		}
	}
	return fmt.Errorf("无法解析时间: %q", s)
}

// LocalDate 数据库日期字段类型：序列化输出 2006-01-02。
type LocalDate time.Time

func (d LocalDate) ToTime() time.Time { return time.Time(d) }

func (d LocalDate) MarshalJSON() ([]byte, error) {
	tt := time.Time(d)
	if tt.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(tt.Format(LayoutDate))
}

func (d *LocalDate) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*d = LocalDate(time.Time{})
		return nil
	}
	s = strings.Trim(s, `"`)
	return d.parseString(s)
}

func (d LocalDate) Value() (driver.Value, error) {
	tt := time.Time(d)
	if tt.IsZero() {
		return nil, nil
	}
	return tt, nil
}

func (d *LocalDate) Scan(v any) error {
	if v == nil {
		*d = LocalDate(time.Time{})
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		*d = LocalDate(val)
	case []byte:
		return d.parseString(strings.TrimSpace(string(val)))
	case string:
		return d.parseString(strings.TrimSpace(val))
	default:
		return fmt.Errorf("LocalDate.Scan: 不支持的类型 %T", v)
	}
	return nil
}

func (d *LocalDate) parseString(s string) error {
	if s == "" {
		*d = LocalDate(time.Time{})
		return nil
	}
	for _, layout := range []string{LayoutDate, LayoutISO, LayoutSQL, "2006/01/02", time.RFC3339} {
		if tt, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			*d = LocalDate(tt)
			return nil
		}
	}
	return fmt.Errorf("无法解析日期: %q", s)
}
