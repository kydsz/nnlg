import { request, requestBlob } from '../http'
import type { EvaluationRecord, PageData } from '../types'

export interface EvaluationListParams {
  page?: number
  page_size?: number
  task_id?: number
  teacher_id?: number
  evaluator_id?: number
  evaluator_name?: string
  keyword?: string
  start_date?: string
  end_date?: string
  order_by?: string
  order?: 'asc' | 'desc'
}

export interface SubmitPayload {
  task_id: number
  dimension_values: Record<string, unknown>
  is_anonymous?: boolean
}

export interface UpdatePayload {
  dimension_values: Record<string, unknown>
  is_anonymous?: boolean
}

export const evaluationApi = {
  list: (params: EvaluationListParams) =>
    request<PageData<EvaluationRecord>>({ url: '/evaluations', method: 'GET', params }),

  submit: (data: SubmitPayload) =>
    request<EvaluationRecord>({ url: '/evaluations', method: 'POST', data }),

  /** 与后端 /evaluations/with-files 约定对齐：task_id + dimension_values(JSON) +
   *  is_anonymous 为表单字段，文件统一放 files 且文件名带 {dim_code}_ 前缀 */
  submitWithFiles: (data: SubmitPayload & { files?: Record<string, File[]> }) => {
    const fd = new FormData()
    fd.append('task_id', String(data.task_id))
    fd.append(
      'dimension_values',
      JSON.stringify(data.dimension_values ?? Object.assign({}, data))
    )
    fd.append('is_anonymous', data.is_anonymous ? 'true' : 'false')
    Object.entries(data.files || {}).forEach(([dim, list]) => {
      list.forEach((f) => fd.append('files', new File([f], `${dim}_${f.name}`, { type: f.type })))
    })
    return request<EvaluationRecord>({
      url: '/evaluations/with-files',
      method: 'POST',
      data: fd,
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  detail: (id: number) =>
    request<EvaluationRecord>({ url: `/evaluations/${id}`, method: 'GET' }),

  exportRecord: (id: number, format?: string) =>
    requestBlob({ url: `/evaluations/${id}/export`, method: 'GET', params: { format } }),

  remove: (id: number) => request<null>({ url: `/evaluations/${id}`, method: 'DELETE' }),

  update: (id: number, data: UpdatePayload) =>
    request<EvaluationRecord>({ url: `/evaluations/${id}`, method: 'PUT', data }),
}
