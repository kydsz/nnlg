package database

import (
	"reflect"
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

func TestSplitSQLStatementsEscapedQuote(t *testing.T) {
	// 反斜杠转义的引号内分号不切分
	src := `INSERT INTO t (v) VALUES ('a\';b'); SELECT 2;`
	got := splitSQLStatements(src)
	want := []string{"INSERT INTO t (v) VALUES ('a\\';b')", "SELECT 2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("转义引号拆分不符:\n got: %#v\nwant: %#v", got, want)
	}
}
