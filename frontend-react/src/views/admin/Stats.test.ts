import { describe, expect, it } from 'vitest'
import {
  statsTabItems,
  SUPERVISORS_TAB_KEY,
  SUPERVISORS_TAB_PERM,
  CAMPUS_TAB_KEY,
  CAMPUS_TAB_PERM,
} from './Stats'

// 页签 key 列表（Tabs items 的顺序即展示顺序）
const keysOf = (canViewSupervisors: boolean, canViewCampus: boolean) =>
  statsTabItems(canViewSupervisors, canViewCampus).map((item) => item.key)

describe('statsTabItems', () => {
  it('持 stats:view_supervisors 时展示督导统计页签', () => {
    expect(keysOf(true, true)).toContain(SUPERVISORS_TAB_KEY)
    expect(keysOf(true, true)).toEqual([
      'teachers',
      'colleges',
      CAMPUS_TAB_KEY,
      SUPERVISORS_TAB_KEY,
      'records',
      'unteached',
    ])
  })

  it('未持 stats:view_supervisors 时隐藏督导统计页签', () => {
    expect(keysOf(false, true)).not.toContain(SUPERVISORS_TAB_KEY)
    expect(keysOf(false, true)).toEqual(['teachers', 'colleges', CAMPUS_TAB_KEY, 'records', 'unteached'])
  })

  it('未持 stats:view_campus 时隐藏校区统计页签（学院管理员口径）', () => {
    expect(keysOf(true, false)).not.toContain(CAMPUS_TAB_KEY)
    expect(keysOf(true, false)).toEqual(['teachers', 'colleges', SUPERVISORS_TAB_KEY, 'records', 'unteached'])
    expect(keysOf(false, false)).toEqual(['teachers', 'colleges', 'records', 'unteached'])
  })

  it('默认页签始终是教师统计（Tabs 默认取首项）', () => {
    expect(keysOf(true, true)[0]).toBe('teachers')
    expect(keysOf(false, false)[0]).toBe('teachers')
  })

  it('隐藏督导统计/校区统计不影响其他页签的标签文案', () => {
    const labels = statsTabItems(false, true).map((item) => item.label)
    expect(labels).toEqual(['教师统计', '学院统计', '校区统计', '听课明细', '未被听课教师'])
    const labelsNoCampus = statsTabItems(false, false).map((item) => item.label)
    expect(labelsNoCampus).toEqual(['教师统计', '学院统计', '听课明细', '未被听课教师'])
  })

  it('页签权限码与后端 role.go / router.go 的码保持一致', () => {
    // 后端：service/role.go「统计报表」组 stats:view_supervisors / stats:view_campus
    //       router.go GET /stats/supervisors、/stats/export/supervisors
    //                 GET /stats/campus、/stats/campuses
    expect(SUPERVISORS_TAB_PERM).toBe('stats:view_supervisors')
    expect(SUPERVISORS_TAB_KEY).toBe('supervisors')
    expect(CAMPUS_TAB_PERM).toBe('stats:view_campus')
    expect(CAMPUS_TAB_KEY).toBe('campus')
  })
})
