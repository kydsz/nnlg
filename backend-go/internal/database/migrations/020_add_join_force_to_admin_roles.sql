-- ============================================================
-- 020：为内置管理员角色下发「超员加入评教任务」权限（task:join_force）
-- ------------------------------------------------------------
-- 背景：任务人数上限默认 4（system_setting.task_max_participants），满员后普通评教人
--       不能加入；管理员需要保留强制加入的处理通道。新增权限码 task:join_force
--       （见 internal/service/role.go PermissionCatalog）。
-- 授予口径：仅管理员（college_admin / school_admin）默认持有；system_admin 由权限
--       中间件与 AllPermissionCodes() 直通，无需补码；督导与教师不默认下发。
-- 幂等：已含目标码者保持不变；permissions 为 NULL / 空串的内置管理员角色直接初始化
--       为仅含目标码的 JSON 数组（对齐 015/017 口径）；重复启动均安全。
-- ============================================================

UPDATE `role`
SET `permissions` = IF(
      `permissions` IS NULL OR `permissions` = '',
      JSON_ARRAY('task:join_force'),
      JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'task:join_force')
    )
WHERE `code` IN ('college_admin', 'school_admin')
  AND (`permissions` IS NULL OR `permissions` = ''
       OR NOT JSON_CONTAINS(`permissions`, '"task:join_force"'));
