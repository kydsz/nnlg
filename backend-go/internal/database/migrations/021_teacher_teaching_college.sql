-- ============================================================
-- 021：教师授课学院关系（多对多）
-- ------------------------------------------------------------
-- 背景：外聘教师的人事学院（user.college_id）与授课/开课学院不一致。
--       原系统只有一个学院口径，导致教师选择、任务创建、评教展示与统计
--       都按人事学院收敛，外聘教师所在授课学院无法找到他。
-- 方案：新增独立关系表，与 user_college（督导负责学院）解耦；
--       user.college_id 继续表示人事学院，本表表示该教师为哪些学院授课。
-- 幂等：CREATE TABLE IF NOT EXISTS；重复启动安全。
-- ============================================================

CREATE TABLE IF NOT EXISTS `teacher_teaching_college` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` INT NOT NULL COMMENT '教师用户ID',
  `college_id` INT NOT NULL COMMENT '授课学院ID',
  `source` VARCHAR(16) NOT NULL DEFAULT 'manual' COMMENT '来源：manual-手工维护 / jwxt-教务识别',
  `join_time` DATETIME NULL COMMENT '关联时间',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_teacher_teaching_college` (`user_id`, `college_id`),
  KEY `idx_ttc_user` (`user_id`),
  KEY `idx_ttc_college` (`college_id`),
  CONSTRAINT `fk_ttc_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_ttc_college` FOREIGN KEY (`college_id`) REFERENCES `college` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='教师授课学院关系表';
