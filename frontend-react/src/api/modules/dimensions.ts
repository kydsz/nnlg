import { request } from '../http'
import type { Dimension, DimensionGroup } from '../types'

export interface GroupPayload {
  code: string
  name: string
  sort_order?: number
  status?: number
}

/** 排序条目（后端 /dimensions/groups/sort 与 /dimensions/sort 均接收 [{id, sort_order}] 数组） */
export interface SortItem {
  id: number
  sort_order: number
}

export interface DimPayload {
  group_id?: number | null
  code: string
  name: string
  field_type: string
  field_config?: Record<string, unknown>
  description?: string
  sort_order?: number
  is_required?: boolean
  status?: number
}

export const dimensionApi = {
  groupList: (params?: { page?: number; page_size?: number; status?: number }) =>
    request<{ list: DimensionGroup[]; total: number }>({
      url: '/dimensions/groups',
      method: 'GET',
      params: { page: 1, page_size: 200, ...params },
    }),
  groupCreate: (data: GroupPayload) =>
    request<DimensionGroup>({ url: '/dimensions/groups', method: 'POST', data }),
  groupUpdate: (id: number, data: Partial<GroupPayload>) =>
    request<DimensionGroup>({ url: `/dimensions/groups/${id}`, method: 'PUT', data }),
  groupSort: (items: SortItem[]) =>
    request<null>({ url: '/dimensions/groups/sort', method: 'PUT', data: items }),
  groupDelete: (id: number) =>
    request<null>({ url: `/dimensions/groups/${id}`, method: 'DELETE' }),

  list: (params?: {
    page?: number
    page_size?: number
    group_id?: number
    field_type?: string
    status?: number
  }) =>
    request<{ list: Dimension[]; total: number }>({
      url: '/dimensions',
      method: 'GET',
      params: { page: 1, page_size: 500, ...params },
    }),

  active: () => request<Dimension[]>({ url: '/dimensions/active', method: 'GET' }),

  get: (id: number) => request<Dimension>({ url: `/dimensions/${id}`, method: 'GET' }),

  create: (data: DimPayload) => request<Dimension>({ url: '/dimensions', method: 'POST', data }),

  update: (id: number, data: Partial<DimPayload>) =>
    request<Dimension>({ url: `/dimensions/${id}`, method: 'PUT', data }),

  sort: (items: SortItem[]) =>
    request<null>({ url: '/dimensions/sort', method: 'PUT', data: items }),

  remove: (id: number) => request<null>({ url: `/dimensions/${id}`, method: 'DELETE' }),
}
