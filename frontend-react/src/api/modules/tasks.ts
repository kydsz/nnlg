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

export const taskApi = {
  list: (params: TaskListParams) =>
    request<PageData<Task>>({ url: '/tasks', method: 'GET', params }),

  get: (id: number) => request<Task>({ url: `/tasks/${id}`, method: 'GET' }),

  create: (data: TaskPayload) => request<Task>({ url: '/tasks', method: 'POST', data }),

  batchCreate: (data: TaskPayload[]) =>
    request<{ created: number; failed: number }>({ url: '/tasks/batch', method: 'POST', data }),

  update: (id: number, data: Partial<TaskPayload>) =>
    request<Task>({ url: `/tasks/${id}`, method: 'PUT', data }),

  cancel: (id: number) => request<null>({ url: `/tasks/${id}/cancel`, method: 'POST' }),

  remove: (id: number) => request<null>({ url: `/tasks/${id}`, method: 'DELETE' }),

  export: (params: TaskListParams & { format?: string }) =>
    requestBlob({ url: '/tasks/export', method: 'POST', data: params }),
}
