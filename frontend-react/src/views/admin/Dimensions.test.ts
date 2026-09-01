import { describe, expect, it } from 'vitest'
import { configSummary, normalizeOptions, toOptionList } from './Dimensions'

describe('toOptionList', () => {
  it('解析对象数组', () => {
    expect(toOptionList([{ label: '教学大纲', value: 'syllabus' }])).toEqual([
      { label: '教学大纲', value: 'syllabus' },
    ])
  })

  it('兼容历史 JSON 字符串（二次编码数据）', () => {
    expect(
      toOptionList('[{"label":"优秀","value":"excellent"},{"label":"良好","value":"good"}]')
    ).toEqual([
      { label: '优秀', value: 'excellent' },
      { label: '良好', value: 'good' },
    ])
  })

  it('兼容纯字符串数组与非法输入', () => {
    expect(toOptionList(['优秀', '良好'])).toEqual([
      { label: '优秀', value: '优秀' },
      { label: '良好', value: '良好' },
    ])
    expect(toOptionList(undefined)).toEqual([])
    expect(toOptionList(null)).toEqual([])
    expect(toOptionList('不是JSON')).toEqual([])
    expect(toOptionList(42)).toEqual([])
  })

  it('缺失 label 时回退 value/value 缺失时回退 label', () => {
    expect(toOptionList([{ value: 'v1' }, { label: 'L2' }])).toEqual([
      { label: '', value: 'v1' },
      { label: 'L2', value: 'L2' },
    ])
  })
})

describe('normalizeOptions', () => {
  it('value 留空时自动取 label', () => {
    expect(
      normalizeOptions([
        { label: '考勤', value: '' },
        { label: '未考勤', value: '' },
      ])
    ).toEqual([
      { label: '考勤', value: '考勤' },
      { label: '未考勤', value: '未考勤' },
    ])
  })

  it('保留显式 value（如 考勤|present）', () => {
    expect(
      normalizeOptions([
        { label: '考勤', value: 'present' },
        { label: '未考勤', value: 'absent' },
      ])
    ).toEqual([
      { label: '考勤', value: 'present' },
      { label: '未考勤', value: 'absent' },
    ])
  })

  it('去除首尾空白并过滤空文本项', () => {
    expect(
      normalizeOptions([
        { label: '  优秀 ', value: 'excellent' },
        { label: '  ', value: 'x' },
      ])
    ).toEqual([{ label: '优秀', value: 'excellent' }])
  })

  it('兼容字符串数组与非法输入', () => {
    expect(normalizeOptions(undefined)).toEqual([])
    expect(normalizeOptions(['考勤', '未考勤'])).toEqual([
      { label: '考勤', value: '考勤' },
      { label: '未考勤', value: '未考勤' },
    ])
  })
})

describe('configSummary', () => {
  it('选项展示为 label 顿号连接', () => {
    expect(
      configSummary({
        options: [
          { label: '教学大纲', value: 'syllabus' },
          { label: '教学方法', value: 'method' },
        ],
      })
    ).toBe('选项：教学大纲、教学方法')
  })

  it('评分类型展示分值，默认步长 1 不展示', () => {
    expect(configSummary({ min_score: 0, max_score: 20, step: 1 })).toBe('分值：0 ~ 20')
    expect(configSummary({ min_score: 0, max_score: 20, step: 0.5 })).toBe(
      '分值：0 ~ 20；步长：0.5'
    )
  })

  it('文本类型展示占位提示', () => {
    expect(configSummary({ placeholder: '请输入' })).toBe('提示：请输入')
  })

  it('数字类型展示范围', () => {
    expect(configSummary({ min_value: 1, max_value: 10 })).toBe('范围：1 ~ 10')
  })

  it('未知配置回退 JSON，空配置展示 -', () => {
    expect(configSummary({ foo: 'bar' })).toBe('{"foo":"bar"}')
    expect(configSummary({})).toBe('-')
    expect(configSummary(null)).toBe('-')
  })
})
