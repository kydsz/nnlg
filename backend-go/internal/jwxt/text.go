package jwxt

import (
	"net/url"
	"sort"
	"unicode/utf8"
)

// truncateRunes 按字符（rune）截断，保证结果仍是合法 UTF-8。
// 按字节切片会把多字节字符切一半，写库/日志都会出现乱码。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// truncate 日志/错误信息用：超长时截断并追加省略号
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return truncateRunes(s, n) + "..."
}

// sortedKeys 返回表单字段名（排序）。
// 日志只记录字段名：hidden input 里可能带会话票据/令牌，值不能落日志。
func sortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
