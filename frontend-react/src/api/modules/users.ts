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

export interface TeachingCollegeRow {
  user_id: number
  user_no: string
  username: string
  teacher_college_id: number | null
  teacher_college_name: string
  college_id: number
  college_name: string
  source: string
  join_time?: string | null
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

  // ---- 授课学院管理 ----
  listTeachingColleges: (params?: {
    page?: number
    page_size?: number
    keyword?: string
    college_id?: number
  }) => request<PageData<TeachingCollegeRow>>({ url: '/teaching-colleges', method: 'GET', params }),

  teachingCollegeCandidates: (keyword?: string) =>
    request<{ list: User[] }>({
      url: '/teaching-colleges/candidates',
      method: 'GET',
      params: { keyword: keyword || undefined },
    }),

  addTeachingCollege: (userId: number, collegeId: number) =>
    request<null>({
      url: '/teaching-colleges',
      method: 'POST',
      data: { user_id: userId, college_id: collegeId },
    }),

  removeTeachingCollege: (userId: number, collegeId: number) =>
    request<null>({ url: `/teaching-colleges/${userId}/${collegeId}`, method: 'DELETE' }),
}
