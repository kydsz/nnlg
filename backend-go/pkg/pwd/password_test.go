package pwd

import (
	"strings"
	"testing"
)

// TestValidate 口令最小强度：长度、字符类型、不得等于工号/姓名、弱口令表
func TestValidate(t *testing.T) {
	cases := []struct {
		name     string
		password string
		userNo   string
		username string
		wantErr  bool
	}{
		{"合规口令", "nnlg2026pass", "T001", "张三", false},
		{"合规口令含符号", "pass-2026!", "T001", "张三", false},
		{"过短", "ab12", "T001", "张三", true},
		{"纯字母", "teacheronly", "T001", "张三", true},
		{"纯数字", "20260812", "T001", "张三", true},
		{"等于工号", "T001", "T001", "张三", true},
		{"等于工号忽略大小写", "t001", "T001", "张三", true},
		{"等于姓名", "张三", "T001", "张三", true},
		{"弱口令表", "12345678", "T001", "张三", true},
		{"历史默认值", "teach123", "T001", "张三", true},
		{"单字符重复", "aaaaaaaa", "T001", "张三", true},
		{"无工号上下文仅校验长度类型", "nnlg2026", "", "", false},
		{"无工号上下文弱口令仍拦截", "password", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.password, c.userNo, c.username)
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate(%q) err=%v, 期望出错=%v", c.password, err, c.wantErr)
			}
		})
	}
}

// 中文口令按字符计长度，避免「字节数足够但实际只有 2 个字」被误判为强口令
func TestValidateCountsRunes(t *testing.T) {
	if err := Validate("中文口令八位啊", "", ""); err == nil {
		t.Fatal("7 个中文字符（21 字节）应因长度不足被拒")
	}
}

func TestHashVerifyRoundtrip(t *testing.T) {
	hash, err := Hash("teach123")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if !Verify("teach123", hash) {
		t.Fatal("正确密码应校验通过")
	}
	if Verify("wrong", hash) {
		t.Fatal("错误密码不应校验通过")
	}
}

func TestHashIsBcryptFormat(t *testing.T) {
	hash, err := Hash("teach123")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	// bcrypt 标准格式 $2a$，与旧 Python 端 passlib 互通的前提
	if !strings.HasPrefix(hash, "$2a$") {
		t.Fatalf("期望 $2a$ 前缀, 实际 %s", hash)
	}
}

func TestHashSaltsEachTime(t *testing.T) {
	h1, _ := Hash("same-password")
	h2, _ := Hash("same-password")
	if h1 == h2 {
		t.Fatal("同一密码两次加密应产生不同哈希（含盐）")
	}
	if !Verify("same-password", h1) || !Verify("same-password", h2) {
		t.Fatal("两个哈希都应校验通过")
	}
}

func TestVerifyWrongLengthPassword(t *testing.T) {
	hash, _ := Hash("123456")
	if Verify("1234567", hash) {
		t.Fatal("长度不同的密码不应校验通过")
	}
	if Verify("", hash) {
		t.Fatal("空密码不应校验通过")
	}
}

func TestVerifyInvalidHash(t *testing.T) {
	if Verify("x", "not-a-hash") {
		t.Fatal("非法哈希应返回 false 而非 panic")
	}
	if Verify("", "") {
		t.Fatal("空密码对空哈希应返回 false")
	}
}
