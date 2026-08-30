import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { statsApi } from '@/api/modules/stats'

/** 当前学期信息（开学日 ~ 开学日+20周），学期变更后各评教页面数据随之变动 */
export function useSemesterDates() {
  const { data } = useQuery({
    // 独立 key，避免与课表页 current-semester 缓存（semester-configs/current）冲突
    queryKey: ['current-semester-info'],
    queryFn: () => statsApi.currentSemester(),
  })
  const dates = useMemo<[string, string] | []>(() => {
    if (!data?.start_date || !data?.end_date) return []
    return [data.start_date, data.end_date]
  }, [data])
  return { currentSemester: data, dates }
}

/**
 * 学期默认日期选择器状态：
 * 未手动修改时始终跟随当前学期（整学期区间）；用户手动选择后以手动值为准；清空后回落学期默认。
 */
export function useSemesterRangePicker() {
  const { dates } = useSemesterDates()
  const [override, setOverride] = useState<[string, string] | null>(null)
  const effective: [string?, string?] = override ?? (dates.length === 2 ? dates : [])
  const onRange = (v: [string, string] | null) => setOverride(v)
  return { effective, onRange }
}