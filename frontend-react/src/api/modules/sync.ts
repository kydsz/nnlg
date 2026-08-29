import { request } from '../http'
import type { SyncResult } from '../types'

export const syncApi = {
  // 状态查询
  teachersSyncStatus: () =>
    request<Record<string, unknown>>({ url: '/teachers/sync-status', method: 'GET' }),

  // 同步操作
  syncAll: (semester?: string) =>
    request<SyncResult>({ url: '/sync/all', method: 'POST', params: { semester } }),

  syncUnitsAndTeachers: () =>
    request<SyncResult>({ url: '/sync/units-and-teachers', method: 'POST' }),

  syncColleges: () =>
    request<SyncResult>({ url: '/colleges/sync-from-jwxt', method: 'POST' }),

  syncTeachers: () =>
    request<SyncResult>({ url: '/teachers/sync-from-jwxt', method: 'POST' }),

  syncCourseSchedule: (semester?: string) =>
    request<SyncResult>({
      url: '/sync/course-schedule',
      method: 'POST',
      params: { semester },
    }),

  // llsykb：按工号同步所选教师课表
  syncLlsykb: (xnxq01id: string, teacherIds: string[]) =>
    request<SyncResult>({
      url: '/sync/llsykb',
      method: 'POST',
      data: { xnxq01id, teacher_ids: teacherIds },
    }),

  llsykbPreview: (data: Record<string, unknown>) =>
    request<unknown>({ url: '/sync/llsykb/preview', method: 'POST', data }),

  // 批量同步（后台任务）：{ xnxq01id, college_id? } → task_id
  llsykbBatch: (xnxq01id: string, collegeId?: number) =>
    request<{ task_id: string } & Record<string, unknown>>({
      url: '/sync/llsykb/batch',
      method: 'POST',
      data: { xnxq01id, college_id: collegeId },
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

  // 课表抓取
  crawlCourseSchedule: (data: {
    username: string
    password: string
    semester: string
    college_code?: string
    teacher_name?: string
  }) => request<SyncResult>({ url: '/course-schedules/crawl', method: 'POST', data }),
}
