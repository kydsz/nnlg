import { describe, expect, it } from 'vitest'
import dayjs from 'dayjs'
import type { EvaluationRecord } from '@/api/types'
import {
  buildTaskUpdatePayload,
  isTaskChanged,
  taskFieldsFromRecord,
} from './evalTaskEdit'

function makeRecord(over: Partial<EvaluationRecord> = {}): EvaluationRecord {
  return {
    id: 1,
    task_id: 10,
    teacher_id: 100,
    teacher_name: '被评教师',
    course_name: '高等数学',
    class_time: '2026-03-02T08:00:00',
    classroom: '1-101',
    evaluator_id: 200,
    evaluator_name: '评教人',
    dimension_values: {},
    is_anonymous: false,
    submit_time: '2026-03-02 12:00',
    schedule: {
      class_time_text: '周一 第1-2节',
      classroom: '1-101',
      class_info: '2401(50)',
      student_count: 50,
      week_pattern: '第1-16周',
    },
    ...over,
  }
}

describe('evalTaskEdit', () => {
  it('详情转表单初值：快照与任务字段合并', () => {
    const f = taskFieldsFromRecord(makeRecord())
    expect(f).toEqual({
      teacherId: 100,
      courseName: '高等数学',
      classTime: dayjs('2026-03-02T08:00:00'),
      classroom: '1-101',
      classTimeText: '周一 第1-2节',
      classInfo: '2401(50)',
      studentCount: 50,
      weekPattern: '第1-16周',
    })
  })

  it('无改动：不判定变化、不产生 task 载荷', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    expect(isTaskChanged(rec, f)).toBe(false)
    expect(buildTaskUpdatePayload(rec, f)).toBeUndefined()
  })

  it('只换教师：仅回传 teacher_id', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    f.teacherId = 300
    expect(buildTaskUpdatePayload(rec, f)).toEqual({ teacher_id: 300 })
  })

  it('只改课程名：仅回传 course_name', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    f.courseName = '线性代数'
    expect(buildTaskUpdatePayload(rec, f)).toEqual({ course_name: '线性代数' })
  })

  it('只改上课时间：回传后端时间格式', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    f.classTime = dayjs('2026-03-03 10:00:00')
    expect(buildTaskUpdatePayload(rec, f)).toEqual({ class_time: '2026-03-03 10:00:00' })
  })

  it('只改教室：同时更新任务与快照教室', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    f.classroom = '2-202'
    expect(buildTaskUpdatePayload(rec, f)).toEqual({
      classroom: '2-202',
      schedule: {
        class_time_text: '周一 第1-2节',
        classroom: '2-202',
        class_info: '2401(50)',
        student_count: 50,
        week_pattern: '第1-16周',
      },
    })
  })

  it('只改周次：仅回传 schedule（含其余快照字段原值）', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    f.weekPattern = '第1-8周'
    expect(buildTaskUpdatePayload(rec, f)).toEqual({
      schedule: {
        class_time_text: '周一 第1-2节',
        classroom: '1-101',
        class_info: '2401(50)',
        student_count: 50,
        week_pattern: '第1-8周',
      },
    })
  })

  it('原无快照：补填班级也会生成 schedule', () => {
    const rec = makeRecord({ schedule: undefined })
    const f = taskFieldsFromRecord(rec)
    f.classInfo = '2402(60)'
    expect(buildTaskUpdatePayload(rec, f)).toEqual({
      schedule: {
        class_time_text: null,
        classroom: '1-101',
        class_info: '2402(60)',
        student_count: null,
        week_pattern: null,
      },
    })
  })

  it('只改周次且任务/快照教室不一致：不把任务教室误写进快照', () => {
    const rec = makeRecord({
      classroom: '2-202',
      schedule: {
        class_time_text: '周一 第1-2节',
        classroom: '1-101',
        class_info: '2401(50)',
        student_count: 50,
        week_pattern: '第1-16周',
      },
    })
    const f = taskFieldsFromRecord(rec)
    expect(f.classroom).toBe('2-202')
    f.weekPattern = '第1-8周'
    expect(buildTaskUpdatePayload(rec, f)).toEqual({
      schedule: {
        class_time_text: '周一 第1-2节',
        classroom: '1-101',
        class_info: '2401(50)',
        student_count: 50,
        week_pattern: '第1-8周',
      },
    })
  })

  it('同时改教师与时间：两类字段一并回传', () => {
    const rec = makeRecord()
    const f = taskFieldsFromRecord(rec)
    f.teacherId = 300
    f.classTime = dayjs('2026-03-03 10:00:00')
    expect(buildTaskUpdatePayload(rec, f)).toEqual({
      teacher_id: 300,
      class_time: '2026-03-03 10:00:00',
    })
  })
})
