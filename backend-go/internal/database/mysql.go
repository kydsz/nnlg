package database

import (
	"time"

	"backend-go/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Init 连接 MySQL 并自动执行幂等迁移。
// 注意：业务表结构的主体仍由外部 SQL 迁移管理；Migrate 只负责增量、幂等的结构变更。
func Init(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.Debug {
		logLevel = logger.Info
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, err
	}

	// 自动执行幂等迁移（列已存在自动跳过，可重复启动）
	if err := Migrate(db); err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}
