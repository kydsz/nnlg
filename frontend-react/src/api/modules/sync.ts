import { request } from '../http'
import type { SyncResult } from '../types'

// 教务系统同步为长耗时请求（全量单位+教师约 1-2 分钟），默认 60s 超时会在后端已成功时
// 仍提示"失败"（后端不受前端断开影响，会继续跑完）。这里统一放宽到 10 分钟。
const SYNC_TIMEOUT = 600000

// syncApi 说明：
// - llsykbBatch / llsykbProgress —— 推荐：逐个老师拉取课表（可按学院/指定老师），覆盖没课老师，带进度条。
// - syncCourseSchedule / crawlCourseSchedule —— 旧方式：整页抓取，易漏没课老师、无进度，建议改用上面推荐接口。
export const syncApi = {
  // 状态查询
  teachersSyncStatus: () =>
    request<Record<string, unknown>>({ url: '/teachers/sync-status', method: 'GET' }),

  // 同步操作
  // sync/all：一次性同步单位 + 教师 + 课表；如需刷课表，建议改用 llsykbBatch
  syncAll: (semester?: string) =>
    request<SyncResult>({ url: '/sync/all', method: 'POST', params: { semester }, timeout: SYNC_TIMEOUT }),

  syncUnitsAndTeachers: () =>
    request<SyncResult>({ url: '/sync/units-and-teachers', method: 'POST', timeout: SYNC_TIMEOUT }),

  syncColleges: () =>
    request<SyncResult>({ url: '/colleges/sync-from-jwxt', method: 'POST', timeout: SYNC_TIMEOUT }),

  syncTeachers: () =>
    request<SyncResult>({ url: '/teachers/sync-from-jwxt', method: 'POST', timeout: SYNC_TIMEOUT }),

  // 旧方式：整页抓取，易漏没课老师、无进度，建议改用 llsykbBatch
  syncCourseSchedule: (semester?: string) =>
    request<SyncResult>({
      url: '/sync/course-schedule',
      method: 'POST',
      params: { semester },
      timeout: SYNC_TIMEOUT,
    }),

  // 旧方式（同步执行）：已改由 llsykbBatch 异步后台执行，此处仅保留兼容
  syncLlsykb: (xnxq01id: string, teacherIds: string[]) =>
    request<SyncResult>({
      url: '/sync/llsykb',
      method: 'POST',
      data: { xnxq01id, teacher_ids: teacherIds },
      timeout: SYNC_TIMEOUT,
    }),

  llsykbPreview: (data: Record<string, unknown>) =>
    request<unknown>({ url: '/sync/llsykb/preview', method: 'POST', data }),

  // 推荐：异步后台任务 + 进度条。teacher_nos 指定老师；college_id 按学院；都不传则同步全部
  llsykbBatch: (xnxq01id: string, collegeId?: number, teacherNos?: string[]) =>
    request<{ task_id: string } & Record<string, unknown>>({
      url: '/sync/llsykb/batch',
      method: 'POST',
      data: { xnxq01id, college_id: collegeId, teacher_nos: teacherNos },
    }),

  llsykbProgress: (taskId: string) =>
    request<Record<string, unknown>>({
      url: `/sync/llsykb/progress/${taskId}`,
      method: 'GET',
    }),

  scanInvalidUsers: () =>
    request<Record<string, unknown>>({ url: '/sync/scan-invalid-users', method: 'GET' }),

  cleanupInvalidUser: (userId: number | string) =>
    request<null>({ url: `/sync/cleanup-user/${userId}`, method: 'DELETE' }),

  // 旧方式：整页抓取，易漏没课老师、无进度，建议改用 llsykbBatch
  crawlCourseSchedule: (data: {
    username: string
    password: string
    semester: string
    college_code?: string
    teacher_name?: string
  }) => request<SyncResult>({ url: '/course-schedules/crawl', method: 'POST', data, timeout: SYNC_TIMEOUT }),
}
