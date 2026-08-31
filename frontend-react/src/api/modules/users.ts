import { request } from '../http'
import type { PageData, User, UserInfo } from '../types'

export interface UserListParams {
  page?: number
  page_size?: number
  keyword?: string
  role?: string
  college_id?: string | number
  research_room_id?: number
  status?: number
  order_by?: string
  order?: 'asc' | 'desc'
}

export interface UserPayload {
  user_no?: string
  username?: string
  password?: string
  role?: string
  roles?: string[]
  college_id?: number | null
  research_room_id?: number | null
  supervisor_college_ids?: number[]
  supervisor_research_room_ids?: number[]
  status?: number
}

export const userApi = {
  list: (params: UserListParams) =>
    request<PageData<User>>({ url: '/users', method: 'GET', params }),

  get: (id: number) => request<User>({ url: `/users/${id}`, method: 'GET' }),

  create: (data: UserPayload) => request<User>({ url: '/users', method: 'POST', data }),

  update: (id: number, data: UserPayload) =>
    request<User>({ url: `/users/${id}`, method: 'PUT', data }),

  remove: (id: number) => request<null>({ url: `/users/${id}`, method: 'DELETE' }),

  updateStatus: (id: number, status: number) =>
    request<null>({ url: `/users/${id}/status`, method: 'PUT', params: { status } }),

  batchStatus: (ids: number[], status: number) =>
    request<null>({ url: '/users/batch-status', method: 'POST', data: { ids, status } }),

  addRole: (id: number, role: string) =>
    request<null>({ url: `/users/${id}/roles/${role}`, method: 'POST' }),

  removeRole: (id: number, role: string) =>
    request<null>({ url: `/users/${id}/roles/${role}`, method: 'DELETE' }),

  getSupervisorScope: (id: number) =>
    request<{ supervisor_college_ids: number[]; supervisor_research_room_ids: number[] }>({
      url: `/users/${id}/supervisor-scope`,
      method: 'GET',
    }),

  updateSupervisorScope: (id: number, data: { college_ids: number[]; research_room_ids: number[] }) =>
    request<null>({ url: `/users/${id}/supervisor-scope`, method: 'PUT', data }),

  updateMyResearchRoom: (researchRoomId: number | null) =>
    request<{ research_room_id: number | null; research_room_name: string | null }>({
      url: '/users/me/research-room',
      method: 'PUT',
      data: { research_room_id: researchRoomId },
    }),
}
