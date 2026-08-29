import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  formatSemester,
  generateSemesters,
  formatDate,
  parseSections,
  formatClassPeriod,
  taskStatusInfo,
  parseWeekPattern,
  isCourseInWeek,
  defaultSemesterStart,
  currentWeekOf,
  getDateForWeekAndDay,
  formatSectionText,
} from '@/utils/format'

describe('formatSemester', () => {
  it('标准学期编码转换为中文', () => {
    expect(formatSemester('2024-2025-1')).toBe('2024-2025学年第1学期')
    expect(formatSemester('2024-2025-2')).toBe('2024-2025学年第2学期')
  })

  it('非法输入原样返回', () => {
    expect(formatSemester('2024-2025')).toBe('2024-2025')
    expect(formatSemester('abc')).toBe('abc')
  })
})

describe('generateSemesters', () => {
  it('默认 2023-2030 生成 16 项且倒序', () => {
    const list = generateSemesters()
    expect(list).toHaveLength(16)
    expect(list[0]).toBe('2030-2031-1')
    expect(list[list.length - 1]).toBe('2023-2024-2')
  })

  it('自定义年份范围，每学年两学期', () => {
    expect(generateSemesters(2024, 2024)).toEqual(['2024-2025-1', '2024-2025-2'])
  })
})

describe('formatDate', () => {
  it('默认带时间输出', () => {
    expect(formatDate('2025-09-15T08:30:00')).toBe('2025-09-15 08:30')
  })

  it('withTime=false 只输出日期', () => {
    expect(formatDate('2025-09-15T08:30:00', false)).toBe('2025-09-15')
  })

  it('空值输出占位符', () => {
    expect(formatDate(null)).toBe('-')
    expect(formatDate(undefined)).toBe('-')
    expect(formatDate('')).toBe('-')
  })
})

describe('parseSections', () => {
  it('按两位数字拆分', () => {
    expect(parseSections('0102')).toEqual(['01', '02'])
    expect(parseSections('010203')).toEqual(['01', '02', '03'])
  })

  it('混入非数字字符时提取数字段', () => {
    expect(parseSections('a01b02')).toEqual(['01', '02'])
  })

  it('空值返回空数组', () => {
    expect(parseSections('')).toEqual([])
    expect(parseSections('5')).toEqual([])
  })
})

describe('formatSectionText', () => {
  it('连续节次输出范围', () => {
    expect(formatSectionText('0102')).toBe('第1-2节')
    expect(formatSectionText('0304')).toBe('第3-4节')
  })

  it('单节输出单节格式', () => {
    expect(formatSectionText('05')).toBe('第5节')
  })

  it('多段取首尾范围', () => {
    expect(formatSectionText('010203')).toBe('第1-3节')
  })

  it('空值输出占位符', () => {
    expect(formatSectionText('')).toBe('-')
    expect(formatSectionText(undefined)).toBe('-')
  })
})

describe('formatClassPeriod', () => {
  it('落在时段内返回对应节次', () => {
    expect(formatClassPeriod('2025-09-15T08:30:00')).toBe('第1-2节')
    expect(formatClassPeriod('2025-09-15T10:05:00')).toBe('第1-2节')
    expect(formatClassPeriod('2025-09-15T10:25:00')).toBe('第3-4节')
    expect(formatClassPeriod('2025-09-15T19:55:00')).toBe('第9-10节')
  })

  it('时段之外返回 null', () => {
    expect(formatClassPeriod('2025-09-15T12:10:00')).toBeNull()
    expect(formatClassPeriod('2025-09-15T22:00:00')).toBeNull()
  })

  it('空值或非法时间返回 null', () => {
    expect(formatClassPeriod(null)).toBeNull()
    expect(formatClassPeriod('not-a-date')).toBeNull()
  })
})

describe('taskStatusInfo', () => {
  it('三种状态映射正确', () => {
    expect(taskStatusInfo(1)).toEqual({ text: '待评', color: 'orange' })
    expect(taskStatusInfo(2)).toEqual({ text: '已评', color: 'green' })
    expect(taskStatusInfo(3)).toEqual({ text: '已取消', color: 'red' })
  })

  it('未知状态兜底', () => {
    expect(taskStatusInfo(99)).toEqual({ text: '状态99', color: 'default' })
  })
})

describe('parseWeekPattern', () => {
  it('空值与"全周"类返回 null 表示恒真', () => {
    expect(parseWeekPattern(null)).toBeNull()
    expect(parseWeekPattern('')).toBeNull()
    expect(parseWeekPattern('全周')).toBeNull()
    expect(parseWeekPattern('全部')).toBeNull()
    expect(parseWeekPattern('每周')).toBeNull()
  })

  it('单周返回 1,3,...,19', () => {
    expect(parseWeekPattern('单周')).toEqual([1, 3, 5, 7, 9, 11, 13, 15, 17, 19])
    expect(parseWeekPattern('奇数周')).toEqual(parseWeekPattern('单周'))
  })

  it('双周返回 2,4,...,20', () => {
    expect(parseWeekPattern('双周')).toEqual([2, 4, 6, 8, 10, 12, 14, 16, 18, 20])
    expect(parseWeekPattern('偶数周')).toEqual(parseWeekPattern('双周'))
  })

  it('解析范围与枚举混合表达式', () => {
    expect(parseWeekPattern('1-12周')).toEqual(
      Array.from({ length: 12 }, (_, i) => i + 1)
    )
    expect(parseWeekPattern('1-2,4,7-8周')).toEqual([1, 2, 4, 7, 8])
    expect(parseWeekPattern('9周')).toEqual([9])
  })

  it('支持 en dash 与波浪线分隔', () => {
    expect(parseWeekPattern('3–5')).toEqual([3, 4, 5])
    expect(parseWeekPattern('3~5')).toEqual([3, 4, 5])
  })

  it('结果去重并升序', () => {
    expect(parseWeekPattern('3,1-2,3')).toEqual([1, 2, 3])
  })

  it('无数字时返回 null', () => {
    expect(parseWeekPattern('abc')).toBeNull()
  })
})

describe('isCourseInWeek', () => {
  it('恒真模式任意周均上课', () => {
    expect(isCourseInWeek('全周', 15)).toBe(true)
    expect(isCourseInWeek(null, 15)).toBe(true)
  })

  it('按解析出的周集合判断', () => {
    expect(isCourseInWeek('1-3周', 2)).toBe(true)
    expect(isCourseInWeek('1-3周', 4)).toBe(false)
    expect(isCourseInWeek('单周', 3)).toBe(true)
    expect(isCourseInWeek('单周', 4)).toBe(false)
    expect(isCourseInWeek('双周', 4)).toBe(true)
  })
})

describe('defaultSemesterStart', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-09-15T10:00:00'))
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('第1学期默认起始日为起始学年 09-01', () => {
    expect(defaultSemesterStart('2024-2025-1')).toBe('2024-09-01')
  })

  it('第2学期默认起始日为结束学年 02-17', () => {
    expect(defaultSemesterStart('2024-2025-2')).toBe('2025-02-17')
  })

  it('非法学期返回今天', () => {
    expect(defaultSemesterStart('bad')).toBe('2025-09-15')
  })
})

describe('currentWeekOf', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2025-09-15T10:00:00'))
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('起始日当天为第1周', () => {
    expect(currentWeekOf('2025-09-15')).toBe(1)
  })

  it('不足7天仍算第1周', () => {
    expect(currentWeekOf('2025-09-10')).toBe(1)
  })

  it('满7天进入第2周', () => {
    expect(currentWeekOf('2025-09-08')).toBe(2)
  })

  it('超过20周截断为20', () => {
    expect(currentWeekOf('2025-01-01')).toBe(20)
  })

  it('未来日期不小于1', () => {
    expect(currentWeekOf('2025-12-01')).toBe(1)
  })
})

describe('getDateForWeekAndDay', () => {
  it('起始日为周一时，第1周周一即起始日', () => {
    expect(getDateForWeekAndDay('2025-09-15', 1, 1).format('YYYY-MM-DD')).toBe('2025-09-15')
  })

  it('weekDay 逐日递增', () => {
    expect(getDateForWeekAndDay('2025-09-15', 1, 4).format('YYYY-MM-DD')).toBe('2025-09-18')
    expect(getDateForWeekAndDay('2025-09-15', 1, 7).format('YYYY-MM-DD')).toBe('2025-09-21')
  })

  it('跨周推算：第2周周一 = 起始日 + 7 天', () => {
    expect(getDateForWeekAndDay('2025-09-15', 2, 1).format('YYYY-MM-DD')).toBe('2025-09-22')
  })

  it('起始日为周日时，所在周的周一是往前 6 天', () => {
    expect(getDateForWeekAndDay('2025-09-14', 1, 1).format('YYYY-MM-DD')).toBe('2025-09-08')
  })
})
