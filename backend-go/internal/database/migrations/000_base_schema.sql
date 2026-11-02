-- ============================================================
-- 000：基础表结构（空库首次部署自动建表）
-- ------------------------------------------------------------
-- 背景：业务表结构此前由 V2（FastAPI + SQLAlchemy create_all）创建，本仓库
--       只有增量迁移（001+），空库部署时 001 的 ALTER 直接报"表不存在"导致
--       启动失败。本文件按 backend-go/internal/model 现有 GORM 模型补齐
--       基础表 DDL（含 001-017 增量迁移的最终形态：weeks / session_epoch /
--       schedule_snapshot / 各唯一索引 / 生成列），空库一次成型。
-- 幂等：全部 CREATE TABLE IF NOT EXISTS，可重复执行；对已有 V2/生产库
--       完全无副作用（表已存在则跳过，后续 001-017 迁移继续负责增量）。
-- 参照：V2 仓库（gitee.com/kydsz/v2）backend/app/models/* 与
--       migration_rbac_roles.sql；差异以 nnlg Go 模型为准（见 README 部署章节）。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

CREATE TABLE IF NOT EXISTS `campus` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `name` VARCHAR(64) NOT NULL COMMENT '校区名称',
  `sort_order` INT NOT NULL DEFAULT 0 COMMENT '排序',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  INDEX `idx_campus_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='校区表';

CREATE TABLE IF NOT EXISTS `college` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `code` VARCHAR(32) NOT NULL COMMENT '学院编码',
  `name` VARCHAR(64) NOT NULL COMMENT '学院名称',
  `campus_id` INT NULL COMMENT '所属校区ID',
  `sort_order` SMALLINT NOT NULL DEFAULT 0 COMMENT '排序',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_college_code` (`code`),
  INDEX `idx_college_campus` (`campus_id`),
  INDEX `idx_college_status` (`status`),
  CONSTRAINT `fk_college_campus` FOREIGN KEY (`campus_id`) REFERENCES `campus` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='学院表';

CREATE TABLE IF NOT EXISTS `research_room` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `code` VARCHAR(32) NULL COMMENT '教研室编码',
  `name` VARCHAR(64) NOT NULL COMMENT '教研室名称',
  `college_id` INT NOT NULL COMMENT '所属学院ID',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_research_room_code` (`code`),
  INDEX `idx_research_room_college` (`college_id`),
  INDEX `idx_research_room_status` (`status`),
  CONSTRAINT `fk_research_room_college` FOREIGN KEY (`college_id`) REFERENCES `college` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='教研室表';

CREATE TABLE IF NOT EXISTS `user` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_no` VARCHAR(32) NOT NULL COMMENT '工号/账号',
  `username` VARCHAR(64) NOT NULL COMMENT '用户名/姓名',
  `password` VARCHAR(255) NOT NULL COMMENT '密码（bcrypt加密）',
  `role` VARCHAR(20) NOT NULL COMMENT '主角色（向后兼容，多角色见 user_role）',
  `college_id` INT NULL COMMENT '所属学院ID',
  `research_room_id` INT NULL COMMENT '所属教研室ID',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `must_change_password` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '首次登录需修改密码',
  `last_login_time` DATETIME NULL COMMENT '最后登录时间',
  `session_epoch` BIGINT NOT NULL DEFAULT 0 COMMENT '会话撤销计数（DB 权威）',
  `refresh_jti` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '当前有效 refresh token 的 jti',
  `refresh_jti_prev` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '上一轮 refresh token 的 jti（重放检测）',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_no` (`user_no`),
  INDEX `idx_user_role` (`role`),
  INDEX `idx_user_college` (`college_id`),
  INDEX `idx_user_research_room` (`research_room_id`),
  INDEX `idx_user_status` (`status`),
  CONSTRAINT `fk_user_college` FOREIGN KEY (`college_id`) REFERENCES `college` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_user_research_room` FOREIGN KEY (`research_room_id`) REFERENCES `research_room` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

CREATE TABLE IF NOT EXISTS `user_role` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` INT NOT NULL COMMENT '用户ID',
  `role` VARCHAR(20) NOT NULL COMMENT '角色编码',
  `assign_time` DATETIME NULL COMMENT '分配时间',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_role` (`user_id`, `role`),
  INDEX `idx_user_role_user` (`user_id`),
  INDEX `idx_user_role_role` (`role`),
  CONSTRAINT `fk_user_role_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户角色关联表';

CREATE TABLE IF NOT EXISTS `user_college` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` INT NOT NULL COMMENT '用户ID',
  `college_id` INT NOT NULL COMMENT '学院ID',
  `join_time` DATETIME NULL COMMENT '加入时间',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_college` (`user_id`, `college_id`),
  INDEX `idx_user_college_user` (`user_id`),
  INDEX `idx_user_college_college` (`college_id`),
  CONSTRAINT `fk_user_college_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_college_college` FOREIGN KEY (`college_id`) REFERENCES `college` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户学院关联表（督导负责学院）';

CREATE TABLE IF NOT EXISTS `user_research_room` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` INT NOT NULL COMMENT '用户ID',
  `research_room_id` INT NOT NULL COMMENT '教研室ID',
  `join_time` DATETIME NULL COMMENT '加入时间',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_research_room` (`user_id`, `research_room_id`),
  INDEX `idx_user_research_room_user` (`user_id`),
  INDEX `idx_user_research_room_room` (`research_room_id`),
  CONSTRAINT `fk_user_research_room_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_research_room_room` FOREIGN KEY (`research_room_id`) REFERENCES `research_room` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户教研室关联表';

CREATE TABLE IF NOT EXISTS `role` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `name` VARCHAR(64) NOT NULL COMMENT '角色名称',
  `code` VARCHAR(32) NOT NULL COMMENT '角色编码',
  `description` VARCHAR(255) NULL COMMENT '角色描述',
  `level` INT NOT NULL DEFAULT 100 COMMENT '优先级（数字越小权限越高）',
  `permissions` JSON NULL COMMENT '权限列表',
  `data_scope` VARCHAR(20) NOT NULL DEFAULT 'self' COMMENT '数据范围: all/college/self',
  `is_system` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否系统内置角色',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_role_code` (`code`),
  INDEX `idx_role_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色表';

CREATE TABLE IF NOT EXISTS `dimension_group` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `code` VARCHAR(32) NOT NULL COMMENT '分组编码',
  `name` VARCHAR(64) NOT NULL COMMENT '分组名称',
  `sort_order` INT NOT NULL DEFAULT 0 COMMENT '排序',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_dimension_group_code` (`code`),
  INDEX `idx_dimension_group_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评教维度分组表';

CREATE TABLE IF NOT EXISTS `evaluation_dimension` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `group_id` INT NULL COMMENT '所属分组ID',
  `code` VARCHAR(32) NOT NULL COMMENT '维度编码',
  `name` VARCHAR(64) NOT NULL COMMENT '维度名称',
  `field_type` VARCHAR(32) NOT NULL COMMENT '字段类型',
  `field_config` JSON NULL COMMENT '字段配置',
  `description` TEXT NULL COMMENT '维度说明',
  `sort_order` INT NOT NULL DEFAULT 0 COMMENT '排序',
  `is_required` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否必填',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-启用 0-禁用',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_dimension_code` (`code`),
  INDEX `idx_dimension_group` (`group_id`),
  INDEX `idx_dimension_type` (`field_type`),
  INDEX `idx_dimension_status` (`status`),
  CONSTRAINT `fk_dimension_group` FOREIGN KEY (`group_id`) REFERENCES `dimension_group` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评教维度表';

CREATE TABLE IF NOT EXISTS `semester_config` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `semester` VARCHAR(32) NOT NULL COMMENT '学期代码，如2025-2026-2',
  `start_date` DATE NOT NULL COMMENT '学期起始日期',
  `weeks` INT NOT NULL DEFAULT 20 COMMENT '每学期周数（001 迁移形态）',
  `is_current` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否为当前学期',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_semester` (`semester`),
  INDEX `idx_semester_current` (`is_current`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='学期配置表';

CREATE TABLE IF NOT EXISTS `course_schedule` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `teacher_id` INT NULL COMMENT '教师ID（无外键，允许引用已删除教师）',
  `teacher_name` VARCHAR(64) NOT NULL COMMENT '教师姓名（冗余）',
  `semester` VARCHAR(32) NOT NULL COMMENT '学年学期，如2025-2026-2',
  `version` INT NOT NULL DEFAULT 1 COMMENT '版本号',
  `is_current` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否为当前有效版本',
  `crawl_time` DATETIME NULL COMMENT '爬取时间',
  `change_summary` TEXT NULL COMMENT '变更摘要',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  INDEX `idx_schedule_teacher_semester` (`teacher_id`, `semester`),
  INDEX `idx_schedule_semester_version` (`semester`, `version`),
  INDEX `idx_schedule_current` (`is_current`),
  CONSTRAINT `fk_course_schedule_teacher` FOREIGN KEY (`teacher_id`) REFERENCES `user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='课程表主表';

CREATE TABLE IF NOT EXISTS `course_schedule_detail` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `schedule_id` INT NOT NULL COMMENT '课程表主表ID',
  `course_name` VARCHAR(128) NOT NULL COMMENT '课程名称',
  `class_info` VARCHAR(256) NOT NULL DEFAULT '' COMMENT '班级信息',
  `week_pattern` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '周次模式，如1-12周',
  `week_day` TINYINT NULL COMMENT '星期几（1-7）',
  `section` VARCHAR(16) NOT NULL DEFAULT '' COMMENT '节次，如0102',
  `classroom` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '教室',
  `raw_data` TEXT NULL COMMENT '原始数据（用于调试）',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  INDEX `idx_detail_schedule` (`schedule_id`),
  INDEX `idx_detail_weekday_section` (`week_day`, `section`),
  CONSTRAINT `fk_detail_schedule` FOREIGN KEY (`schedule_id`) REFERENCES `course_schedule` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='课程明细表';

CREATE TABLE IF NOT EXISTS `course_schedule_version` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `semester` VARCHAR(32) NOT NULL COMMENT '学年学期',
  `version` INT NOT NULL COMMENT '版本号',
  `crawl_time` DATETIME NULL COMMENT '爬取时间',
  `teacher_count` INT NOT NULL DEFAULT 0 COMMENT '教师数量',
  `course_count` INT NOT NULL DEFAULT 0 COMMENT '课程数量',
  `change_summary` TEXT NULL COMMENT '变更摘要',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sem_version` (`semester`, `version`),
  INDEX `idx_version_semester` (`semester`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='课程表版本历史表';

CREATE TABLE IF NOT EXISTS `evaluation_task` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `teacher_id` INT NULL COMMENT '被评教师ID（无外键，允许引用已删除教师）',
  `teacher_name` VARCHAR(64) NOT NULL COMMENT '被评教师姓名（冗余）',
  `course_name` VARCHAR(128) NOT NULL COMMENT '课程名称',
  `class_time` DATETIME NULL COMMENT '上课时间',
  `classroom` VARCHAR(64) NULL COMMENT '教室',
  `schedule_snapshot` JSON NULL COMMENT '课表信息快照（加入待评时固化，007 迁移形态）',
  `status` SMALLINT NOT NULL DEFAULT 1 COMMENT '状态 1-待评 2-已评 3-取消',
  `start_time` DATETIME NULL COMMENT '评教开始时间',
  `end_time` DATETIME NULL COMMENT '评教结束时间',
  `evaluation_count` INT NOT NULL DEFAULT 0 COMMENT '评教次数',
  `has_supervisor_eval` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否有督导评教',
  `create_by` INT NULL COMMENT '创建人ID',
  `is_deleted` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否已删除（软删除）',
  `dedupe_key` CHAR(64) GENERATED ALWAYS AS (
    IF(`is_deleted` = 0 AND `status` <> 3,
       SHA2(CONCAT(`teacher_id`, '|', `course_name`, '|', IFNULL(CAST(`class_time` AS CHAR), '')), 256),
       NULL)
  ) STORED COMMENT '任务去重生成列（013 迁移形态）',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_dedupe_active` (`dedupe_key`),
  INDEX `idx_task_teacher` (`teacher_id`),
  INDEX `idx_task_status` (`status`),
  INDEX `idx_task_create_by` (`create_by`),
  INDEX `idx_task_is_deleted` (`is_deleted`),
  CONSTRAINT `fk_task_create_by` FOREIGN KEY (`create_by`) REFERENCES `user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评教任务表';

CREATE TABLE IF NOT EXISTS `evaluation_record` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `task_id` INT NOT NULL COMMENT '评教任务ID',
  `evaluator_id` INT NULL COMMENT '评教人ID（无外键，允许引用已删除评教人）',
  `evaluator_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '评教人姓名（冗余）',
  `evaluator_role` VARCHAR(20) NOT NULL COMMENT '评教人角色（冗余）',
  `dimension_values` JSON NOT NULL COMMENT '维度值（JSON，key 为维度 code）',
  `submit_time` DATETIME NULL COMMENT '提交时间',
  `is_anonymous` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否匿名评教',
  `is_deleted` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否作废（软删除，006/012 依赖）',
  `active_key` VARCHAR(64) GENERATED ALWAYS AS (
    IF(`is_deleted` = 0, CONCAT(CAST(`task_id` AS CHAR), '_', COALESCE(CAST(`evaluator_id` AS CHAR), '0')), NULL)
  ) VIRTUAL COMMENT '有效记录唯一键生成列（012 迁移形态，须 VIRTUAL：task_id 带 CASCADE 外键）',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_evaluator_active` (`active_key`),
  INDEX `idx_eval_record_task` (`task_id`),
  INDEX `idx_eval_record_evaluator` (`evaluator_id`),
  INDEX `idx_eval_record_submit_time` (`submit_time`),
  CONSTRAINT `fk_eval_record_task` FOREIGN KEY (`task_id`) REFERENCES `evaluation_task` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评教记录表';

CREATE TABLE IF NOT EXISTS `operation_log` (
  `id` INT NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` INT NULL COMMENT '操作用户ID',
  `user_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '操作用户名（冗余）',
  `operation_type` VARCHAR(32) NOT NULL COMMENT '操作类型',
  `module` VARCHAR(32) NOT NULL COMMENT '模块',
  `target_id` INT NULL COMMENT '操作目标ID',
  `target_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '操作目标类型',
  `content` JSON NULL COMMENT '操作详情',
  `ip_address` VARCHAR(45) NOT NULL DEFAULT '' COMMENT 'IP地址',
  `user_agent` VARCHAR(512) NOT NULL DEFAULT '' COMMENT 'User-Agent',
  `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间（可空：GORM 未显式赋值时会写 NULL，与 V2 库一致）',
  `update_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间（可空，同上）',
  PRIMARY KEY (`id`),
  INDEX `idx_log_user` (`user_id`),
  INDEX `idx_log_module` (`module`),
  INDEX `idx_log_operation_type` (`operation_type`),
  INDEX `idx_log_create_time` (`create_time`),
  CONSTRAINT `fk_log_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='操作日志表';
