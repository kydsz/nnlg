import { request } from '../http'

export interface TaskSettings {
  task_max_participants: number
}

export const settingApi = {
  get: () => request<TaskSettings>({ url: '/settings', method: 'GET' }),

  update: (data: TaskSettings) => request<TaskSettings>({ url: '/settings', method: 'PUT', data }),
}
