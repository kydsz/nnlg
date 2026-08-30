-- ============================================================
-- 001：semester_config 增加 weeks（每学期周数）
-- ------------------------------------------------------------
-- 背景：学期结束日不再写死 20 周，改为「开学日期 + weeks × 7 天」。
-- 幂等：重复执行报"Duplicate column name"(MySQL 1060) 时由启动迁移器自动忽略。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

ALTER TABLE `semester_config` ADD COLUMN `weeks` INT NOT NULL DEFAULT 20 AFTER `start_date`;