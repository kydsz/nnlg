-- ============================================================
-- 003：内置角色补充 schedule:view_college（查看本学院教师课表）
-- ------------------------------------------------------------
-- 背景：课表页「选择教师」弹层原本仅放行管理员（user:view）或督导
--       （schedule:view + 督导角色）。需求：普通教师也能查看本院其他
--       教师的课表，故新增权限码 schedule:view_college 并分配给 teacher
--       角色。后端教师范围由 TeacherScopeOf 天然限定为『教师主学院』。
-- 幂等：使用 CASE + JSON_CONTAINS 判断，已存在 schedule:view_college 的
--       角色保持不变；重复执行不会重复追加，可安全重复启动。
-- 说明：本文件在服务启动时通过 internal/database.Migrate 自动执行（//go:embed）。
-- ============================================================

UPDATE `role`
SET `permissions` = CASE
    WHEN `permissions` IS NULL OR `permissions` = '' THEN JSON_ARRAY('schedule:view_college')
    WHEN NOT JSON_CONTAINS(`permissions`, '"schedule:view_college"') THEN JSON_ARRAY_APPEND(CAST(`permissions` AS JSON), '$', 'schedule:view_college')
    ELSE `permissions`
  END
WHERE `code` IN ('teacher');
