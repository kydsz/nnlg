-- ============================================================
-- 012：评教记录(task_id, evaluator_id) 并发防重复（条件唯一索引）
-- ------------------------------------------------------------
-- 背景：重复提交判定为"先查后插"非原子，配合死信重放/崩溃窗口，
--       同一 (task_id, evaluator_id) 可落两条记录并双加计数。
--       对"未删除"记录加唯一约束：软删除（is_deleted=1）行不参与唯一，
--       允许删除后重新提交（对齐 ValidateSubmit 的 is_deleted=0 判定）。
-- 实现：MySQL 不支持函数索引，用生成列 active_key 承载唯一约束——
--       is_deleted=0 时 active_key = "taskid_evaluatorid"（非空，参与唯一）；
--       is_deleted=1 时 active_key = NULL（MySQL 唯一索引允许多个 NULL）。
-- 注意：必须用 VIRTUAL 生成列。task_id 挂在 ON DELETE CASCADE 外键上
--       （evaluation_record_ibfk_1），MySQL 限制 STORED 生成列的基列不能带
--       CASCADE/SET NULL 外键动作，否则报 Error 1215 Cannot add foreign key
--       constraint（FOREIGN_KEY_CHECKS=0 也绕不过）。VIRTUAL 可加唯一索引，
--       读写语义一致，仅不落盘。
-- 幂等：重复执行报 Duplicate column / Duplicate key 由 Migrate 跳过。
-- ============================================================

ALTER TABLE `evaluation_record`
  ADD COLUMN `active_key` VARCHAR(64) GENERATED ALWAYS AS (
    IF(`is_deleted` = 0, CONCAT(CAST(`task_id` AS CHAR), '_', COALESCE(CAST(`evaluator_id` AS CHAR), '0')), NULL)
  ) VIRTUAL,
  ADD UNIQUE KEY `uk_task_evaluator_active` (`active_key`);