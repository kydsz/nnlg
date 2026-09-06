-- ============================================================
-- 008：给 teacher 角色补充 evaluation:create（提交评教）权限
-- ------------------------------------------------------------
-- 背景：POST /evaluations 路由强制 RequirePermission("evaluation:create")
--       （见 router.go：evaluations.POST("", RequirePermission("evaluation:create"), ...)），
--       但 role 表 teacher 角色的初始 permissions 里没有该权限码
--       （仅 supervisor/system_admin 有）→ 全部教师账号提交评教直接 403。
--       代码里教师同行评教分支（ValidateSubmit：不能评自己/只能评同学院）
--       与移动端提交页均完整存在，历史记录全为 admin/supervisor 提交，
--       确认为初始迁移遗漏而非设计。
-- 范围：仅补路由门所需的 evaluation:create；业务约束（不能评自己、
--       只能评同学院、任务范围）仍由 service 层 ValidateSubmit 把关。
-- 幂等：使用 CASE + JSON_CONTAINS 判断，已存在 evaluation:create 的角色
--       保持不变；重复执行不会重复追加，可安全重复启动。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
--       2026-08-31 已在生产库直接 UPDATE 过一次（备份：teacher_perm_backup.txt），
--       本迁移与其效果一致，重复执行安全。
-- ============================================================

UPDATE `role`
SET `permissions` = CASE
    WHEN `permissions` IS NULL OR `permissions` = '' THEN JSON_ARRAY('evaluation:create')
    WHEN NOT JSON_CONTAINS(`permissions`, '"evaluation:create"') THEN JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'evaluation:create')
    ELSE `permissions`
  END
WHERE `code` IN ('teacher');
