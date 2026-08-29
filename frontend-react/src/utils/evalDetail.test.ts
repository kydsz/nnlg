import { describe, expect, it } from 'vitest'
import type {
  EvaluationDimDetail,
  EvaluationRecord,
  DimensionSchemaGroup,
} from '@/api/types'
import {
  isImageUrl,
  isImageArray,
  isFileArray,
  getFileNameFromUrl,
  formatValueText,
  groupEvaluationDimensions,
  buildEvalPrintHtml,
} from '@/utils/evalDetail'

function makeRecord(overrides: Partial<EvaluationRecord> = {}): EvaluationRecord {
  return {
    id: 1,
    task_id: 1,
    teacher_id: 100,
    teacher_name: '张老师',
    course_name: '高等数学',
    evaluator_id: 2,
    evaluator_name: '李同学',
    dimension_values: {},
    is_anonymous: false,
    submit_time: '2025-09-15T10:00:00',
    ...overrides,
  }
}

describe('isImageUrl', () => {
  it('识别 data URI 与图片扩展名', () => {
    expect(isImageUrl('data:image/png;base64,xxx')).toBe(true)
    expect(isImageUrl('http://x.com/a.jpg')).toBe(true)
    expect(isImageUrl('http://x.com/a.PNG')).toBe(true)
    expect(isImageUrl('http://x.com/a.jpeg?token=1')).toBe(true)
  })

  it('/files/ 下按扩展名判断（图片是图片、文件不是图片）', () => {
    expect(isImageUrl('/files/photo_12345.jpg')).toBe(true)
    expect(isImageUrl('/api/v1/files/evaluations/1/q1/uuid_现场.png')).toBe(true)
    // 后端 filepath 为 {uuid}_{原名}，非图片扩展名不得判为图片
    expect(isImageUrl('/files/report.pdf')).toBe(false)
    expect(isImageUrl('/files/evidence.docx')).toBe(false)
    expect(isImageUrl('/api/v1/files/evaluations/1/q1/uuid_report.pdf')).toBe(false)
  })

  it('非图片返回 false', () => {
    expect(isImageUrl('http://x.com/a.pdf')).toBe(false)
    expect(isImageUrl('http://x.com/a')).toBe(false)
  })
})

describe('isImageArray', () => {
  it('全为图片 URL 的数组返回 true', () => {
    expect(isImageArray(['http://x/a.jpg', '/files/b.png'])).toBe(true)
  })

  it('空数组或非图片数组返回 false', () => {
    expect(isImageArray([])).toBe(false)
    expect(isImageArray(['http://x/a.pdf'])).toBe(false)
  })

  it('非字符串元素返回 false', () => {
    expect(isImageArray([1, 2])).toBe(false)
    expect(isImageArray('http://x/a.jpg')).toBe(false)
  })
})

describe('isFileArray', () => {
  it('非图片 URL 数组返回 true', () => {
    expect(isFileArray(['http://x/a.pdf'])).toBe(true)
    expect(isFileArray(['/files/report.docx'])).toBe(true)
  })

  it('图片数组返回 false', () => {
    expect(isFileArray(['http://x/a.jpg'])).toBe(false)
  })

  it('非法元素返回 false', () => {
    expect(isFileArray([123])).toBe(false)
    expect(isFileArray([])).toBe(false)
  })
})

describe('getFileNameFromUrl', () => {
  it('取 URL 最后一段作为文件名', () => {
    expect(getFileNameFromUrl('http://localhost:3000/files/成绩表.xlsx')).toBe('成绩表.xlsx')
  })

  it('对 URL 编码的文件名解码', () => {
    expect(getFileNameFromUrl('http://localhost:3000/files/%E6%88%90%E7%BB%A9.pdf')).toBe(
      '成绩.pdf'
    )
  })

  it('相对路径基于 origin 解析', () => {
    expect(getFileNameFromUrl('/files/a.pdf')).toBe('a.pdf')
  })

  it('空路径回退为"未知文件"', () => {
    expect(getFileNameFromUrl('')).toBe('未知文件')
  })
})

describe('formatValueText', () => {
  it('空值输出占位符', () => {
    expect(formatValueText(null)).toBe('-')
    expect(formatValueText(undefined)).toBe('-')
  })

  it('数组以逗号连接', () => {
    expect(formatValueText([1, 'a'])).toBe('1, a')
  })

  it('对象输出 JSON', () => {
    expect(formatValueText({ a: 1 })).toBe('{"a":1}')
  })

  it('基础类型转字符串', () => {
    expect(formatValueText('x')).toBe('x')
    expect(formatValueText(5)).toBe('5')
    expect(formatValueText(false)).toBe('false')
  })
})

describe('groupEvaluationDimensions - 旧格式 dimension_details', () => {
  const details: EvaluationDimDetail[] = [
    {
      code: 'q1',
      name: '教学态度',
      group_code: 'attitude',
      group_name: '态度组',
      field_type: 'score',
      value: 9,
      score: 9,
      max_score: 10,
    },
    {
      code: 'q2',
      name: '备课情况',
      group_code: 'attitude',
      group_name: '态度组',
      field_type: 'score',
      value: 8,
      score: 8,
      max_score: 10,
    },
    { code: 'q3', name: '备注', field_type: 'text', value: '无' },
  ]

  it('同组合并为一组并累计得分', () => {
    const groups = groupEvaluationDimensions(makeRecord({ dimension_details: details }))
    expect(groups).toHaveLength(2)
    expect(groups[0].key).toBe('attitude')
    expect(groups[0].name).toBe('态度组')
    expect(groups[0].rows).toHaveLength(2)
    expect(groups[0].score).toBe(17)
    expect(groups[0].max_score).toBe(20)
  })

  it('无分组信息的归入"未分组"', () => {
    const groups = groupEvaluationDimensions(makeRecord({ dimension_details: details }))
    expect(groups[1].key).toBe('other')
    expect(groups[1].name).toBe('未分组')
    expect(groups[1].rows[0].code).toBe('q3')
  })

  it('旧格式优先于新格式 schema', () => {
    const groups = groupEvaluationDimensions(
      makeRecord({
        dimension_details: details,
        dimension_groups: [{ id: 1, name: '新格式组', dimensions: [] }],
      })
    )
    expect(groups[0].key).toBe('attitude')
  })
})

describe('groupEvaluationDimensions - 新格式 dimension_groups', () => {
  const schema: DimensionSchemaGroup[] = [
    {
      id: 1,
      name: '教学态度',
      group_code: 'attitude',
      dimensions: [
        { code: 'q1', name: '教学态度', field_type: 'score', max_score: 10 },
        { code: 'q2', name: '评语', field_type: 'text' },
      ],
    },
  ]

  it('按 schema 组装行并累计得分', () => {
    const groups = groupEvaluationDimensions(
      makeRecord({ dimension_values: { q1: 8, q2: '很好' }, dimension_groups: schema })
    )
    expect(groups).toHaveLength(1)
    expect(groups[0].key).toBe('attitude')
    expect(groups[0].rows).toHaveLength(2)
    expect(groups[0].score).toBe(8)
    expect(groups[0].max_score).toBe(10)
    expect(groups[0].rows[1].value).toBe('很好')
    expect(groups[0].rows[1].score).toBeUndefined()
  })

  it('缺 group_code 时以 id 作为 key', () => {
    const groups = groupEvaluationDimensions(
      makeRecord({
        dimension_values: {},
        dimension_groups: [{ id: 7, name: 'G', dimensions: [] }],
      })
    )
    expect(groups[0].key).toBe('7')
  })
})

describe('groupEvaluationDimensions - 兜底', () => {
  it('无 schema 时把 values 平铺为"未分组"', () => {
    const groups = groupEvaluationDimensions(
      makeRecord({ dimension_values: { a: 1, b: 'x' } })
    )
    expect(groups).toHaveLength(1)
    expect(groups[0].key).toBe('other')
    expect(groups[0].name).toBe('未分组')
    expect(groups[0].rows.map((r) => r.code)).toEqual(['a', 'b'])
    expect(groups[0].rows[0].field_type).toBe('number')
    expect(groups[0].score).toBe(0)
  })
})

describe('buildEvalPrintHtml', () => {
  it('包含标题与基本信息', () => {
    const html = buildEvalPrintHtml(
      makeRecord({
        dimension_details: [
          { code: 'q1', name: '总分项', group_code: 'g', group_name: 'G', value: 8, score: 8, max_score: 10 },
        ],
        total_score: 8,
      })
    )
    expect(html).toContain('评教详情')
    expect(html).toContain('张老师')
    expect(html).toContain('高等数学')
    expect(html).toContain('李同学')
    expect(html).toContain('8 / 10')
  })

  it('对用户输入做 HTML 转义，防止注入', () => {
    const html = buildEvalPrintHtml(
      makeRecord({
        teacher_name: '<script>alert(1)</script>',
        dimension_details: [
          { code: 'q1', name: 'N', group_code: 'g', group_name: 'G', display_value: '<b>粗体</b>' },
        ],
      })
    )
    expect(html).not.toContain('<script>alert(1)</script>')
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;')
    expect(html).toContain('&lt;b&gt;粗体&lt;/b&gt;')
  })

  it('图片数组渲染为 img 标签', () => {
    const html = buildEvalPrintHtml(
      makeRecord({
        dimension_details: [
          { code: 'imgs', name: '照片', group_code: 'g', group_name: 'G', value: ['http://x/a.jpg'] },
        ],
      })
    )
    expect(html).toContain('<img src="http://x/a.jpg"')
  })

  it('文件数组渲染为下载链接，文件名解码', () => {
    const html = buildEvalPrintHtml(
      makeRecord({
        dimension_details: [
          { code: 'fs', name: '附件', group_code: 'g', group_name: 'G', value: ['/files/report.pdf'] },
        ],
      })
    )
    expect(html).toContain('<a href="/files/report.pdf"')
    expect(html).toContain('report.pdf</a>')
  })

  it('total_score 缺省时按分组得分求和', () => {
    const html = buildEvalPrintHtml(
      makeRecord({
        dimension_details: [
          { code: 'q1', name: 'A', group_code: 'g', group_name: 'G', value: 8, score: 8, max_score: 10 },
        ],
      })
    )
    expect(html).toContain('<b style="color:#c00;font-size:16px">8')
  })
})
