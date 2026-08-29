package config

import (
	"strings"
	"testing"
)

// 环境隔离：清空与配置相关的环境变量，避免宿主机全局变量（如 DEBUG）干扰
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"APP_NAME", "APP_VERSION", "DEBUG", "PORT",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"SECRET_KEY", "ACCESS_TOKEN_EXPIRE_MINUTES", "COOKIE_SECURE", "UPLOAD_DIR",
		"CORS_ORIGINS",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadMissingSecretKey(t *testing.T) {
	isolateEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("未设置 SECRET_KEY 应报错")
	}
}

func TestLoadShortSecretKey(t *testing.T) {
	isolateEnv(t)
	t.Setenv("SECRET_KEY", "short-secret") // 12 字符
	if _, err := Load(); err == nil {
		t.Fatal("SECRET_KEY 不足 16 字符应报错")
	}
}

func TestLoadDefaults(t *testing.T) {
	isolateEnv(t)
	t.Setenv("SECRET_KEY", "0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Port != "8000" {
		t.Fatalf("默认端口应为 8000, 实际 %s", cfg.Port)
	}
	if cfg.TokenExpireMinutes != 1440 {
		t.Fatalf("默认过期时间应为 1440, 实际 %d", cfg.TokenExpireMinutes)
	}
	if cfg.Debug {
		t.Fatal("默认 Debug 应为 false")
	}
	if cfg.DBPort != 3306 || cfg.DBName != "teaching_eval_v2" {
		t.Fatalf("默认 DB 配置不符: %+v", cfg)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	isolateEnv(t)
	t.Setenv("SECRET_KEY", "0123456789abcdef")
	t.Setenv("PORT", "9000")
	t.Setenv("DEBUG", "true")
	t.Setenv("DB_HOST", "10.0.0.5")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_PASSWORD", "p@ss")
	t.Setenv("CORS_ORIGINS", "https://a.com,https://b.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Port != "9000" || !cfg.Debug || cfg.DBHost != "10.0.0.5" || cfg.DBPort != 3307 {
		t.Fatalf("环境变量覆盖不生效: %+v", cfg)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[1] != "https://b.com" {
		t.Fatalf("CORS_ORIGINS 应按逗号拆分: %v", cfg.CORSOrigins)
	}
}

func TestDSN(t *testing.T) {
	cfg := &Config{
		DBUser: "tev2", DBPassword: "secret", DBHost: "mysql",
		DBPort: 3306, DBName: "teaching_eval_v2",
	}
	dsn := cfg.DSN()
	for _, part := range []string{
		"tev2:secret@tcp(mysql:3306)/teaching_eval_v2",
		"charset=utf8mb4", "parseTime=True",
	} {
		if !strings.Contains(dsn, part) {
			t.Fatalf("DSN 缺少 %q: %s", part, dsn)
		}
	}
}
