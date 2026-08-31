import { request } from '../http'
import type { SyncResult } from '../types'

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
    request<SyncResult>({ url: '/sync/all', method: 'POST', params: { semester } }),

  syncUnitsAndTeachers: () =>
    request<SyncResult>({ url: '/sync/units-and-teachers', method: 'POST' }),

  syncColleges: () =>
    request<SyncResult>({ url: '/colleges/sync-from-jwxt', method: 'POST' }),

  syncTeachers: () =>
    request<SyncResult>({ url: '/teachers/sync-from-jwxt', method: 'POST' }),

  // 旧方式：整页抓取，易漏没课老师、无进度，建议改用 llsykbBatch
  syncCourseSchedule: (semester?: string) =>
    request<SyncResult>({
      url: '/sync/course-schedule',
      method: 'POST',
      params: { semester },
    }),

  // 旧方式（同步执行）：已改由 llsykbBatch 异步后台执行，此处仅保留兼容
  syncLlsykb: (xnxq01id: string, teacherIds: string[]) =>
    request<SyncResult>({
      url: '/sync/llsykb',
      method: 'POST',
      data: { xnxq01id, teacher_ids: teacherIds },
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
  }) => request<SyncResult>({ url: '/course-schedules/crawl', method: 'POST', data }),
}
