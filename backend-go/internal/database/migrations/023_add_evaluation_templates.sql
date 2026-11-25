-- 评价模板机制：理论课 / 实践教学两套评价表
-- 模板为产品固定两种，编码在 model.TemplateTheory / model.TemplatePractice 中定义，不建独立表。
-- 清理早期版本创建但未接入业务的 evaluation_template 表（幂等，可安全重复执行）
DROP TABLE IF EXISTS `evaluation_template`;

-- 1) 分组归属模板：common=所有模板通用，theory/practice=仅对应模板
ALTER TABLE `dimension_group`
  ADD COLUMN `template_code` VARCHAR(32) NOT NULL DEFAULT 'common' COMMENT '所属评价模板：common/theory/practice' AFTER `name`;

UPDATE `dimension_group` SET `template_code` = 'theory'
WHERE `code` IN ('attitude', 'content', 'method', 'effect');

-- 3) 任务绑定模板（存量任务默认理论课，口径不变）
ALTER TABLE `evaluation_task`
  ADD COLUMN `template_code` VARCHAR(32) NOT NULL DEFAULT 'theory' COMMENT '评价模板编码' AFTER `course_name`;

-- 4) 实践教学维度名称可能较长，放宽到 255
ALTER TABLE `evaluation_dimension`
  MODIFY COLUMN `name` VARCHAR(255) NOT NULL COMMENT '维度名称';

-- 5) 实践教学评价表：5 个评分维度 + 学生情况（迟到人数）
INSERT IGNORE INTO `dimension_group` (`code`, `name`, `template_code`, `sort_order`, `status`) VALUES
('practice_attitude', '教学态度', 'practice', 10, 1),
('practice_prepare', '教学准备', 'practice', 11, 1),
('practice_content', '教学内容', 'practice', 12, 1),
('practice_method', '教学方法', 'practice', 13, 1),
('practice_effect', '教学效果', 'practice', 14, 1),
('practice_students', '学生情况', 'practice', 15, 1);

-- 教学态度（10 分）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_attitude'),
 'prac_attitude_1',
 '教师思想及教学态度端正，认真履行职责，批改实验、实习报告认真负责、及时，注重教书育人。',
 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 1, 1);

-- 教学准备（10 分）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_prepare'),
 'prac_prepare_1',
 '实验、实习准备工作充分，实验指导书、实验仪器设备、实习教学（生活）设施配套齐全，使用状态良好，指导教师配置合理，满足教学需要。',
 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 1, 1);

-- 教学内容（30 分 = 9 + 9 + 12）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_content'),
 'prac_content_1', '实践教学目的、要求明确，实践项目安排合理',
 'score', '{"min_score":0,"max_score":9,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_content'),
 'prac_content_2', '实践教学环节紧密结合专业（课程）有关知识，基本理论、基本操作原理与方法清楚。',
 'score', '{"min_score":0,"max_score":9,"step":1}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_content'),
 'prac_content_3', '有综合性或设计性训练，实践项目有利于培养学生科学的思维方法及观察分析、解决实际问题的能力。',
 'score', '{"min_score":0,"max_score":12,"step":1}', 1, 3, 1);

-- 教学方法（30 分 = 9 + 12 + 9）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_method'),
 'prac_method_1', '指导教师示范操作正规、准确、熟练，突出重点、善于启发，注意培养学生严谨的科学实践作风。',
 'score', '{"min_score":0,"max_score":9,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_method'),
 'prac_method_2', '指导教师正确熟练指导学生处理实践项目中出现的问题，注重学生动手能力或实际工作能力的训练。',
 'score', '{"min_score":0,"max_score":12,"step":1}', 1, 2, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_method'),
 'prac_method_3', '重视实践教学中的安全教育，对学生要求明确，防范措施得力，无安全事故。',
 'score', '{"min_score":0,"max_score":9,"step":1}', 1, 3, 1);

-- 教学效果（20 分 = 10 + 10）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_effect'),
 'prac_effect_1', '通过实践教学，学生能巩固所学理论知识并掌握生产技术知识。',
 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 1, 1),
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_effect'),
 'prac_effect_2', '通过实践教学，有效提高学生的动手实验能力或独立工作能力。',
 'score', '{"min_score":0,"max_score":10,"step":1}', 1, 2, 1);

-- 学生情况：迟到人数（实践表要求，理论课表单不出现）
INSERT IGNORE INTO `evaluation_dimension` (`group_id`, `code`, `name`, `field_type`, `field_config`, `is_required`, `sort_order`, `status`) VALUES
((SELECT `id` FROM `dimension_group` WHERE `code` = 'practice_students'),
 'prac_late_count', '迟到人数', 'number', '{"placeholder":"请输入迟到人数","min":0}', 0, 1, 1);
