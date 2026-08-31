-- ============================================================
-- 005：给 supervisor 角色补充 task:create（创建评教任务）权限
-- ------------------------------------------------------------
-- 背景：移动端课表页支持教师将课程加入"待评课表"（调用 POST /tasks），
--       该路由要求 task:create 权限（见 router.go：RequirePermission("task:create")）。
--       004 仅给 teacher 分配了该权限。若某账号同时是"督导老师(supervisor)"
--       又需要把自己的课程加进待评任务，删除 teacher 角色后该功能会失效
--       （权限并集丢失 task:create，路由直接 403）。
--       故在此给 supervisor 补充 task:create，使其在无 teacher 角色时仍可建待评任务。
-- 范围：checkTaskTargetScope 督导分支天然限定只能操作自己负责学院内的教师任务，
--       因此仅需补 task:create 即可，无需新增细粒度权限码。
-- 幂等：使用 CASE + JSON_CONTAINS 判断，已存在 task:create 的角色保持不变；
--       重复执行不会重复追加，可安全重复启动。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
--       如需校级督导(school_supervisor)/院级督导(college_supervisor)同样支持，
--       在下方 WHERE code IN (...) 中追加对应编码即可。
-- ============================================================

UPDATE `role`
SET `permissions` = CASE
    WHEN `permissions` IS NULL OR `permissions` = '' THEN JSON_ARRAY('task:create')
    WHEN NOT JSON_CONTAINS(`permissions`, '"task:create"') THEN JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'task:create')
    ELSE `permissions`
  END
WHERE `code` IN ('supervisor');
