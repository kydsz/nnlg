-- ============================================================
-- 002：内置角色补充 task:view（查看评教任务）权限
-- ------------------------------------------------------------
-- 背景：任务列表/详情/导出接口均要求 task:view 权限。teacher / supervisor /
--       college_supervisor 等内置角色的历史权限数据可能未包含该权限码，
--       导致"普通教师查看不了本学院评教任务"（接口直接 403）。
-- 幂等：使用 CASE + JSON_CONTAINS 判断，已存在 task:view 的角色保持不变；
--       重复执行不会重复追加，可安全重复启动。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = CASE
    WHEN `permissions` IS NULL OR `permissions` = '' THEN JSON_ARRAY('task:view')
    WHEN NOT JSON_CONTAINS(`permissions`, '"task:view"') THEN JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'task:view')
    ELSE `permissions`
  END
WHERE `code` IN ('teacher', 'supervisor', 'college_supervisor');
