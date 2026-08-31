-- ============================================================
-- 006：修复多角色督导提交评教后被记为教师评教的历史数据
-- ------------------------------------------------------------
-- 背景：评教提交（service.Evaluation.Submit）落库角色与"督导已评"标志原先取
--       user.role 主角色单列判断（model.IsSupervisorRole(viewer.Role)）。
--       同时拥有"教师 + 督导"角色的账号，主角色列为 teacher、督导角色记在
--       user_role 关联表，导致其提交的评教被记成 teacher 同行评教：
--       evaluation_record.evaluator_role 未记督导编码、
--       evaluation_task.has_supervisor_eval 未置位（移动端/管理端显示"督导未评"）。
--       代码已改为多角色判定（User.SupervisorRole() 督导优先），本迁移修复存量数据。
-- 范围：仅调整"评教人在 user_role 中持有督导角色、但记录角色不是督导编码"的
--       未删除评教记录；再按修好的记录回填任务的督导已评标志。
--       多督导角色并存时按 校级督导 > 督导老师 > 院级督导 取优先（与代码一致）。
-- 幂等：两条 UPDATE 均按条件收敛，重复执行命中 0 行，可安全重复启动。
-- ============================================================

UPDATE evaluation_record r
JOIN (
    SELECT ur.user_id,
           SUBSTRING_INDEX(
               GROUP_CONCAT(ur.role ORDER BY FIELD(ur.role, 'school_supervisor', 'supervisor', 'college_supervisor')),
               ',', 1
           ) AS supervisor_role
    FROM user_role ur
    WHERE ur.role IN ('school_supervisor', 'supervisor', 'college_supervisor')
    GROUP BY ur.user_id
) s ON s.user_id = r.evaluator_id
SET r.evaluator_role = s.supervisor_role
WHERE r.is_deleted = 0
  AND r.evaluator_role NOT IN ('school_supervisor', 'supervisor', 'college_supervisor');

UPDATE evaluation_task t
SET t.has_supervisor_eval = 1
WHERE t.has_supervisor_eval = 0
  AND EXISTS (
      SELECT 1
      FROM evaluation_record r
      WHERE r.task_id = t.id
        AND r.is_deleted = 0
        AND r.evaluator_role IN ('school_supervisor', 'supervisor', 'college_supervisor')
  );
