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

func TestSignAccessParseRoundtrip(t *testing.T) {
	token, err := SignAccess(testSecret, 12345, 30, 7)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	info, err := Parse(testSecret, token)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.UserID != 12345 {
		t.Fatalf("期望 uid=12345, 实际 %d", info.UserID)
	}
	if info.TokenType != TokenTypeAccess {
		t.Fatalf("期望 token_type=access, 实际 %q", info.TokenType)
	}
	if info.Epoch != 7 {
		t.Fatalf("期望 ep=7, 实际 %d", info.Epoch)
	}
}

func TestSignRefreshRoundtrip(t *testing.T) {
	token, err := SignRefresh(testSecret, 42, 7, "jti-abc", 3)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	info, err := Parse(testSecret, token)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.UserID != 42 || info.TokenType != TokenTypeRefresh || info.JTI != "jti-abc" || info.Epoch != 3 {
		t.Fatalf("载荷不符: %+v", info)
	}
}

func TestParseAccessRejectsRefresh(t *testing.T) {
	refresh, _ := SignRefresh(testSecret, 1, 7, "j", 0)
	if _, err := ParseAccess(testSecret, refresh); err == nil {
		t.Fatal("refresh token 不应能作为 access 使用")
	}
	access, _ := SignAccess(testSecret, 1, 30, 0)
	if _, err := ParseRefresh(testSecret, access); err == nil {
		t.Fatal("access token 不应能作为 refresh 使用")
	}
}

func TestParseAccessAllowsMissingTokenType(t *testing.T) {
	// 旧签发的无 token_type token（如历史遗留）仍可解析为 access，避免升级断链
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "5",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	signed, _ := token.SignedString([]byte(testSecret))
	if _, err := ParseAccess(testSecret, signed); err != nil {
		t.Fatalf("无 token_type 的旧 token 应可作 access 使用: %v", err)
	}
}

func TestSignUsesHS256WithStringSub(t *testing.T) {
	token, err := SignAccess(testSecret, 7, 30, 0)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
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
	token, _ := SignAccess("another-secret-0000", 1, 30, 0)
	if _, err := Parse(testSecret, token); err == nil {
		t.Fatal("错误密钥应解析失败")
	}
}

func TestParseExpired(t *testing.T) {
	token, _ := SignAccess(testSecret, 1, -1, 0) // 已过期
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
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"1","exp":99999999999}`))
	noneToken := header + "." + payload + "."
	if _, err := Parse(testSecret, noneToken); err == nil {
		t.Fatal("none 算法 token 应被拒绝")
	}
}

func TestNewJTIUnique(t *testing.T) {
	a, b := NewJTI(), NewJTI()
	if a == "" || b == "" || a == b {
		t.Fatalf("jti 应非空且唯一: a=%q b=%q", a, b)
	}
}
