package config

import (
	"errors"
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config 全部配置项，从环境变量 / .env 读取
type Config struct {
	AppName string `env:"APP_NAME" envDefault:"Teaching Evaluation System V2"`
	Version string `env:"APP_VERSION" envDefault:"2.0.0"`
	Debug   bool   `env:"DEBUG" envDefault:"false"`
	Port    string `env:"PORT" envDefault:"8000"`

	DBHost     string `env:"DB_HOST" envDefault:"localhost"`
	DBPort     int    `env:"DB_PORT" envDefault:"3306"`
	DBUser     string `env:"DB_USER" envDefault:"root"`
	DBPassword string `env:"DB_PASSWORD"`
	DBName     string `env:"DB_NAME" envDefault:"teaching_eval_v2"`

	SecretKey          string `env:"SECRET_KEY"`
	TokenExpireMinutes int    `env:"ACCESS_TOKEN_EXPIRE_MINUTES" envDefault:"1440"`
	CookieSecure       bool   `env:"COOKIE_SECURE" envDefault:"false"`
	UploadDir          string `env:"UPLOAD_DIR" envDefault:"uploads"`

	JWXTBaseURLXS string `env:"JWXT_BASE_URL_XS" envDefault:"http://qzjw.bwgl.cn/gllgdxbwglxy_jsxsd"`
	JWXTBaseURLGL string `env:"JWXT_BASE_URL_GL" envDefault:"http://qzjw.bwgl.cn/gllgdxbwglxy"`
	JWXTUser      string `env:"JWXT_USERNAME"`
	JWXTPassword  string `env:"JWXT_PASSWORD"`

	DefaultUserPassword string `env:"DEFAULT_USER_PASSWORD" envDefault:"teach123"`

	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"*"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	if len(cfg.SecretKey) < 16 {
		return nil, errors.New("SECRET_KEY 未设置或长度不足 16 字符，请在 .env 中配置安全密钥")
	}
	return cfg, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}
