import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { scheduleApi } from '@/api/modules/schedule'
import type { SemesterConfig } from '@/api/types'

/**
 * 移动端页面共用的学期数据：学期配置、学期下拉选项（倒序）、当前学期。
 * queryKey 与首页一致，多页共享缓存。
 */
export function useSemesters() {
  const { data: semesterConfigs } = useQuery({
    queryKey: ['semester-configs'],
    queryFn: () => scheduleApi.semesterConfigs(),
  })
  const { data: currentSemester } = useQuery({
    queryKey: ['semester-current'],
    queryFn: () => scheduleApi.currentSemester(),
  })
  const configList: SemesterConfig[] = semesterConfigs || []
  const semesterOptions = useMemo(
    () => Array.from(new Set(configList.map((c) => c.semester))).sort().reverse(),
    [configList]
  )
  return { configList, semesterOptions, currentSemester }
}
