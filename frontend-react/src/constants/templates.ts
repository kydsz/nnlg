/**
 * 评价模板元数据唯一来源。
 * 任务/维度等页面统一从这里取标签、颜色与提示，避免文案再次散落。
 */

/** 任务可选模板编码（通用 common 只用于维度分组，不属于任务模板） */
export type TemplateCode = 'theory' | 'practice'

/** 含通用分组的模板键（维度分组表用） */
export type TemplateKey = TemplateCode | 'common'

export interface TemplateMeta {
  label: string
  /** antd Tag 预设色；通用模板不设色，渲染时不传 color */
  color?: string
  /** 任务创建弹窗的动态提示文案 */
  hint?: string
}

export const TEMPLATE_INFO: Record<TemplateKey, TemplateMeta> = {
  theory: {
    label: '理论课',
    color: 'blue',
    hint: '将使用理论课听评课评价表',
  },
  practice: {
    label: '实践教学',
    color: 'orange',
    hint: '将使用实践教学评价表（含迟到人数等字段）',
  },
  common: {
    label: '通用',
  },
}

/** 归一化为理论课/实践教学：未知、空值一律回落到理论课，与后端 model.NormalizeTemplate 口径一致 */
export function templateInfo(code?: string | null): TemplateMeta {
  return code === 'practice' ? TEMPLATE_INFO.practice : TEMPLATE_INFO.theory
}

/** 模板展示名（理论课 / 实践教学），未知或空值回落「理论课」 */
export function templateLabel(code?: string | null): string {
  return templateInfo(code).label
}

export interface TemplateOption {
  label: string
  value: string
}

/** 任务创建/筛选下拉：仅理论课与实践教学 */
export const TEMPLATE_OPTIONS: TemplateOption[] = [
  { label: TEMPLATE_INFO.theory.label, value: 'theory' },
  { label: TEMPLATE_INFO.practice.label, value: 'practice' },
]

/** 带「全部模板」的筛选下拉（首项 value 为空字符串） */
export const TEMPLATE_FILTER_OPTIONS: TemplateOption[] = [
  { label: '全部模板', value: '' },
  ...TEMPLATE_OPTIONS,
]

/** 管理端维度分组表单/筛选：额外包含通用分组 */
export const GROUP_TEMPLATE_OPTIONS: TemplateOption[] = [
  ...TEMPLATE_OPTIONS,
  { label: TEMPLATE_INFO.common.label, value: 'common' },
]
