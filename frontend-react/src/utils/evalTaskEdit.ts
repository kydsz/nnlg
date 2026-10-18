import dayjs from 'dayjs'
import type { EvaluationRecord } from '@/api/types'
import type { EvaluationTaskPayload } from '@/api/modules/evaluations'

/** 编辑评教记录时的课堂课程信息表单态 */
export interface TaskFormFields {
  teacherId?: number
  courseName: string
  classTime: dayjs.Dayjs | null
  classroom: string
  classTimeText: string
  classInfo: string
  studentCount: number | null
  weekPattern: string
}

const TIME_FMT = 'YYYY-MM-DD HH:mm:ss'

/** 后端时间（ISO/SQL 字符串）统一输出为 YYYY-MM-DD HH:mm:ss，空值输出 null */
const fmtTaskTime = (v?: string | null): string | null =>
  v ? dayjs(v).format(TIME_FMT) : null

/** 评教详情 → 课堂信息表单初值 */
export function taskFieldsFromRecord(rec: EvaluationRecord): TaskFormFields {
  return {
    teacherId: rec.teacher_id ?? undefined,
    courseName: rec.course_name || '',
    classTime: rec.class_time ? dayjs(rec.class_time) : null,
    classroom: rec.classroom || rec.schedule?.classroom || '',
    classTimeText: rec.schedule?.class_time_text || '',
    classInfo: rec.schedule?.class_info || '',
    studentCount: rec.schedule?.student_count ?? null,
    weekPattern: rec.schedule?.week_pattern || '',
  }
}

const sameTime = (a: dayjs.Dayjs | null, b?: string | null) =>
  (a ? a.format(TIME_FMT) : null) === fmtTaskTime(b)

/** 课堂信息是否相对详情发生变化（决定是否显示影响提示、是否回传 task） */
export function isTaskChanged(orig: EvaluationRecord, next: TaskFormFields): boolean {
  return (
    next.teacherId !== (orig.teacher_id ?? undefined) ||
    next.courseName !== (orig.course_name || '') ||
    !sameTime(next.classTime, orig.class_time) ||
    next.classroom !== (orig.classroom || orig.schedule?.classroom || '') ||
    next.classTimeText !== (orig.schedule?.class_time_text || '') ||
    next.classInfo !== (orig.schedule?.class_info || '') ||
    (next.studentCount ?? null) !== (orig.schedule?.student_count ?? null) ||
    next.weekPattern !== (orig.schedule?.week_pattern || '')
  )
}

/**
 * 构建 task 更新载荷：
 * - 未变化返回 undefined——记录编辑不触碰任务（避免已删任务/重复校验误伤记录侧保存）；
 * - 仅回传发生变化字段，未传字段后端不改。
 */
export function buildTaskUpdatePayload(
  orig: EvaluationRecord,
  next: TaskFormFields
): EvaluationTaskPayload | undefined {
  if (!isTaskChanged(orig, next)) return undefined

  const payload: EvaluationTaskPayload = {}
  if (next.teacherId !== orig.teacher_id) payload.teacher_id = next.teacherId
  if (next.courseName !== (orig.course_name || '')) payload.course_name = next.courseName

  const cls = next.classTime ? next.classTime.format(TIME_FMT) : null
  if (cls && cls !== fmtTaskTime(orig.class_time)) payload.class_time = cls

  const origClassroom = orig.classroom || orig.schedule?.classroom || ''
  if (next.classroom !== origClassroom) {
    payload.classroom = next.classroom
  }

  // 快照教室与任务教室口径可能不同：只在用户改动教室（相对初值）或改动其它快照字段时重建快照；
  // 重建时教室优先沿用快照原值（无快照回退任务值），避免「只改周次」把任务教室误写进快照
  const classroomTouched = next.classroom !== origClassroom
  const scheduleChanged =
    classroomTouched ||
    next.classTimeText !== (orig.schedule?.class_time_text || '') ||
    next.classInfo !== (orig.schedule?.class_info || '') ||
    (next.studentCount ?? null) !== (orig.schedule?.student_count ?? null) ||
    next.weekPattern !== (orig.schedule?.week_pattern || '')
  if (scheduleChanged) {
    payload.schedule = {
      class_time_text: next.classTimeText || null,
      classroom: (classroomTouched ? next.classroom : orig.schedule?.classroom ?? next.classroom) || null,
      class_info: next.classInfo || null,
      student_count: next.studentCount,
      week_pattern: next.weekPattern || null,
    }
  }
  return payload
}
