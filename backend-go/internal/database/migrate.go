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
		for _, stmt := range splitSQLStatements(string(data)) {
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

// splitSQLStatements 按语句分隔符拆分 SQL，带引号感知：分号出现在单引号字符串、
// 注释（-- 至行尾）或反勾号标识符内时不作为分隔符；行首整行注释被剥离。
func splitSQLStatements(src string) []string {
	var stmts []string
	var cur strings.Builder
	inSingle, inBacktick := false, false
	runes := []rune(src)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		// 行注释（-- 开头）不在字符串/反勾号内、且当前语句仅含空白时跳过至行尾。
		// 注：MySQL 将双引号 " 也作字符串定界符，此处仅跟踪 ' 与 `；现有迁移全部用 ' 与 `，
		//     若未来引入双引号内含分号的语句需扩展状态机。
		if !inSingle && !inBacktick && ch == '-' && next == '-' && strings.TrimSpace(cur.String()) == "" {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}
		if !inBacktick && ch == '\'' && !isEscaped(runes, i) {
			inSingle = !inSingle
			cur.WriteRune(ch)
			continue
		}
		if !inSingle && ch == '`' {
			inBacktick = !inBacktick
			cur.WriteRune(ch)
			continue
		}
		if !inSingle && !inBacktick && ch == ';' {
			stmt := strings.TrimSpace(cur.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			cur.Reset()
			continue
		}
		cur.WriteRune(ch)
	}
	if tail := strings.TrimSpace(cur.String()); tail != "" {
		stmts = append(stmts, tail)
	}
	return stmts
}

// isEscaped 判断 rune[i] 是否被奇数个反斜杠转义（SQL 字符串内）。
func isEscaped(runes []rune, i int) bool {
	slashes := 0
	for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
		slashes++
	}
	return slashes%2 == 1
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
