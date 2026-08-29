package main

import (
	"log"
	"os"
	"strings"

	"backend-go/internal/config"
	"backend-go/internal/database"
	"backend-go/internal/router"

	"github.com/joho/godotenv"
)

func main() {
	// .env 可选，环境变量优先
	_ = godotenv.Load()
	normalizeDebug()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	r := router.Setup(cfg, db)

	log.Printf("%s v%s 启动于 :%s", cfg.AppName, cfg.Version, cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// normalizeDebug 清理非布尔值的 DEBUG 环境变量
// （如本机全局 DEBUG=release 会干扰解析，移除后让 .env 的 DEBUG 生效）
func normalizeDebug() {
	switch v := os.Getenv("DEBUG"); strings.ToLower(v) {
	case "", "true", "false", "1", "0":
	default:
		_ = os.Unsetenv("DEBUG")
	}
}
