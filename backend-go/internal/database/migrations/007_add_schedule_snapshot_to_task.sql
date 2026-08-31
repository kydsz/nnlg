-- ============================================================
-- 007：任务表新增课表信息快照字段 schedule_snapshot
-- ------------------------------------------------------------
-- 背景：评教记录详情/导出需要展示"提交评教时看到的课表信息"
--       （上课时间/教室/班级/应到人数/周次）。此前这些信息在
--       读详情时实时去 course_schedule_detail 现查，会随课表
--       变化漂移，且与"加入待评任务"时点不一致。
--       现改为在"把课程添加进待评任务"时，将整块课表信息固化为
--       JSON 快照存到 evaluation_task.schedule_snapshot，
--       详情/导出优先读取该快照。
-- 结构：JSON 对象，含
--       class_time_text / classroom / class_info /
--       student_count / week_pattern（与提交页展示一致）。
-- 幂等：1060 Duplicate column name 会被迁移器自动忽略。
-- ============================================================

ALTER TABLE evaluation_task
  ADD COLUMN schedule_snapshot JSON NULL COMMENT '课表信息快照（加入待评时固化）' AFTER classroom;
