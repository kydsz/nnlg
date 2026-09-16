-- ============================================================
-- 015：内置角色补充 task:view_all（查看他人评教任务）权限
-- ------------------------------------------------------------
-- 背景：评教任务的可见范围改为「默认只看与我相关的任务」（我创建的 / 我被评的 /
--       我评过的），需要查看本学院（或负责学院/全校）他人任务的角色，必须显式
--       持有 task:view_all。接口侧由 internal/service/task.go 的 buildQuery 在
--       服务端强制收敛，前端筛选参数无法绕过。
-- 授予对象：管理员（本学院范围）、三类督导（负责学院 / 全校范围）。
--       teacher 不授予——普通教师默认只能看到与我相关的任务；确需放开时，由
--       管理员在「角色管理」页勾选「评教任务·查看他人评教任务」分配，无需改代码。
-- 幂等：使用 CASE + JSON_CONTAINS 判断，已存在 task:view_all 的角色保持不变；
--       重复执行不会重复追加，可安全重复启动。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = CASE
    WHEN `permissions` IS NULL OR `permissions` = '' THEN JSON_ARRAY('task:view_all')
    WHEN NOT JSON_CONTAINS(`permissions`, '"task:view_all"') THEN JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'task:view_all')
    ELSE `permissions`
  END
WHERE `code` IN ('college_admin', 'school_admin', 'school_supervisor', 'college_supervisor', 'supervisor');
