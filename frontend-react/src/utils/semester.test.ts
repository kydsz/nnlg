import { describe, expect, it } from 'vitest'
import dayjs from 'dayjs'
import { semesterRangeOf } from './semester'
import type { SemesterConfig } from '@/api/types'

const configs: SemesterConfig[] = [
  { id: 1, semester: '2025-2026-1', start_date: '2025-09-01', weeks: 18, is_current: true },
  { id: 2, semester: '2025-2026-2', start_date: '', weeks: 20, is_current: false },
]

describe('semesterRangeOf', () => {
  it('有配置：区间为开学日 ~ 开学日+周数*7-1 天', () => {
    expect(semesterRangeOf(configs, '2025-2026-1')).toEqual(['2025-09-01', '2026-01-04'])
  })

  it('配置缺开学日：回退默认开学日 + 默认 20 周', () => {
    const [start, end] = semesterRangeOf(configs, '2025-2026-2')
    expect(start).toBe('2026-02-17')
    expect(end).toBe(dayjs(start).add(20 * 7 - 1, 'day').format('YYYY-MM-DD'))
  })

  it('无配置的学期：整体回退默认开学日', () => {
    const [start, end] = semesterRangeOf(configs, '2024-2025-1')
    expect(start).toBe('2024-09-01')
    expect(end).toBe(dayjs(start).add(20 * 7 - 1, 'day').format('YYYY-MM-DD'))
  })

  it('未选学期（初始态/全部学期）：无区间', () => {
    expect(semesterRangeOf(configs, undefined)).toEqual([])
    expect(semesterRangeOf(undefined, '2025-2026-1')).toEqual(['2025-09-01', '2026-01-18'])
  })
})
