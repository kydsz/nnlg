package model

import (
	"encoding/json"
	"testing"
	"time"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return string(b)
}

// ---------- LocalTime ----------

func TestLocalTimeMarshalZeroIsNull(t *testing.T) {
	var lt LocalTime
	if got := mustJSON(t, lt); got != "null" {
		t.Fatalf("零值应输出 null, 实际 %s", got)
	}
	// 指针零值同样输出 null（DB 可空字段约定）
	var p *LocalTime
	if got := mustJSON(t, p); got != "null" {
		t.Fatalf("nil 指针应输出 null, 实际 %s", got)
	}
}

func TestLocalTimeMarshalFormats(t *testing.T) {
	seconds := LocalTime(time.Date(2024, 5, 1, 8, 30, 5, 0, time.Local))
	if got := mustJSON(t, seconds); got != `"2024-05-01T08:30:05"` {
		t.Fatalf("秒级输出不符: %s", got)
	}
	// 微秒非零 → 6 位小数（对齐 Python isoformat）
	micros := LocalTime(time.Date(2024, 5, 1, 8, 30, 5, 123456000, time.Local))
	if got := mustJSON(t, micros); got != `"2024-05-01T08:30:05.123456"` {
		t.Fatalf("微秒级输出不符: %s", got)
	}
	// 纳秒但微秒为零（如 .0000001）不应输出小数
	nano := LocalTime(time.Date(2024, 5, 1, 8, 30, 5, 100, time.Local))
	if got := mustJSON(t, nano); got != `"2024-05-01T08:30:05"` {
		t.Fatalf("亚微秒输出不符: %s", got)
	}
}

func TestLocalTimeUnmarshalFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string // 以秒级布局重新序列化的期望值
	}{
		{`"2024-05-01T08:30:05"`, `"2024-05-01T08:30:05"`},
		{`"2024-05-01 08:30:05"`, `"2024-05-01T08:30:05"`},      // SQL 布局
		{`"2024-05-01"`, `"2024-05-01T00:00:00"`},               // 仅日期
		{`"2024/05/01"`, `"2024-05-01T00:00:00"`},               // 斜杠日期
		{`"2024-05-01T08:30:05+08:00"`, `"2024-05-01T08:30:05"`}, // RFC3339
		{`"2024-05-01T08:30"`, `"2024-05-01T08:30:00"`},         // 精确到分
		{`"2024-05-01T08:30:05.5"`, `"2024-05-01T08:30:05.500000"`}, // Python 微秒风格
	}
	for _, c := range cases {
		var lt LocalTime
		if err := json.Unmarshal([]byte(c.in), &lt); err != nil {
			t.Fatalf("解析 %s 失败: %v", c.in, err)
		}
		if got := mustJSON(t, lt); got != c.want {
			t.Fatalf("输入 %s: 期望 %s, 实际 %s", c.in, c.want, got)
		}
	}
}

func TestLocalTimeUnmarshalNullAndEmpty(t *testing.T) {
	var lt LocalTime
	if err := json.Unmarshal([]byte("null"), &lt); err != nil {
		t.Fatalf("null 应可解析: %v", err)
	}
	if !time.Time(lt).IsZero() {
		t.Fatal("null 应解析为零值")
	}
	if err := json.Unmarshal([]byte(`""`), &lt); err != nil {
		t.Fatalf("空串应可解析: %v", err)
	}
	if !time.Time(lt).IsZero() {
		t.Fatal("空串应解析为零值")
	}
}

func TestLocalTimeUnmarshalInvalid(t *testing.T) {
	var lt LocalTime
	if err := json.Unmarshal([]byte(`"not-a-time"`), &lt); err == nil {
		t.Fatal("非法时间应报错")
	}
}

func TestLocalTimeValueScan(t *testing.T) {
	// 零值写库 → NULL
	var zero LocalTime
	if v, err := zero.Value(); err != nil || v != nil {
		t.Fatalf("零值应写 NULL: v=%v err=%v", v, err)
	}
	tt := time.Date(2024, 5, 1, 8, 30, 5, 0, time.Local)
	lt := LocalTime(tt)
	v, err := lt.Value()
	if err != nil {
		t.Fatalf("Value 失败: %v", err)
	}
	if !v.(time.Time).Equal(tt) {
		t.Fatalf("Value 不符: %v", v)
	}

	// Scan：nil / time.Time / string / []byte / 不支持类型
	var dst LocalTime
	if err := dst.Scan(nil); err != nil || !time.Time(dst).IsZero() {
		t.Fatalf("Scan(nil) 应得零值: %v", err)
	}
	if err := dst.Scan(tt); err != nil || !time.Time(dst).Equal(tt) {
		t.Fatalf("Scan(time.Time) 失败: %v", err)
	}
	if err := dst.Scan("2024-05-01 08:30:05"); err != nil || !time.Time(dst).Equal(tt) {
		t.Fatalf("Scan(string) 失败: %v", err)
	}
	if err := dst.Scan([]byte("2024-05-01T08:30:05")); err != nil || !time.Time(dst).Equal(tt) {
		t.Fatalf("Scan([]byte) 失败: %v", err)
	}
	if err := dst.Scan(12345); err == nil {
		t.Fatal("Scan(不支持类型) 应报错")
	}
}

// ---------- LocalDate ----------

func TestLocalDateMarshalUnmarshal(t *testing.T) {
	var zero LocalDate
	if got := mustJSON(t, zero); got != "null" {
		t.Fatalf("零值应输出 null, 实际 %s", got)
	}
	d := LocalDate(time.Date(2024, 5, 1, 23, 59, 59, 0, time.Local))
	if got := mustJSON(t, d); got != `"2024-05-01"` {
		t.Fatalf("日期输出不符: %s", got)
	}

	var parsed LocalDate
	if err := json.Unmarshal([]byte(`"2024-05-01"`), &parsed); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := mustJSON(t, parsed); got != `"2024-05-01"` {
		t.Fatalf("往返不一致: %s", got)
	}
	if err := json.Unmarshal([]byte(`"2024-05-01T08:30:00"`), &parsed); err != nil {
		t.Fatalf("应兼容 ISO 时间格式: %v", err)
	}
	if err := json.Unmarshal([]byte(`"2024/05/01"`), &parsed); err != nil {
		t.Fatalf("应兼容斜杠格式: %v", err)
	}
	if err := json.Unmarshal([]byte(`"bad"`), &parsed); err == nil {
		t.Fatal("非法日期应报错")
	}

	// Value/Scan 与 LocalTime 同约定
	if v, err := zero.Value(); err != nil || v != nil {
		t.Fatalf("零值应写 NULL: v=%v err=%v", v, err)
	}
	var dst LocalDate
	if err := dst.Scan(nil); err != nil || !time.Time(dst).IsZero() {
		t.Fatalf("Scan(nil) 应得零值: %v", err)
	}
	if err := dst.Scan("2024-05-01"); err != nil {
		t.Fatalf("Scan(string) 失败: %v", err)
	}
	if got := mustJSON(t, dst); got != `"2024-05-01"` {
		t.Fatalf("Scan 后序列化不符: %s", got)
	}
}

func TestTimePtrHelpers(t *testing.T) {
	tt := time.Date(2024, 5, 1, 8, 30, 5, 0, time.Local)
	lt := LocalTimePtr(tt)
	if !lt.ToTime().Equal(tt) {
		t.Fatal("LocalTimePtr 转换不符")
	}
	ld := LocalDatePtr(tt)
	if got := mustJSON(t, ld); got != `"2024-05-01"` {
		t.Fatalf("LocalDatePtr 输出不符: %s", got)
	}
}
