package jwxt

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// 按字节截断会把多字节字符切一半，写库/日志出现非法 UTF-8（原 raw[:200] 的问题）
func TestTruncateRunesKeepsValidUTF8(t *testing.T) {
	raw := strings.Repeat("中文", 100) // 600 字节 / 200 字符
	got := truncateRunes(raw, 200)
	if got != raw {
		t.Fatalf("200 字符以内应原样返回, 实际长度 %d", utf8.RuneCountInString(got))
	}

	raw = strings.Repeat("中文", 150) // 450 字节 / 300 字符
	got = truncateRunes(raw, 200)
	if !utf8.ValidString(got) {
		t.Fatal("截断结果必须是合法 UTF-8")
	}
	if n := utf8.RuneCountInString(got); n != 200 {
		t.Fatalf("应按字符截到 200, 实际 %d", n)
	}
	// 同一输入按字节截断会切出非法 UTF-8，说明此处必须按字符处理
	if utf8.ValidString(raw[:200]) {
		t.Fatal("用例失效：字节截断未切到多字节字符中间")
	}
}

func TestTruncateRunesEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"n 为 0 返回空", "abc", 0, ""},
		{"n 为负返回空", "abc", -1, ""},
		{"短于上限原样", "abc", 10, "abc"},
		{"恰好等长原样", "abc", 3, "abc"},
		{"ASCII 截断", "abcdef", 3, "abc"},
		{"中文按字符截断", "中文标题", 2, "中文"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := truncateRunes(c.in, c.n); got != c.want {
				t.Fatalf("truncateRunes(%q, %d) = %q, 期望 %q", c.in, c.n, got, c.want)
			}
		})
	}
}

func TestTruncateAppendsEllipsis(t *testing.T) {
	if got := truncate("abcdef", 10); got != "abcdef" {
		t.Fatalf("未超长不应追加省略号: %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc..." {
		t.Fatalf("超长应截断并追加省略号: %q", got)
	}
	if got := truncate("中文标题很长", 2); got != "中文..." {
		t.Fatalf("中文应按字符截断: %q", got)
	}
}

// 日志只记录表单字段名：hidden input 的值可能携带会话票据/令牌，禁止落日志
func TestSortedKeysOnlyNames(t *testing.T) {
	values := url.Values{
		"E":            {"secret-ticket"},
		"A":            {"1"},
		"__VIEWSTATE":  {"state-blob"},
		"__EVENTTARGET": {},
	}
	want := []string{"A", "E", "__EVENTTARGET", "__VIEWSTATE"}
	if got := sortedKeys(values); !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedKeys = %v, 期望 %v", got, want)
	}
	if joined := strings.Join(sortedKeys(values), " "); strings.Contains(joined, "secret-ticket") {
		t.Fatal("字段名列表不得包含字段值")
	}
}
