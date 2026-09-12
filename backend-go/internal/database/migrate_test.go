package database

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

// 迁移器拆分必须引号敏感：注释/单引号字符串/反勾号内的分号不得误切（AC10）。
func TestSplitSQLStatements(t *testing.T) {
	src := `
-- 注释里有个分号; 不应切分
ALTER TABLE t ADD COLUMN c INT;
INSERT INTO t (v) VALUES ('a;b');
UPDATE t SET note = 'it''s;ok' WHERE id = 1;
SELECT ` + "`weird;col`" + ` FROM t;
CREATE TABLE u (id INT);`
	got := splitSQLStatements(src)
	want := []string{
		"ALTER TABLE t ADD COLUMN c INT",
		"INSERT INTO t (v) VALUES ('a;b')",
		"UPDATE t SET note = 'it''s;ok' WHERE id = 1",
		"SELECT `weird;col` FROM t",
		"CREATE TABLE u (id INT)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("拆分结果不符:\n got: %#v\nwant: %#v", got, want)
	}
}

// 所有迁移文件都必须能被拆分器正确解析：引号内的分号（如 CONCAT(a,'|',b)）
// 一旦被误切，语句会在生产迁移时执行失败，此测试在提交阶段就把问题挡住。
func TestMigrationFilesParse(t *testing.T) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatalf("读取迁移目录失败: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := migrationFiles.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", e.Name(), err)
		}
		stmts := splitSQLStatements(string(data))
		if len(stmts) == 0 {
			t.Fatalf("%s 未拆分出任何语句", e.Name())
		}
		for _, s := range stmts {
			if strings.Count(s, "'")%2 != 0 {
				t.Fatalf("%s 片段引号不成对（疑似被误切）:\n%s", e.Name(), s)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("未找到任何迁移文件")
	}
}

func TestSplitSQLStatementsEscapedQuote(t *testing.T) {
	// 反斜杠转义的引号内分号不切分
	src := `INSERT INTO t (v) VALUES ('a\';b'); SELECT 2;`
	got := splitSQLStatements(src)
	want := []string{"INSERT INTO t (v) VALUES ('a\\';b')", "SELECT 2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("转义引号拆分不符:\n got: %#v\nwant: %#v", got, want)
	}
}
