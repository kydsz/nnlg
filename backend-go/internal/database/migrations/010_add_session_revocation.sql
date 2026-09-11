-- ============================================================
-- 010：会话撤销 DB 权威源（user 表加 session_epoch / refresh jti）
-- ------------------------------------------------------------
-- 背景：会话 epoch 撤销此前仅依赖 Redis，默认部署（REDIS_ADDR 为空）下
--       登出/改密不吊销已签发 JWT，为隐私与安全缺口。本次将撤销计数
--       落库为权威源（session_epoch），Redis 仅作加速缓存。
--   refresh_jti / refresh_jti_prev：refresh token 轮换与重放检测。
--       轮换时将当前 jti 写入 refresh_jti、旧值移入 refresh_jti_prev；
--       再次使用 refresh_jti_prev 即判定为重放/盗用，撤销全部会话。
-- 幂等：重复执行报 Duplicate column 由 Migrate 跳过。
-- ============================================================

ALTER TABLE `user`
  ADD COLUMN `session_epoch` BIGINT NOT NULL DEFAULT 0 COMMENT '会话撤销计数（DB 权威），登出/禁用/改密自增使旧 token 失效',
  ADD COLUMN `refresh_jti` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '当前有效 refresh token 的 jti',
  ADD COLUMN `refresh_jti_prev` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '上一轮 refresh token 的 jti（重放检测）';