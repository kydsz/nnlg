import { describe, expect, it } from 'vitest'
import { ROLE_NAMES, roleName, roleNamesStr } from '@/utils/roleNames'

describe('ROLE_NAMES', () => {
  it('覆盖全部 7 种角色', () => {
    expect(Object.keys(ROLE_NAMES)).toEqual([
      'system_admin',
      'school_admin',
      'college_admin',
      'school_supervisor',
      'supervisor',
      'college_supervisor',
      'teacher',
    ])
  })
})

describe('roleName', () => {
  it('已知编码转换为中文名', () => {
    expect(roleName('system_admin')).toBe('系统管理员')
    expect(roleName('teacher')).toBe('教师')
    expect(roleName('school_supervisor')).toBe('校级督导')
  })

  it('未知编码原样返回', () => {
    expect(roleName('future_role')).toBe('future_role')
  })

  it('空值返回占位符', () => {
    expect(roleName(null)).toBe('-')
    expect(roleName(undefined)).toBe('-')
    expect(roleName('')).toBe('-')
  })
})

describe('roleNamesStr', () => {
  it('多个角色以" / "连接', () => {
    expect(roleNamesStr(['teacher', 'supervisor'])).toBe('教师 / 督导老师')
  })

  it('未知编码保留原样', () => {
    expect(roleNamesStr(['teacher', 'unknown'])).toBe('教师 / unknown')
  })

  it('空列表返回占位符', () => {
    expect(roleNamesStr([])).toBe('-')
    expect(roleNamesStr(null)).toBe('-')
    expect(roleNamesStr(undefined)).toBe('-')
  })
})
