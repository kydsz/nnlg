package pwd

import "golang.org/x/crypto/bcrypt"

// Hash 密码加密（bcrypt，与旧后端 passlib 互相兼容）
func Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// Verify 校验密码
func Verify(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
