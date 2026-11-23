-- ============================================================
-- 022：评教任务学院快照（人事学院 + 授课学院）
-- ------------------------------------------------------------
-- 背景：任务、评教记录、导出与统计原先把「被评教师主学院」当作学院口径，
--       外聘教师授课学院与人事学院不一致时会归错学院。
-- 方案：任务创建时固化两个快照：
--       teacher_college_id = 被评教师人事学院
--       course_college_id  = 本次评教的授课/开课学院
--       读路径统一 course_college_id 优先，空则回退 teacher_college_id / 教师主学院。
-- 历史数据：回填为教师当前人事学院，保证统计连续。
-- 幂等：ADD COLUMN 重复执行由迁移器忽略 1060；ADD INDEX 忽略 1061；
--       回填只处理 NULL 行。不加外键（迁移器不忽略 1826 重复约束）。
-- ============================================================

ALTER TABLE `evaluation_task`
  ADD COLUMN `teacher_college_id` INT NULL COMMENT '被评教师人事学院快照';

ALTER TABLE `evaluation_task`
  ADD COLUMN `course_college_id` INT NULL COMMENT '任务授课/开课学院快照';

ALTER TABLE `evaluation_task`
  ADD INDEX `idx_task_teacher_college` (`teacher_college_id`);

ALTER TABLE `evaluation_task`
  ADD INDEX `idx_task_course_college` (`course_college_id`);

UPDATE `evaluation_task` t
JOIN `user` u ON u.id = t.teacher_id
SET t.teacher_college_id = COALESCE(t.teacher_college_id, u.college_id),
    t.course_college_id  = COALESCE(t.course_college_id, u.college_id)
WHERE t.teacher_college_id IS NULL OR t.course_college_id IS NULL;
