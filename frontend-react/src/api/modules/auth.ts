import { request } from '../http'
import type { LoginResult, UserInfo } from '../types'

export const authApi = {
  login: (data: { user_no: string; password: string }) =>
    request<LoginResult>({ url: '/auth/login', method: 'POST', data }),

  logout: () => request<null>({ url: '/auth/logout', method: 'POST' }),

  refresh: () => request<LoginResult>({ url: '/auth/refresh', method: 'POST' }),

  me: () => request<UserInfo>({ url: '/auth/me', method: 'GET' }),

  changePassword: (data: { old_password: string; new_password: string }) =>
    request<null>({ url: '/auth/password', method: 'POST', data }),
}
