package pdfgen

import (
	"reflect"
	"testing"
)

func TestDerefValue(t *testing.T) {
	t.Run("非指针返回false", func(t *testing.T) {
		if v, ok := DerefValue("abc"); ok || v != nil {
			t.Fatalf("非指针应返回 (nil, false), 得到 (%v, %v)", v, ok)
		}
	})
	t.Run("nil接口返回false", func(t *testing.T) {
		if v, ok := DerefValue(nil); ok || v != nil {
			t.Fatalf("nil 应返回 (nil, false), 得到 (%v, %v)", v, ok)
		}
	})
	t.Run("一级指针解引用", func(t *testing.T) {
		s := "教室"
		v, ok := DerefValue(&s)
		if !ok || v != "教室" {
			t.Fatalf("一级指针解引用错误: (%v, %v)", v, ok)
		}
	})
	t.Run("多级指针解引用", func(t *testing.T) {
		f := 3.14
		p := &f
		pp := &p
		v, ok := DerefValue(&pp)
		if !ok || v != f || !reflect.DeepEqual(v, f) {
			t.Fatalf("多级指针解引用错误: (%v, %v)", v, ok)
		}
	})
	t.Run("nil指针返回true", func(t *testing.T) {
		var s *string
		v, ok := DerefValue(s)
		if !ok || v != nil {
			t.Fatalf("nil 指针应返回 (nil, true), 得到 (%v, %v)", v, ok)
		}
	})
}

func TestCellText(t *testing.T) {
	f := 88.5
	i := 42
	nilStr := (*string)(nil)

	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nil", nil, ""},
		{"整数浮点去尾", 88.0, "88"},
		{"小数浮点", f, "88.5"},
		{"bool真", true, "True"},
		{"bool假", false, "False"},
		{"字符串原样", "abc", "abc"},
		{"int走默认格式化", i, "42"},
		{"字符串指针", func() interface{} { s := "教室"; return &s }(), "教室"},
		{"nil指针空串", nilStr, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cellText(c.in); got != c.want {
				t.Fatalf("cellText(%v) = %q, 期望 %q", c.in, got, c.want)
			}
		})
	}
}
