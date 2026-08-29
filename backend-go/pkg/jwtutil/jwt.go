package jwtutil

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Sign 签发 token，sub 为用户 ID（与旧后端保持一致）
func Sign(secret string, userID int64, expireMinutes int) (string, error) {
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID, 10),
		"exp": time.Now().Add(time.Duration(expireMinutes) * time.Minute).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Parse 校验并取出用户 ID
func Parse(secret, tokenStr string) (int64, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非法的签名算法")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return 0, errors.New("无效的 token")
	}

	sub, err := token.Claims.GetSubject()
	if err != nil || sub == "" {
		return 0, errors.New("无效的 token 载荷")
	}
	uid, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, errors.New("无效的用户 ID")
	}
	return uid, nil
}
