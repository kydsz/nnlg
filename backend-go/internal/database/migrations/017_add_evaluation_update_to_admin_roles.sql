-- ============================================================
-- 017：为内置管理员角色下发「编辑评教记录」权限（evaluation:update）
-- ------------------------------------------------------------
-- 背景：管理员需要修改评教记录的全部可编辑信息（维度作答、匿名标记，以及记录所挂
--       课堂的课程信息——教师/课程/上课时间/教室/课表快照，改的是任务本身，影响该
--       任务下全部记录）。新增权限码 evaluation:update（见 internal/service/role.go
--       PermissionCatalog）；系统管理员经 AllPermissionCodes() 自动获得全部码；
--       其余内置角色需迁移下发，否则记录编辑入口对其不可见/不可用。
-- 授予口径：仅管理员（college_admin / school_admin）默认持有；system_admin 自动；
--       督导与教师不默认下发（原记录编辑入口借用 evaluation:delete，督导/教师本就
--       进不了管理端，本迁移不改变这一边界）。历史自定义角色如原先依赖
--       evaluation:delete 编辑记录，可由系统管理员在「角色管理」页改配 evaluation:update。
-- 幂等：已含目标码者保持不变；permissions 为 NULL / 空串的内置管理员角色直接初始化为
--       仅含目标码的 JSON 数组（对齐 015 的初始化口径）；重复启动均安全。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = IF(
      `permissions` IS NULL OR `permissions` = '',
      JSON_ARRAY('evaluation:update'),
      JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'evaluation:update')
    )
WHERE `code` IN ('college_admin', 'school_admin')
  AND (`permissions` IS NULL OR `permissions` = ''
       OR NOT JSON_CONTAINS(`permissions`, '"evaluation:update"'));
