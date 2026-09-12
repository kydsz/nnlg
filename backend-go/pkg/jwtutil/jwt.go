package jwtutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// token_type claims 取值
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// TokenInfo 解析出的 token 载荷信息
type TokenInfo struct {
	UserID    int64  `json:"user_id"`
	TokenType string `json:"token_type"`
	JTI       string `json:"jti"`
	Epoch     int64  `json:"ep"`
}

// SignAccess 签发短期 access_token，仅携带身份（sub）与会话 epoch，不携带权限。
func SignAccess(secret string, userID int64, expireMinutes int, epoch int64) (string, error) {
	return sign(secret, map[string]interface{}{
		"sub":        strconv.FormatInt(userID, 10),
		"token_type": TokenTypeAccess,
		"ep":         epoch,
		"exp":        time.Now().Add(time.Duration(expireMinutes) * time.Minute).Unix(),
	})
}

// SignRefresh 签发长期 refresh_token（HttpOnly cookie，Redis 可撤销）。
// jti 用于标识单个会话令牌；epoch 与会话撤销计数耦合，登出/禁用/改密后自增即可令其失效。
func SignRefresh(secret string, userID int64, days int, jti string, epoch int64) (string, error) {
	return sign(secret, map[string]interface{}{
		"sub":        strconv.FormatInt(userID, 10),
		"token_type": TokenTypeRefresh,
		"jti":        jti,
		"ep":         epoch,
		"exp":        time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix(),
	})
}

func sign(secret string, claims map[string]interface{}) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(claims)).SignedString([]byte(secret))
}

// NewJTI 生成随机 jti（refresh token 唯一标识）。随机源失败返回错误，
// 不再降级为时间戳（时间戳可预测，削弱 jti 随机性）。
func NewJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Parse 校验签名与过期时间，返回 token 载荷信息。不限定 token_type，由调用方进一步校验。
func Parse(secret, tokenStr string) (*TokenInfo, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非法的签名算法")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("无效的 token")
	}

	sub, err := token.Claims.GetSubject()
	if err != nil || sub == "" {
		return nil, errors.New("无效的 token 载荷")
	}
	uid, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return nil, errors.New("无效的用户 ID")
	}
	info := &TokenInfo{UserID: uid}
	if m, ok := token.Claims.(jwt.MapClaims); ok {
		if v, ok := m["token_type"].(string); ok {
			info.TokenType = v
		}
		if v, ok := m["jti"].(string); ok {
			info.JTI = v
		}
		// ep 可能是数字（float64）或字符串，统一兼容
		switch v := m["ep"].(type) {
		case float64:
			info.Epoch = int64(v)
		case int64:
			info.Epoch = v
		case string:
			info.Epoch, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	return info, nil
}

// ParseAccess 解析并强制要求 access token：
// 缺失 token_type 或非 access 一律拒绝（缺失即视为非法令牌，防模糊携带）。
func ParseAccess(secret, tokenStr string) (*TokenInfo, error) {
	info, err := Parse(secret, tokenStr)
	if err != nil {
		return nil, err
	}
	if info.TokenType != TokenTypeAccess {
		return nil, errors.New("token 类型不允许")
	}
	return info, nil
}

// ParseRefresh 解析并强制要求 refresh token。
func ParseRefresh(secret, tokenStr string) (*TokenInfo, error) {
	info, err := Parse(secret, tokenStr)
	if err != nil {
		return nil, err
	}
	if info.TokenType != TokenTypeRefresh {
		return nil, errors.New("token 类型不允许")
	}
	return info, nil
}
