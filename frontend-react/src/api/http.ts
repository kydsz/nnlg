import axios, { AxiosError, type AxiosRequestConfig } from 'axios'
import type { ApiResponse } from './types'

export class ApiError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.code = code
  }
}

type UnauthorizedHandler = () => void

let onUnauthorized: UnauthorizedHandler | null = null

/** 注册 401 统一处理（在 main.tsx 中绑定 zustand logout + 跳转） */
export function setUnauthorizedHandler(fn: UnauthorizedHandler) {
  onUnauthorized = fn
}

const instance = axios.create({
  baseURL: '/api/v1',
  timeout: 60000,
})

instance.interceptors.request.use((config) => {
  const token = localStorage.getItem('te-auth')
  if (token) {
    try {
      const parsed = JSON.parse(token) as { state?: { token?: string } }
      const t = parsed?.state?.token
      // 仅在拿到真实 JWT 时附加 Bearer；旧后端走 HttpOnly Cookie（自动携带）
      if (t && t.split('.').length === 3) {
        config.headers.Authorization = `Bearer ${t}`
      }
    } catch {
      /* ignore */
    }
  }
  return config
})

instance.interceptors.response.use(
  (resp) => {
    // blob 直接返回
    if (resp.config.responseType === 'blob') return resp
    const body = resp.data as ApiResponse
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code !== 200) {
        return Promise.reject(new ApiError(body.code, body.message || '请求失败'))
      }
      // 统一拆包
      resp.data = body.data
    }
    return resp
  },
  (error: AxiosError<ApiResponse>) => {
    const status = error.response?.status
    if (status === 401) {
      onUnauthorized?.()
      return Promise.reject(new ApiError(401, '登录已过期，请重新登录'))
    }
    const body = error.response?.data
    const msg = body?.message || error.message || '网络错误'
    return Promise.reject(new ApiError(body?.code ?? status ?? 0, msg))
  }
)

/** 统一请求封装：返回拆包后的 data */
export async function request<T>(config: AxiosRequestConfig): Promise<T> {
  const resp = await instance.request<ApiResponse<T>>(config)
  return resp.data as T
}

/** blob 请求（导出下载） */
export async function requestBlob(config: AxiosRequestConfig): Promise<Blob> {
  const resp = await instance.request<Blob>({ ...config, responseType: 'blob' })
  return resp.data
}

export default instance
