import { request } from '../http'
import type { Campus, College, ResearchRoom } from '../types'

export interface CampusPayload {
  name: string
  sort_order?: number
  status?: number
}

export interface CollegePayload {
  name: string
  code: string
  campus_id?: number | null
  sort_order?: number
  status?: number
}

export interface RoomPayload {
  name: string
  college_id: number
  status?: number
}

export const orgApi = {
  // 校区
  campusList: (params?: { page?: number; page_size?: number; status?: number }) =>
    request<{ list: Campus[]; total: number }>({
      url: '/campuses',
      method: 'GET',
      params: { page: 1, page_size: 200, ...params },
    }),
  campusCreate: (data: CampusPayload) => request<Campus>({ url: '/campuses', method: 'POST', data }),
  campusUpdate: (id: number, data: Partial<CampusPayload>) =>
    request<Campus>({ url: `/campuses/${id}`, method: 'PUT', data }),
  campusDelete: (id: number) => request<null>({ url: `/campuses/${id}`, method: 'DELETE' }),

  // 学院
  collegeList: (params?: { page?: number; page_size?: number; campus_id?: number; status?: number }) =>
    request<{ list: College[]; total: number }>({
      url: '/colleges',
      method: 'GET',
      params: { page: 1, page_size: 200, ...params },
    }),
  collegeCreate: (data: CollegePayload) =>
    request<College>({ url: '/colleges', method: 'POST', data }),
  collegeUpdate: (id: number, data: Partial<CollegePayload>) =>
    request<College>({ url: `/colleges/${id}`, method: 'PUT', data }),
  collegeDelete: (id: number) => request<null>({ url: `/colleges/${id}`, method: 'DELETE' }),

  // 教研室
  roomList: (params?: { page?: number; page_size?: number; college_id?: number; status?: number }) =>
    request<{ list: ResearchRoom[]; total: number }>({
      url: '/research-rooms',
      method: 'GET',
      params: { page: 1, page_size: 500, ...params },
    }),
  roomCreate: (data: RoomPayload) =>
    request<ResearchRoom>({ url: '/research-rooms', method: 'POST', data }),
  roomUpdate: (id: number, data: Partial<RoomPayload>) =>
    request<ResearchRoom>({ url: `/research-rooms/${id}`, method: 'PUT', data }),
  roomDelete: (id: number) => request<null>({ url: `/research-rooms/${id}`, method: 'DELETE' }),
}
