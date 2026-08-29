package pwd

import (
	"strings"
	"testing"
)

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
