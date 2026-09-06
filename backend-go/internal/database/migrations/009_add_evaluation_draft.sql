-- ============================================================
-- 009：新增评教草稿表 evaluation_draft（填写听课表「暂存」）
-- ------------------------------------------------------------
-- 背景：移动端填写听课表支持「暂存」，随时保存进度后恢复，
--       不污染正式评教记录 evaluation_record（记录/统计/导出/唯一性判断均不受影响）。
-- 结构：同一任务同一评教人仅一条草稿（uk_task_evaluator 唯一索引）。
--       存 dimension_values(JSON) 与 is_anonymous，提交成功后在事务内删除。
-- 幂等：CREATE TABLE IF NOT EXISTS 可重复执行。
-- ============================================================

CREATE TABLE IF NOT EXISTS evaluation_draft (
  id BIGINT NOT NULL AUTO_INCREMENT,
  task_id BIGINT NULL COMMENT '评教任务 id',
  evaluator_id BIGINT NULL COMMENT '评教人（打分教师/督导）id',
  evaluator_name VARCHAR(64) NULL COMMENT '评教人姓名快照',
  dimension_values JSON NULL COMMENT '维度值（JSON，同 evaluation_record）',
  is_anonymous TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否匿名评教',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_task_evaluator (task_id, evaluator_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;