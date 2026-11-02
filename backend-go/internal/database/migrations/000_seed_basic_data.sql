-- ============================================================
-- 000：基础种子数据（内置角色 / 校区 / 评教维度分组与维度）
-- ------------------------------------------------------------
-- 背景：此前由 V2 的 init_data.py 与 migration_rbac_roles.sql 在旧端初始化，
--       空库部署无任何内置角色与评教维度，系统无法使用。本文件把这些静态
--       基础数据收进迁移链，空库自动初始化。
-- 内容：内置角色 6 个（迁移链 002-017 会继续幂等补齐/自愈权限码，故此处按
--       V2 migration_rbac_roles.sql 的基线权限原样种子化）、校区 2 个、
--       评教维度分组 8 个、评教维度 23 个（V2 init_data.py 原数据；其中
--       field_type=textarea 与 Go 端枚举不符，改用 text）。
-- 幂等：角色/分组/维度均按业务编码去重（INSERT IGNORE 依赖 000_base_schema
--       的唯一键，已存在即跳过，不覆盖用户改过的配置）；校区按名称去重。
-- 不含：默认账号（admin/S001/T001/D001）——口令需按环境变量
--       DEFAULT_USER_PASSWORD 做 bcrypt，SQL 无法表达，由启动时
--       database.SeedAccounts 幂等创建（对齐 V2 init_data.py 行为）。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

-- 内置角色（基线权限，002-017 迁移启动时自动补齐增量权限码）
INSERT IGNORE INTO `role` (`name`, `code`, `description`, `level`, `permissions`, `data_scope`, `is_system`, `status`) VALUES
('系统管理员', 'system_admin', '拥有系统所有权限', 1, '["user:view","user:create","user:update","user:delete","org:view","campus:manage","college:manage","research_room:manage","role:manage","dimension:manage","task:view","task:create","task:update","task:delete","evaluation:view","evaluation:create","evaluation:view_anonymous","stats:view","schedule:view","sync:execute"]', 'all', 1, 1),
('学院管理员', 'college_admin', '管理本学院的用户、评教任务等', 10, '["user:view","user:create","user:update","user:delete","org:view","college:manage","research_room:manage","task:view","task:create","task:update","task:delete","evaluation:view","evaluation:view_anonymous","stats:view","schedule:view"]', 'college', 1, 1),
('校级督导', 'school_supervisor', '可跨学院督导评教，查看所有学院数据', 20, '["task:view","task:create","task:update","evaluation:view","evaluation:create","evaluation:view_anonymous","stats:view","schedule:view","user:view"]', 'all', 1, 1),
('院级督导', 'college_supervisor', '仅负责本学院的督导评教工作', 25, '["task:view","task:create","task:update","evaluation:view","evaluation:create","stats:view","schedule:view"]', 'college', 1, 1),
('督导老师', 'supervisor', '通用督导角色（向后兼容）', 22, '["task:view","task:create","task:update","evaluation:view","evaluation:create","stats:view","schedule:view"]', 'college', 1, 1),
('教师', 'teacher', '查看自己的评教记录和课表', 50, '["evaluation:view","evaluation:create","stats:view","schedule:view"]', 'self', 1, 1);

-- 初始校区（按名称去重，不覆盖已有数据）
INSERT INTO `campus` (`name`, `sort_order`, `status`)
SELECT '南宁校区', 1, 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `campus` WHERE `name` = '南宁校区');

INSERT INTO `campus` (`name`, `sort_order`, `status`)
SELECT '桂林校区', 2, 1 FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `campus` WHERE `name` = '桂林校区');

-- 评教维度分组
INSERT IGNORE INTO `dimension_group` (`code`, `name`, `sort_order`, `status`) VALUES
('attendance', '考勤信息', 1, 1),
('evaluation_option', '评价选项', 2, 1),
('attitude', '教学态度', 3, 1),
('content', '教学内容', 4, 1),
('method', '教学方法与手段', 5, 1),
('effect', '教学效果', 6, 1),
('comment', '评语及建议', 7, 1),
('listening', '听课情况', 8, 1);

-- 评教维度（group_id 由分组编码动态解析；code 已存在则跳过）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
-- 考勤信息
((SELECT `id` FROM `dimension_group` WHERE `code` = 'attendance'), 'attendance_check', '课堂考勤', 'single_choice', '{"options":[{"label":"考勤","value":"present"},{"label":"未考勤","value":"absent"}]}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'attendance'), 'expected_count', '应到人数', 'number', '{"placeholder":"请输入应到人数","min":0}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'attendance'), 'actual_count', '实到人数', 'number', '{"placeholder":"请输入实到人数","min":0}', 1, 3, 1),
-- 评价选项
((SELECT `id` FROM `dimension_group` WHERE `code` = 'evaluation_option'), 'teaching_documents', '教学文档', 'multiple_choice', '{"options":[{"label":"教学大纲","value":"syllabus"},{"label":"教材","value":"textbook"},{"label":"教案","value":"lesson_plan"},{"label":"教学进度表","value":"schedule"},{"label":"学生考勤表","value":"attendance_sheet"}]}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'evaluation_option'), 'teaching_progress', '教学进度', 'single_choice', '{"options":[{"label":"符合","value":"match"},{"label":"稍快","value":"fast"},{"label":"稍慢","value":"slow"},{"label":"差距较大(4学时以上)","value":"large_gap"}]}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'evaluation_option'), 'classroom_order', '课堂秩序', 'single_choice', '{"options":[{"label":"好","value":"good"},{"label":"较好","value":"fair"},{"label":"不好","value":"poor"}]}', 1, 3, 1),
-- 教学态度
((SELECT `id` FROM `dimension_group` WHERE `code` = 'attitude'), 'attitude_punctual', '按时上、下课，无迟到、拖堂等现象', 'score', '{"min_score":0,"max_score":5,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'attitude'), 'attitude_management', '严格课堂管理，检查学生出勤情况', 'score', '{"min_score":0,"max_score":5,"step":1}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'attitude'), 'attitude_manner', '仪态大方、精神饱满、富有感染力', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 3, 1),
-- 教学内容
((SELECT `id` FROM `dimension_group` WHERE `code` = 'content'), 'content_objective', '教学目标明确，符合教学大纲', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'content'), 'content_familiar', '熟悉教学内容，节奏流畅，重点难点突出', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'content'), 'content_innovation', '教学理念/设计/改革具有高阶性、创新性、挑战度', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 3, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'content'), 'content_ideology', '课程思政融入明显', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 4, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'content'), 'content_practical', '理论联系实际，注重能力培养', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 5, 1),
-- 教学方法与手段
((SELECT `id` FROM `dimension_group` WHERE `code` = 'method'), 'method_blackboard', '板书合理、课件规范', 'score', '{"min_score":0,"max_score":5,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'method'), 'method_interactive', '启发式/讨论式教学，注重互动', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 2, 1),
-- 教学效果
((SELECT `id` FROM `dimension_group` WHERE `code` = 'effect'), 'effect_atmosphere', '课堂气氛活跃，听课率高', 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'effect'), 'effect_innovation', '能启发学生创新，收获大', 'score', '{"min_score":0,"max_score":5,"step":1}', 1, 2, 1),
-- 评语及建议（V2 用 textarea，Go 端枚举无此项，改用 text）
((SELECT `id` FROM `dimension_group` WHERE `code` = 'comment'), 'comment_advantages', '教学优点', 'text', '{"placeholder":"请输入教学优点"}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'comment'), 'comment_problems', '存在问题', 'text', '{"placeholder":"请输入存在的问题"}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'comment'), 'comment_suggestions', '改进建议', 'text', '{"placeholder":"请输入改进建议"}', 1, 3, 1),
-- 听课情况
((SELECT `id` FROM `dimension_group` WHERE `code` = 'listening'), 'listening_content', '听课内容', 'text', '{"placeholder":"请记录听课的具体内容、课堂情况"}', 0, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'listening'), 'attendance_rate', '出勤率', 'number', '{"placeholder":"请输入出勤率百分比(0-100)","min":0,"max":100,"unit":"%"}', 0, 2, 1);
