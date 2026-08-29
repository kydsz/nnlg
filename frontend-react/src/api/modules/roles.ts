import { request } from '../http'
import type { PermissionGroup, RoleInfo } from '../types'

export interface RolePayload {
  name: string
  code?: string
  description?: string
  level?: number
  permissions?: string[]
  data_scope?: 'all' | 'college' | 'self'
  status?: number
}

export const roleApi = {
  list: (params?: { page?: number; page_size?: number }) =>
    request<{ list: RoleInfo[]; total: number }>({
      url: '/roles',
      method: 'GET',
      params: { page: 1, page_size: 200, ...params },
    }),

  get: (id: number) => request<RoleInfo>({ url: `/roles/${id}`, method: 'GET' }),

  permissionGroups: () =>
    request<PermissionGroup[]>({ url: '/roles/permissions', method: 'GET' }),

  create: (data: RolePayload) => request<RoleInfo>({ url: '/roles', method: 'POST', data }),

  update: (id: number, data: RolePayload) =>
    request<RoleInfo>({ url: `/roles/${id}`, method: 'PUT', data }),

  remove: (id: number) => request<null>({ url: `/roles/${id}`, method: 'DELETE' }),
}
