-- ============================================================
-- 018：为内置管理员角色下发「查看他人评教详情」权限（evaluation:view_all）
-- ------------------------------------------------------------
-- 背景：评教记录列表/详情的可见性分两层——evaluation:view_all 决定「能否看他人评教」，
--       AccessibleCollegeIDs 决定「能看哪些学院的」（见 internal/service/evaluation.go
--       buildRecordQuery）。学院管理员未持有该码时落入「仅自己评的 + 评给自己的」
--       分支，而管理员本人既非评教人也非被评教师，评教记录页恒为空，看不到本学院
--       任何记录；其已持有的 evaluation:view_anonymous / evaluation:update 也随之
--       无的放矢。与 RBAC.md「匿名评教查看权限」表中「学院管理员可查看本学院
--       完整信息」的口径相悖。
-- 授予口径：仅管理员（college_admin / school_admin）默认持有；system_admin 自动；
--       三类督导不默认下发——评教记录含匿名身份，督导按 RBAC.md 口径仅看自己评的，
--       需要看他人评教时由系统管理员在「角色管理」页勾选「查看他人评教详情」（与
--       015 给督导补 task:view_all 不同：任务不含匿名身份，评教记录含）。
-- 幂等：已含目标码者保持不变；permissions 为 NULL / 空串的内置管理员角色直接初始化为
--       仅含目标码的 JSON 数组（对齐 017 的初始化口径）；重复启动均安全。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = IF(
      `permissions` IS NULL OR `permissions` = '',
      JSON_ARRAY('evaluation:view_all'),
      JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'evaluation:view_all')
    )
WHERE `code` IN ('college_admin', 'school_admin')
  AND (`permissions` IS NULL OR `permissions` = ''
       OR NOT JSON_CONTAINS(`permissions`, '"evaluation:view_all"'));
