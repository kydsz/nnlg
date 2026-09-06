import dayjs from 'dayjs'
import type { SemesterConfig } from '@/api/types'
import { defaultSemesterStart } from './format'

/**
 * 学期的日期区间（开学日 ~ 开学日+周数*7-1 天），按「记录所属学期」口径
 * 过滤听课时间用。未选学期（初始态/全部学期）返回空数组；
 * 配置缺失时回退默认开学日与默认 20 周。
 */
export function semesterRangeOf(
  configs: SemesterConfig[] | undefined,
  semester: string | undefined
): [string, string] | [] {
  if (!semester) return []
  const selectedCfg = configs?.find((c) => c.semester === semester)
  const start = selectedCfg?.start_date || defaultSemesterStart(semester)
  const weeks = selectedCfg?.weeks || 20
  const end = dayjs(start).add(weeks * 7 - 1, 'day').format('YYYY-MM-DD')
  return [start, end]
}
