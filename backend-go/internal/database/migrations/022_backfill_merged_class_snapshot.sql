-- 022 回填合班课程的应到人数快照（幂等，可重复启动）。
-- 背景：合班课程在教务课表里是「一个班一行」（同课程+周次模式+星期+节次+教室多行），
--       历史任务在加入待评固化快照时只取了单行班级的 (N) 人数当应到，
--       导致实到÷应到 虚高、导出出勤率超过 100%。
-- 做法：快照 class_info 能精确对应某条明细行、且该行属于多行「同一堂课」时，
--       把快照的 student_count 回填为该堂课合并人数之和、class_info 回填为合并班级串。
-- 口径保守：合班行里任何一行解析不出 (N) 人数时不回填（不臆造总和）；
--          仅在合并人数大于快照现值时更新（保证幂等，且不动本来就正确的快照）。
UPDATE evaluation_task t
JOIN course_schedule cs
  ON cs.teacher_id = t.teacher_id AND cs.is_current = 1
JOIN course_schedule_detail d
  ON d.schedule_id = cs.id
 AND d.course_name = t.course_name
 AND d.class_info = JSON_UNQUOTE(JSON_EXTRACT(t.schedule_snapshot, '$.class_info'))
 AND d.week_day = WEEKDAY(t.class_time) + 1
 AND d.section = CASE
      WHEN TIME(t.class_time) BETWEEN '08:30:00' AND '10:05:00' THEN '0102'
      WHEN TIME(t.class_time) BETWEEN '10:25:00' AND '12:00:00' THEN '0304'
      WHEN TIME(t.class_time) BETWEEN '14:00:00' AND '15:35:00' THEN '0506'
      WHEN TIME(t.class_time) BETWEEN '15:45:00' AND '17:20:00' THEN '0708'
      WHEN TIME(t.class_time) BETWEEN '18:20:00' AND '19:55:00' THEN '0910'
      WHEN TIME(t.class_time) BETWEEN '20:05:00' AND '21:40:00' THEN '1112'
      ELSE ''
    END
JOIN (
  SELECT d2.schedule_id, d2.course_name, d2.week_pattern, d2.week_day, d2.section, d2.classroom,
         COUNT(*) AS n_rows,
         GROUP_CONCAT(d2.class_info ORDER BY d2.id SEPARATOR ',') AS merged_info,
         SUM(CASE WHEN SUBSTRING_INDEX(SUBSTRING_INDEX(d2.class_info, '(', -1), ')', 1) REGEXP '^[0-9]+$'
                  THEN CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(d2.class_info, '(', -1), ')', 1) AS DECIMAL(10,2))
                  ELSE 0 END) AS merged_sum
  FROM course_schedule_detail d2
  GROUP BY d2.schedule_id, d2.course_name, d2.week_pattern, d2.week_day, d2.section, d2.classroom
  HAVING n_rows > 1
     AND SUM(SUBSTRING_INDEX(SUBSTRING_INDEX(d2.class_info, '(', -1), ')', 1) REGEXP '^[0-9]+$') = n_rows
) g
  ON g.schedule_id = d.schedule_id AND g.course_name = d.course_name
 AND g.week_pattern = d.week_pattern AND g.week_day = d.week_day
 AND g.section = d.section AND g.classroom = d.classroom
SET t.schedule_snapshot = JSON_SET(t.schedule_snapshot,
      '$.student_count', g.merged_sum,
      '$.class_info', g.merged_info)
WHERE t.class_time IS NOT NULL
  AND t.schedule_snapshot IS NOT NULL
  AND JSON_EXTRACT(t.schedule_snapshot, '$.student_count') IS NOT NULL
  AND g.merged_sum > CAST(JSON_UNQUOTE(JSON_EXTRACT(t.schedule_snapshot, '$.student_count')) AS DECIMAL(10,2));
