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
}

export interface SubmitPayload {
  task_id: number
  dimension_values: Record<string, unknown>
  is_anonymous?: boolean
}

export const evaluationApi = {
  list: (params: EvaluationListParams) =>
    request<PageData<EvaluationRecord>>({ url: '/evaluations', method: 'GET', params }),

  submit: (data: SubmitPayload) =>
    request<EvaluationRecord>({ url: '/evaluations', method: 'POST', data }),

  submitWithFiles: (data: SubmitPayload & { files?: Record<string, File[]> }) => {
    const fd = new FormData()
    fd.append('payload', JSON.stringify(data))
    Object.entries(data.files || {}).forEach(([dim, list]) => {
      list.forEach((f) => fd.append(`files_${dim}`, f))
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
}
