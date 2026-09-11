-- ============================================================
-- 011：课表版本号并发兜底（course_schedule_version 唯一索引）
-- ------------------------------------------------------------
-- 背景：版本号原以 MAX+1 生成且无锁，并发同步可产生重复版本号。
--       事务内行锁（nextGlobalVersion 的 FOR UPDATE）为第一道防线，
--       唯一索引为第二道兜底：并发败方插入时命中唯一约束报错回滚，
--       避免脏数据，任务被标记失败而非静默成功。
-- 注意：如历史库已存在重复 (semester, version)（正是本缺陷的历史产物），
--       ALTER 会因 Duplicate entry 失败导致应用启动报错，需先人工清理重复行。
-- 幂等：重复执行报 Duplicate key 由 Migrate 跳过。
-- ============================================================

ALTER TABLE `course_schedule_version`
  ADD UNIQUE KEY `uk_sem_version` (`semester`, `version`);