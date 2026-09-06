import type { EvaluationListParams } from '@/api/modules/evaluations'

export type EvaluatedTab = 'received' | 'sent'

/**
 * 已评记录页筛选。semester 为 undefined 表示初始态（由当前学期异步定位补上）；
 * 空串表示「全部学期」（不传日期区间）。
 */
export interface EvaluatedFilters {
  semester: string | undefined
}

export function defaultEvaluatedFiltersFor(semester?: string): EvaluatedFilters {
  return { semester }
}

/** 已评记录列表的查询参数：服务端分页 + 筛选（学期→日期区间按「记录所属学期」口径） */
export function buildEvaluatedListParams(args: {
  type: EvaluatedTab
  userId: number
  filters: EvaluatedFilters
  range: readonly [string, string] | []
  page: number
}): EvaluationListParams {
  const { type, userId, filters, range, page } = args
  const params: EvaluationListParams = { page, page_size: 20 }
  if (type === 'received') {
    params.teacher_id = userId
  } else {
    params.evaluator_id = userId
  }
  if (filters.semester && range.length === 2) {
    params.start_date = range[0]
    params.end_date = range[1]
  }
  return params
}
