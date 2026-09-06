import { describe, expect, it } from 'vitest'
import { defaultFiltersFor, showMyCreatedHint, MY_CREATED_EMPTY_HINT } from './homeFilters'

describe('defaultFiltersFor', () => {
  it('初始视图（无学期）：创建者默认「我创建的」，状态与关键词为默认空值', () => {
    expect(defaultFiltersFor()).toEqual({
      semester: undefined,
      statusFilter: '',
      creatorFilter: 'my_created',
      keyword: '',
    })
  })

  it('学期为主轴：切换学期回到该学期的默认态，只保留学期', () => {
    const drifted = {
      semester: '2025-2026-1',
      statusFilter: '2' as const,
      creatorFilter: 'other_created' as const,
      keyword: '高数',
    }
    expect(defaultFiltersFor('2025-2026-2')).toEqual({
      semester: '2025-2026-2',
      statusFilter: '',
      creatorFilter: 'my_created',
      keyword: '',
    })
    // 旧筛选不残留
    expect(defaultFiltersFor('2025-2026-2')).not.toEqual(drifted)
  })
})

describe('showMyCreatedHint', () => {
  it('「我创建的」空列表需要引导提示', () => {
    expect(showMyCreatedHint(defaultFiltersFor('2025-2026-1'))).toBe(true)
  })

  it('「全部任务」「其他创建的」空列表不提示', () => {
    expect(
      showMyCreatedHint({ ...defaultFiltersFor('2025-2026-1'), creatorFilter: '' })
    ).toBe(false)
    expect(
      showMyCreatedHint({ ...defaultFiltersFor('2025-2026-1'), creatorFilter: 'other_created' })
    ).toBe(false)
  })

  it('提示文案指向「全部任务」', () => {
    expect(MY_CREATED_EMPTY_HINT).toContain('全部任务')
  })
})
