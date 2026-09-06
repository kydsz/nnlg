package database

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate 服务启动时自动执行幂等迁移。
// 按文件名顺序执行 internal/database/migrations 下的每个 SQL 文件（按分号拆分逐条执行）；
// 列已存在等"重复执行"类错误（MySQL 1060 Duplicate column name）自动忽略，可安全重复启动。
func Migrate(db *gorm.DB) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		data, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		for _, stmt := range strings.Split(string(data), ";") {
			stmt = strings.TrimSpace(stmt)
			// 去掉纯注释片段（-- 打头的行）
			var lines []string
			for _, ln := range strings.Split(stmt, "\n") {
				if t := strings.TrimSpace(ln); strings.HasPrefix(t, "--") || t == "" {
					continue
				}
				lines = append(lines, ln)
			}
			sql := strings.Join(lines, "\n")
			if strings.TrimSpace(sql) == "" {
				continue
			}
			if err := db.Exec(sql).Error; err != nil {
				if isDuplicateError(err) {
					continue // 幂等：列/键已存在则跳过
				}
				return fmt.Errorf("migration %s failed: %w", name, err)
			}
		}
	}
	return nil
}

// isDuplicateError 判断是否为"列/键已存在"类幂等错误（MySQL 1060 Duplicate column / 1061 Duplicate key）
func isDuplicateError(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) && (me.Number == 1060 || me.Number == 1061) {
		return true
	}
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate column name") || strings.Contains(msg, "Duplicate key name")
}