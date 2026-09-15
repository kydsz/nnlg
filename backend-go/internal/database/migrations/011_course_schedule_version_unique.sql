-- ============================================================
-- 011：课表版本号并发兜底（course_schedule_version 唯一索引）
-- ------------------------------------------------------------
-- 背景：版本号原以 MAX+1 生成且无锁，并发同步可产生重复版本号。
--       事务内行锁（nextGlobalVersion 的 FOR UPDATE）为第一道防线，
--       唯一索引为第二道兜底：并发败方插入时命中唯一约束报错回滚，
--       避免脏数据，任务被标记失败而非静默成功。
-- 自愈：历史库可能已存在重复 (semester, version)（正是本缺陷的历史产物），
--       直接建唯一索引会报 Error 1062 Duplicate entry 使启动失败。
--       先按 (semester, version) 去重、每组保留最新一行（MAX(id)）再建索引；
--       该表为纯版本历史（无外键引用其 id），重复行内容近乎相同，去重无损。
--       DELETE 幂等：再次执行删 0 行。唯一索引已存在报 1061 由 Migrate 跳过。
-- ============================================================

DELETE v FROM `course_schedule_version` v
JOIN `course_schedule_version` keep
  ON v.`semester` = keep.`semester`
 AND v.`version` = keep.`version`
 AND v.`id` < keep.`id`;

ALTER TABLE `course_schedule_version`
  ADD UNIQUE KEY `uk_sem_version` (`semester`, `version`);
