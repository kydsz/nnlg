-- ============================================================
-- 013：任务去重并发兜底（evaluation_task 条件唯一索引）
-- ------------------------------------------------------------
-- 背景：创建任务的去重是「先查后插」，非原子。两个并发请求（双击、批量与单条并发、
--       多人同时加入待评）可同时通过预检查，插入两条同 (teacher_id, course_name, class_time)
--       的任务，导致同一门课重复评教、统计重复计数。
-- 方案：应用侧预检查保留（用于给出「任务ID=xx」的友好提示与批量跳过），
--       本唯一索引作为并发兜底：败方插入命中唯一冲突（MySQL 1062），
--       应用将其转换为与预检查一致的去重语义（单条接口报重复，批量接口计入 skipped）。
-- 实现：MySQL 不支持条件索引，用 STORED 生成列承载——
--       is_deleted = 0 且 status <> 3（未取消）时 dedupe_key 为 sha256(teacher|course|time)，
--       否则为 NULL（唯一索引允许多个 NULL）。因此取消/软删除后重新加入同一门课不受限，
--       口径与 service.Task 的预检查（is_deleted = 0 AND status <> 取消）完全一致。
-- 幂等：重复执行报 Duplicate column / Duplicate key 由 Migrate 跳过。
-- 注意：若历史库已存在重复的「未删除未取消」任务（正是本缺陷的历史产物），
--       加索引时唯一性校验会失败（Duplicate entry）导致启动报错，需先人工清理重复行。
-- ============================================================

ALTER TABLE `evaluation_task`
  ADD COLUMN `dedupe_key` CHAR(64) GENERATED ALWAYS AS (
    IF(`is_deleted` = 0 AND `status` <> 3,
       SHA2(CONCAT(`teacher_id`, '|', `course_name`, '|', IFNULL(CAST(`class_time` AS CHAR), '')), 256),
       NULL)
  ) STORED,
  ADD UNIQUE KEY `uk_task_dedupe_active` (`dedupe_key`);
