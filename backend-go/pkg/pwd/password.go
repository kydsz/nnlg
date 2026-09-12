package pwd

import (
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// Hash 密码加密（bcrypt，与旧后端 passlib 互相兼容）
func Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// Verify 校验密码
func Verify(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// MinPasswordLength 口令最小长度（按字符计，避免中文口令被字节长度误判）
const MinPasswordLength = 8

// weakPasswords 常见弱口令：项目历史默认值与常见字典前几位
var weakPasswords = map[string]bool{
	"12345678": true, "123456789": true, "1234567890": true,
	"11111111": true, "00000000": true, "88888888": true,
	"password": true, "passw0rd": true, "password1": true,
	"abc12345": true, "admin123": true, "administrator": true,
	"qwertyui": true, "qwerty123": true, "iloveyou": true,
	"teach123": true, "teacher123": true, "nnlg1234": true,
	"a1234567": true, "1qaz2wsx": true, "woaini123": true,
}

// Validate 校验口令是否满足最小强度（与前端提示「至少 8 位，含字母和数字」一致，
// 由服务端强制执行，避免绕过前端直接调接口设置弱口令）：
//  1. 长度不少于 MinPasswordLength 个字符；
//  2. 至少包含字母与数字两类字符；
//  3. 不得等于工号或用户名（忽略大小写）——默认口令取工号是历史问题，必须禁止；
//  4. 不得为常见弱口令，也不得是单一字符重复（如 11111111、aaaaaaaa）。
//
// userNo/username 允许为空（例如仅校验长度与字符类型）。
func Validate(password, userNo, username string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return errors.New("密码长度不能少于 8 位")
	}
	if !hasLetterAndDigit(password) {
		return errors.New("密码需同时包含字母和数字")
	}
	lower := strings.ToLower(password)
	if userNo != "" && lower == strings.ToLower(userNo) {
		return errors.New("密码不能与工号相同")
	}
	if username != "" && lower == strings.ToLower(username) {
		return errors.New("密码不能与姓名相同")
	}
	if weakPasswords[lower] {
		return errors.New("密码过于简单，请更换更复杂的密码")
	}
	if isRepeatedRune(password) {
		return errors.New("密码不能是单一字符重复")
	}
	return nil
}

// hasLetterAndDigit 至少包含一个 ASCII 字母与一个数字（与前端 /[a-zA-Z]/ + /[0-9]/ 一致）
func hasLetterAndDigit(s string) bool {
	var hasLetter, hasDigit bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	return hasLetter && hasDigit
}

// isRepeatedRune 判断是否为同一字符重复（如 11111111、aaaaaaaa、........）
func isRepeatedRune(s string) bool {
	first := rune(0)
	for i, r := range s {
		if i == 0 {
			first = r
			continue
		}
		if r != first {
			return false
		}
	}
	return true
}
