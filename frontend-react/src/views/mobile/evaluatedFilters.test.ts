import { describe, expect, it } from 'vitest'
import {
  defaultEvaluatedFiltersFor,
  buildEvaluatedListParams,
  type EvaluatedFilters,
} from './evaluatedFilters'

const RANGE = ['2025-09-01', '2025-12-28'] as const

describe('defaultEvaluatedFiltersFor', () => {
  it('初始态不传学期，由当前学期异步定位补上', () => {
    expect(defaultEvaluatedFiltersFor()).toEqual({ semester: undefined, keyword: '' })
  })

  it('传入学期即该学期的默认视图', () => {
    expect(defaultEvaluatedFiltersFor('2025-2026-1')).toEqual({ semester: '2025-2026-1', keyword: '' })
  })
})

describe('buildEvaluatedListParams', () => {
  const base = { userId: 7, page: 1 }

  it('评给我的：teacher_id = 自己', () => {
    const p = buildEvaluatedListParams({
      type: 'received', filters: defaultEvaluatedFiltersFor('2025-2026-1'), range: RANGE, ...base,
    })
    expect(p).toEqual({ page: 1, page_size: 20, teacher_id: 7, start_date: '2025-09-01', end_date: '2025-12-28' })
  })

  it('我评的：evaluator_id = 自己', () => {
    const p = buildEvaluatedListParams({
      type: 'sent', filters: defaultEvaluatedFiltersFor('2025-2026-1'), range: RANGE, ...base,
    })
    expect(p).toEqual({ page: 1, page_size: 20, evaluator_id: 7, start_date: '2025-09-01', end_date: '2025-12-28' })
  })

  it('「全部学期」（空串）：不传日期区间', () => {
    const p = buildEvaluatedListParams({
      type: 'sent', filters: { semester: '', keyword: '' }, range: RANGE, ...base,
    })
    expect(p.start_date).toBeUndefined()
    expect(p.end_date).toBeUndefined()
  })

  it('初始态（undefined）：不传日期区间，等当前学期定位', () => {
    const p = buildEvaluatedListParams({
      type: 'received', filters: defaultEvaluatedFiltersFor(), range: [], ...base,
    })
    expect(p.start_date).toBeUndefined()
  })

  it('页码与页大小随请求推进', () => {
    const p = buildEvaluatedListParams({
      type: 'sent', filters: { semester: '', keyword: '' }, range: [], userId: 7, page: 3,
    })
    expect(p.page).toBe(3)
    expect(p.page_size).toBe(20)
  })

  it('关键词去除首尾空格后传给服务端，空白词不传', () => {
    const withKw = buildEvaluatedListParams({
      type: 'sent', filters: { semester: '', keyword: '  高等数学  ' }, range: [], userId: 7, page: 1,
    })
    expect(withKw.keyword).toBe('高等数学')

    const blank = buildEvaluatedListParams({
      type: 'sent', filters: { semester: '', keyword: '   ' }, range: [], userId: 7, page: 1,
    })
    expect(blank.keyword).toBeUndefined()
  })

  it('关键词与学期筛选叠加生效', () => {
    const p = buildEvaluatedListParams({
      type: 'received', filters: { semester: '2025-2026-1', keyword: '王五' }, range: RANGE, userId: 7, page: 1,
    })
    expect(p.keyword).toBe('王五')
    expect(p.start_date).toBe('2025-09-01')
  })
})
