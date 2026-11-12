import { request, requestBlob } from '../http'
import type { PageData, Task, EvalScheduleInfo } from '../types'

export interface TaskListParams {
  page?: number
  page_size?: number
  keyword?: string
  status?: number
  teacher_id?: number
  has_supervisor_eval?: boolean
  create_by?: number
  create_by_not?: number
  /** 只看我加入的任务 */
  joined_by?: number
  start_date?: string
  end_date?: string
  order_by?: string
  order?: 'asc' | 'desc'
  /** 1=只看已删除任务（后端仅对持 task:restore/task:restore_own 权限者生效）；缺省/0=正常任务 */
  deleted?: 0 | 1
}

export interface TaskPayload {
  course_name: string
  teacher_id: number
  classroom?: string
  class_time?: string
  /** 加入待评时固化的课表信息快照 */
  schedule?: EvalScheduleInfo
}

/** /tasks/batch 响应（对齐后端：created 为任务对象数组，created_count 为成功数） */
export interface BatchCreateResult {
  created: unknown[]
  skipped: { teacher_id?: number; course_name?: string; reason?: string }[]
  created_count: number
  skipped_count: number
}

/** 加入任务响应 */
export interface JoinResult {
  task_id: number
  participant_count: number
  max_participants: number
  already_joined: boolean
  over_limit: boolean
  created: boolean
}

export const taskApi = {
  list: (params: TaskListParams) =>
    request<PageData<Task>>({ url: '/tasks', method: 'GET', params }),

  get: (id: number) => request<Task>({ url: `/tasks/${id}`, method: 'GET' }),

  create: (data: TaskPayload) => request<Task>({ url: '/tasks', method: 'POST', data }),

  /** 加入指定任务 */
  join: (id: number) => request<JoinResult>({ url: `/tasks/${id}/join`, method: 'POST' }),

  /** 课表加入待评任务：已有同课任务则加入，否则创建并加入 */
  joinOrCreate: (data: TaskPayload) =>
    request<JoinResult>({ url: '/tasks/join', method: 'POST', data }),

  batchCreate: (data: TaskPayload[]) =>
    request<BatchCreateResult>({ url: '/tasks/batch', method: 'POST', data: { tasks: data } }),

  update: (id: number, data: Partial<TaskPayload>) =>
    request<Task>({ url: `/tasks/${id}`, method: 'PUT', data }),


  remove: (id: number) => request<null>({ url: `/tasks/${id}`, method: 'DELETE' }),

  restore: (id: number) => request<null>({ url: `/tasks/${id}/restore`, method: 'POST' }),

  export: (params: TaskListParams & { format?: string }) =>
    requestBlob({ url: '/tasks/export', method: 'POST', data: params }),
}
