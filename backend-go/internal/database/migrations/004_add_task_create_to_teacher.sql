-- ============================================================
-- 004：给 teacher 角色补充 task:create（创建评教任务）权限
-- ------------------------------------------------------------
-- 背景：移动端课表页支持教师将课程加入"待评课表"（调用 POST /tasks），
--       该路由要求 task:create 权限。teacher 角色的历史权限数据可能
--       未包含该权限码，导致"普通教师无法添加自己的课程/本院教师课程进待评任务"。
-- 范围：后端 checkTaskTargetScope 教师分支天然限定只能为同学院教师
--       （含自己）创建任务，因此仅需补 task:create 即可，无需新增细粒度权限码。
-- 幂等：使用 CASE + JSON_CONTAINS 判断，已存在 task:create 的角色保持不变；
--       重复执行不会重复追加，可安全重复启动。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = CASE
    WHEN `permissions` IS NULL OR `permissions` = '' THEN JSON_ARRAY('task:create')
    WHEN NOT JSON_CONTAINS(`permissions`, '"task:create"') THEN JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'task:create')
    ELSE `permissions`
  END
WHERE `code` IN ('teacher');
