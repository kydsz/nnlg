import { request, requestBlob } from '../http'
import type { PageData, Task } from '../types'

export interface TaskListParams {
  page?: number
  page_size?: number
  keyword?: string
  status?: number
  teacher_id?: number
  has_supervisor_eval?: boolean
  create_by?: number
  create_by_not?: number
}

export interface TaskPayload {
  course_name: string
  teacher_id: number
  classroom?: string
  class_time?: string
}

/** /tasks/batch 响应（对齐后端：created 为任务对象数组，created_count 为成功数） */
export interface BatchCreateResult {
  created: unknown[]
  skipped: { teacher_id?: number; course_name?: string; reason?: string }[]
  created_count: number
  skipped_count: number
}

export const taskApi = {
  list: (params: TaskListParams) =>
    request<PageData<Task>>({ url: '/tasks', method: 'GET', params }),

  get: (id: number) => request<Task>({ url: `/tasks/${id}`, method: 'GET' }),

  create: (data: TaskPayload) => request<Task>({ url: '/tasks', method: 'POST', data }),

  batchCreate: (data: TaskPayload[]) =>
    request<BatchCreateResult>({ url: '/tasks/batch', method: 'POST', data: { tasks: data } }),

  update: (id: number, data: Partial<TaskPayload>) =>
    request<Task>({ url: `/tasks/${id}`, method: 'PUT', data }),

  cancel: (id: number) => request<null>({ url: `/tasks/${id}/cancel`, method: 'POST' }),

  remove: (id: number) => request<null>({ url: `/tasks/${id}`, method: 'DELETE' }),

  export: (params: TaskListParams & { format?: string }) =>
    requestBlob({ url: '/tasks/export', method: 'POST', data: params }),
}
