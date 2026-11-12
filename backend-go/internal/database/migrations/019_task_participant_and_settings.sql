-- ============================================================
-- 019：评教任务参与人 + 系统配置
-- ------------------------------------------------------------
-- 背景：同一节课原先只能由一个人加入评教任务（任务按教师+课程+上课时间唯一）。
--       领导团听课时 3-4 人需要共同加入同一任务、各自提交评教，因此新增参与人关系，
--       并支持全局可配置的人数上限（默认 4，含创建人）。
-- 方案：
--   1. evaluation_task_participant：任务参与人，(task_id, user_id) 唯一；
--   2. system_setting：键值配置表，初始化 task_max_participants = 4；
--   3. 历史参与人按 evaluation_record 的去重评教人回填（含已作废记录），
--      不把未评教的历史创建人回填为参与人，避免历史任务永远无法「已评」。
-- 幂等：CREATE TABLE IF NOT EXISTS + INSERT IGNORE；不重算历史任务状态
--       （新口径从下一次加入/提交/作废/恢复开始生效）。
-- ============================================================

CREATE TABLE IF NOT EXISTS `evaluation_task_participant` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `task_id` INT NOT NULL COMMENT '评教任务ID',
  `user_id` INT NOT NULL COMMENT '参与人ID',
  `joined_at` DATETIME NULL COMMENT '加入时间',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_participant_user` (`task_id`, `user_id`),
  KEY `idx_task_participant_user` (`user_id`),
  CONSTRAINT `fk_task_participant_task` FOREIGN KEY (`task_id`) REFERENCES `evaluation_task` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评教任务参与人表';

CREATE TABLE IF NOT EXISTS `system_setting` (
  `setting_key` VARCHAR(64) NOT NULL COMMENT '配置键',
  `setting_value` VARCHAR(255) NOT NULL COMMENT '配置值',
  `description` VARCHAR(255) NULL COMMENT '说明',
  `update_by` INT NULL COMMENT '最后修改人',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`setting_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='系统配置表';

INSERT IGNORE INTO `system_setting` (`setting_key`, `setting_value`, `description`)
VALUES ('task_max_participants', '4', '单个评教任务最多参与人数（含创建人）');

INSERT IGNORE INTO `evaluation_task_participant` (`task_id`, `user_id`, `joined_at`, `create_time`, `update_time`)
SELECT r.task_id, r.evaluator_id,
       MIN(COALESCE(r.submit_time, r.create_time, NOW())),
       MIN(COALESCE(r.create_time, NOW())),
       NOW()
FROM `evaluation_record` r
WHERE r.evaluator_id IS NOT NULL
GROUP BY r.task_id, r.evaluator_id;
