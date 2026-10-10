-- ============================================================
-- 016：为已持有 delete 码的内置角色镜像下发 restore 码（恢复能力）
-- ------------------------------------------------------------
-- 背景：为软删除的评教任务（EvaluationTask）与作废的评教记录（EvaluationRecord）
--       补上「恢复」能力。新增权限码 task:restore / task:restore_own /
--       evaluation:restore / evaluation:restore_own（见 internal/service/role.go
--       PermissionCatalog）。系统管理员经 AllPermissionCodes() 自动获得全部码；
--       其余内置角色需迁移镜像下发，否则恢复入口对其不可见/不可用。
-- 授予口径：restore 是管理端专属动作——恢复入口（Tasks.tsx / Evaluations.tsx）都在
--       AdminGate 之后，仅 system_admin / college_admin / school_admin 可达；系统管理员
--       另经 AllPermissionCodes() 恒获全部码。管理员恒持完整 delete 码，故只镜像
--       「完整 delete -> 完整 restore」两条：
--       task:delete        -> task:restore
--       evaluation:delete  -> evaluation:restore
--       *_own 变体不再自动镜像下发：task:restore_own / evaluation:restore_own 的唯一
--       可能受让者是非管理端角色（teacher / 督导），它们进不了恢复 UI，自动下发只会造出
--       「能删/作废、却不能恢复」的死权限（见 P1-1）。这类角色删除/作废后如需恢复，统一
--       联系管理员（与移动端删除确认文案「如需恢复请联系管理员」一致）。两个 *_own 码仍
--       保留在 PermissionCatalog 中，可由管理员在「角色管理」页按需手动分配。
-- 幂等：追加用 JSON_ARRAY_APPEND + NOT JSON_CONTAINS 判断，已含目标码者保持不变；
--       移除用 JSON_REMOVE + JSON_SEARCH，命中才改、无残留则跳过；重复启动均安全。
--       WHERE 一律先排除 permissions 为 NULL / 空串的行，避免对非法 JSON 报错。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'task:restore')
WHERE `permissions` IS NOT NULL AND `permissions` <> ''
  AND JSON_CONTAINS(`permissions`, '"task:delete"')
  AND NOT JSON_CONTAINS(`permissions`, '"task:restore"');

-- 自愈：移除非管理端内置角色残留的 task:restore_own（历史版本曾按 delete_own 镜像下发，
-- 但恢复入口在 AdminGate 之后，这些角色用不到，属死权限）。JSON_SEARCH 命中才改，幂等。
UPDATE `role`
SET `permissions` = JSON_REMOVE(
      CAST(`permissions` AS JSON),
      JSON_UNQUOTE(JSON_SEARCH(CAST(`permissions` AS JSON), 'one', 'task:restore_own'))
    )
WHERE `code` IN ('teacher', 'supervisor', 'school_supervisor', 'college_supervisor')
  AND `permissions` IS NOT NULL AND `permissions` <> ''
  AND JSON_SEARCH(CAST(`permissions` AS JSON), 'one', 'task:restore_own') IS NOT NULL;

UPDATE `role`
SET `permissions` = JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'evaluation:restore')
WHERE `permissions` IS NOT NULL AND `permissions` <> ''
  AND JSON_CONTAINS(`permissions`, '"evaluation:delete"')
  AND NOT JSON_CONTAINS(`permissions`, '"evaluation:restore"');

-- 自愈：同理移除非管理端内置角色残留的 evaluation:restore_own（当前无角色持有
-- evaluation:delete_own，此语句通常为无操作，仅为与 task 侧口径一致、防未来残留）。
UPDATE `role`
SET `permissions` = JSON_REMOVE(
      CAST(`permissions` AS JSON),
      JSON_UNQUOTE(JSON_SEARCH(CAST(`permissions` AS JSON), 'one', 'evaluation:restore_own'))
    )
WHERE `code` IN ('teacher', 'supervisor', 'school_supervisor', 'college_supervisor')
  AND `permissions` IS NOT NULL AND `permissions` <> ''
  AND JSON_SEARCH(CAST(`permissions` AS JSON), 'one', 'evaluation:restore_own') IS NOT NULL;
