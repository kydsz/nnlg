import type {
  EvaluationRecord,
  EvaluationDimDetail,
  DimensionSchemaGroup,
} from '@/api/types'

export interface EvalDetailRow {
  code: string
  name: string
  field_type: string
  value: unknown
  display_value?: string
  score?: number
  max_score?: number
}

export interface EvalDetailGroup {
  key: string
  name: string
  rows: EvalDetailRow[]
  score: number
  max_score: number
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

const IMAGE_EXT_RE = /\.(jpe?g|png|gif|webp|bmp|svg)(\?.*)?$/i

export function isImageUrl(v: string): boolean {
  // 后端 /files/ 是统一文件服务（图片与 pdf/docx 同前缀），
  // 必须按扩展名判定，否则 /files/report.pdf 会被误判为图片
  return v.startsWith('data:image/') || IMAGE_EXT_RE.test(v)
}

/** 判断字符串数组是否为图片 URL 数组 */
export function isImageArray(v: unknown): v is string[] {
  return (
    Array.isArray(v) &&
    v.length > 0 &&
    v.every((x) => typeof x === 'string') &&
    v.some((x) => isImageUrl(x as string))
  )
}

/**
 * 解析维度 field_config 中的选项映射（value -> label）。
 * 兼容 config.options 为对象数组，或历史遗留的二次编码 JSON 字符串。
 */
export function optionLabelMap(fieldConfig: unknown): Map<string, string> {
  const map = new Map<string, string>()
  if (!fieldConfig || typeof fieldConfig !== 'object') return map
  let options: unknown = (fieldConfig as { options?: unknown }).options
  if (typeof options === 'string') {
    try {
      options = JSON.parse(options)
    } catch {
      return map
    }
  }
  if (!Array.isArray(options)) return map
  for (const o of options) {
    if (!o || typeof o !== 'object') continue
    const rec = o as { label?: unknown; value?: unknown }
    const label = rec.label == null ? '' : String(rec.label)
    const value = rec.value == null ? '' : String(rec.value)
    map.set(value === '' ? label : value, label || value)
  }
  return map
}

/** 单选/多选原始值转显示文本（选项未匹配时保留原值） */
export function formatChoiceText(v: unknown, labels: Map<string, string>): string | undefined {
  const list = (Array.isArray(v) ? (v as unknown[]) : [v])
    .map((x) => String(x))
    .filter((x) => x !== '' && x !== 'null' && x !== 'undefined')
  if (list.length === 0) return undefined
  return list.map((x) => labels.get(x) || x).join('、')
}

/** 判断字符串数组是否为文件（非图片）URL 数组 */
export function isFileArray(v: unknown): v is string[] {
  return (
    Array.isArray(v) &&
    v.length > 0 &&
    v.every((x) => typeof x === 'string' && (x.startsWith('http') || x.startsWith('/') || x.startsWith('data:'))) &&
    !isImageArray(v)
  )
}

export function getFileNameFromUrl(url: string): string {
  try {
    const p = new URL(url, window.location.origin).pathname
    return decodeURIComponent(p.split('/').pop() || '未知文件')
  } catch {
    return url.split('/').pop() || '未知文件'
  }
}

/** 值转展示文本（数组 join、对象 JSON） */
export function formatValueText(v: unknown): string {
  if (v == null) return '-'
  if (Array.isArray(v)) return v.map((x) => String(x)).join(', ')
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

/**
 * 把评教详情组装为分组明细（兼容两种后端格式）：
 * - 旧 Python 后端：dimension_details[] 已含 group/score/display_value
 * - 新 Go 后端：dimension_groups(schema) + dimension_values(原始值)
 */
export function groupEvaluationDimensions(detail: EvaluationRecord): EvalDetailGroup[] {
  // 旧格式
  const legacy = detail.dimension_details
  if (legacy && legacy.length > 0) {
    return groupLegacy(legacy)
  }

  const values = detail.dimension_values || {}
  const groups = detail.dimension_groups
  if (groups && groups.length > 0) {
    return groupFromSchema(groups, values)
  }

  // 兜底：无 schema，把 values 平铺为"未分组"
  const rows: EvalDetailRow[] = Object.entries(values).map(([code, v]) => ({
    code,
    name: code,
    field_type: Array.isArray(v) ? 'array' : typeof v,
    value: v,
  }))
  return [{ key: 'other', name: '未分组', rows, score: 0, max_score: 0 }]
}

function groupLegacy(list: EvaluationDimDetail[]): EvalDetailGroup[] {
  const map = new Map<string, EvalDetailGroup>()
  list.forEach((d, i) => {
    const key = d.group_code || String(d.group_id ?? 'other')
    const name = d.group_name || '未分组'
    let g = map.get(key)
    if (!g) {
      g = { key, name, rows: [], score: 0, max_score: 0 }
      map.set(key, g)
    }
    const row: EvalDetailRow = {
      code: d.code || `item_${i}`,
      name: d.name || d.code || `item_${i}`,
      field_type: d.field_type || 'text',
      value: d.value,
      display_value: d.display_value,
      score: d.score,
      max_score: d.max_score,
    }
    g.rows.push(row)
    if (typeof d.score === 'number') g.score += d.score
    if (typeof d.max_score === 'number') g.max_score += d.max_score
  })
  return [...map.values()]
}

function groupFromSchema(
  groups: DimensionSchemaGroup[],
  values: Record<string, unknown>
): EvalDetailGroup[] {
  const out: EvalDetailGroup[] = []
  for (const g of groups) {
    const grp: EvalDetailGroup = {
      key: g.group_code || String(g.id ?? g.name ?? 'other'),
      name: g.group_name || g.name || '未分组',
      rows: [],
      score: 0,
      max_score: 0,
    }
    for (const dim of g.dimensions || []) {
      const v = values[dim.code]
      const ft = String(dim.field_type || 'text')
      let score: number | undefined
      let maxScore: number | undefined
      if (ft === 'score' && typeof v === 'number') {
        score = v
        maxScore =
          typeof dim.max_score === 'number'
            ? dim.max_score
            : Number((dim as { max_score?: number }).max_score ?? 0)
        grp.score += v
        if (maxScore) grp.max_score += maxScore
      }
      // 单选/多选：原始值映射为中文标签
      let displayValue: string | undefined
      if (ft === 'single_choice' || ft === 'multiple_choice') {
        displayValue = formatChoiceText(v, optionLabelMap((dim as { field_config?: unknown }).field_config))
      }
      grp.rows.push({
        code: dim.code,
        name: dim.name || dim.code,
        field_type: ft,
        value: v,
        display_value: displayValue,
        score,
        max_score: maxScore,
      })
    }
    out.push(grp)
  }
  return out
}

/** 组装打印/导出 HTML（信息区 + 分组明细） */
export function buildEvalPrintHtml(detail: EvaluationRecord): string {
  const groups = groupEvaluationDimensions(detail)
  // 总分优先取后端返回，缺省再按分组累计（兼容旧后端）
  const total = detail.total_score ?? groups.reduce((s, g) => s + g.score, 0)
  const totalMax = detail.max_total_score ?? groups.reduce((s, g) => s + g.max_score, 0)

  const infoRows: [string, string][] = [
    ['教师', detail.teacher_name],
    ['课程', detail.course_name],
    ['评教人', detail.evaluator_name],
    ['评教角色', detail.evaluator_role_name || detail.evaluator_role || '-'],
    ['提交时间', detail.submit_time || '-'],
    ['是否匿名', detail.is_anonymous ? '是' : '否'],
  ]

  const dimHtml = groups
    .map((g) => {
      const rowsHtml = g.rows
        .map((r) => {
          let valHtml: string
          if (isImageArray(r.value)) {
            valHtml = (r.value as string[])
              .map((u) => `<img src="${escapeHtml(u)}" style="max-width:200px;max-height:150px;margin:4px;" />`)
              .join('')
          } else if (isFileArray(r.value)) {
            valHtml = (r.value as string[])
              .map(
                (u) =>
                  `<div><a href="${escapeHtml(u)}" target="_blank">${escapeHtml(getFileNameFromUrl(u))}</a></div>`
              )
              .join('')
          } else {
            valHtml = escapeHtml(r.display_value || formatValueText(r.value))
          }
          return `<tr><td style="border:1px solid #ddd;padding:6px;width:35%">${escapeHtml(r.name)}</td><td style="border:1px solid #ddd;padding:6px">${valHtml}</td></tr>`
        })
        .join('')
      const groupScore =
        g.max_score > 0
          ? `<span style="float:right;color:#c00">${g.score}/${g.max_score}分</span>`
          : ''
      return `<h3 style="border-bottom:2px solid #104186;padding-bottom:4px;margin:16px 0 8px">${escapeHtml(g.name)}${groupScore}</h3><table style="border-collapse:collapse;width:100%">${rowsHtml}</table>`
    })
    .join('')

  return `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>评教详情</title>
<style>body{font-family:"Microsoft YaHei",sans-serif;padding:24px;color:#252525}h1{font-size:20px;text-align:center}table{font-size:13px}</style>
</head><body>
<h1>评教详情</h1>
<p style="text-align:center;color:#999">南宁理工学院</p>
<table style="border-collapse:collapse;width:100%;margin-bottom:8px">
${infoRows
  .map(
    ([k, v]) =>
      `<tr><td style="border:1px solid #ddd;padding:6px;width:25%;background:#f7f9fc">${k}</td><td style="border:1px solid #ddd;padding:6px">${escapeHtml(v ?? '-')}</td></tr>`
  )
  .join('')}
<tr><td style="border:1px solid #ddd;padding:6px;background:#f7f9fc"><b>总分</b></td><td style="border:1px solid #ddd;padding:6px"><b style="color:#c00;font-size:16px">${total}${totalMax ? ` / ${totalMax}` : ''}</b></td></tr>
</table>
${dimHtml}
<p style="text-align:center;color:#999;margin-top:24px">生成时间：${new Date().toLocaleString('zh-CN')}</p>
</body></html>`
}

/** 新窗口打开打印 */
export function printHtml(html: string) {
  const w = window.open('', '_blank')
  if (!w) return
  w.document.write(html)
  w.document.close()
  setTimeout(() => w.print(), 300)
}
