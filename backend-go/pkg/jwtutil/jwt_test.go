package jwtutil

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "unit-test-secret"

func TestSignParseRoundtrip(t *testing.T) {
	token, err := Sign(testSecret, 12345, 30)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	uid, err := Parse(testSecret, token)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if uid != 12345 {
		t.Fatalf("期望 uid=12345, 实际 %d", uid)
	}
}

func TestSignUsesHS256WithStringSub(t *testing.T) {
	token, err := Sign(testSecret, 7, 30)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	// header 段必须是 HS256；sub 必须是字符串形式（与旧后端互通的前提）
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token 应为 3 段, 实际 %d 段", len(parts))
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("header 解码失败: %v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("header 解析失败: %v", err)
	}
	if header["alg"] != "HS256" {
		t.Fatalf("期望 alg=HS256, 实际 %v", header["alg"])
	}
	claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(claimsJSON, &claims)
	if _, ok := claims["sub"].(string); !ok {
		t.Fatalf("sub 应为字符串, 实际 %T", claims["sub"])
	}
}

func TestParseWrongSecret(t *testing.T) {
	token, _ := Sign("another-secret-0000", 1, 30)
	if _, err := Parse(testSecret, token); err == nil {
		t.Fatal("错误密钥应解析失败")
	}
}

func TestParseExpired(t *testing.T) {
	token, _ := Sign(testSecret, 1, -1) // 已过期
	if _, err := Parse(testSecret, token); err == nil {
		t.Fatal("过期 token 应解析失败")
	}
}

func TestParseMalformed(t *testing.T) {
	for _, tok := range []string{"", "not.a.token", "a.b"} {
		if _, err := Parse(testSecret, tok); err == nil {
			t.Fatalf("畸形 token %q 应解析失败", tok)
		}
	}
}

func TestParseEmptySub(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	signed, _ := token.SignedString([]byte(testSecret))
	if _, err := Parse(testSecret, signed); err == nil {
		t.Fatal("缺少 sub 应解析失败")
	}
}

func TestParseNonNumericSub(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "abc",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	signed, _ := token.SignedString([]byte(testSecret))
	if _, err := Parse(testSecret, signed); err == nil {
		t.Fatal("sub 非数字应解析失败")
	}
}

func TestParseRejectsNoneAlgorithm(t *testing.T) {
	// 手工构造 alg=none 且无签名的 token，验证密钥函数对非 HMAC 算法的拒绝
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"1","exp":99999999999}`))
	noneToken := header + "." + payload + "."
	if _, err := Parse(testSecret, noneToken); err == nil {
		t.Fatal("none 算法 token 应被拒绝")
	}
}
