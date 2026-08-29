import { request } from '../http'
import type { CourseItem, PageData, SemesterConfig, TeacherSchedule } from '../types'

export const scheduleApi = {
  list: (params?: {
    page?: number
    page_size?: number
    semester?: string
    teacher_id?: number
    teacher_name?: string
  }) => request<PageData<CourseItem>>({ url: '/course-schedules/list', method: 'GET', params }),

  detail: (id: number) =>
    request<TeacherSchedule>({ url: `/course-schedules/detail/${id}`, method: 'GET' }),

  byTeacher: (teacherId: number, semester?: string) =>
    request<TeacherSchedule>({
      url: `/course-schedules/teacher/${teacherId}`,
      method: 'GET',
      params: { semester },
    }),

  mySchedule: (semester?: string) =>
    request<TeacherSchedule>({
      url: '/course-schedules/my-schedule',
      method: 'GET',
      params: { semester },
    }),

  semesters: () => request<string[]>({ url: '/course-schedules/semesters', method: 'GET' }),

  versions: (semester: string) =>
    request<unknown[]>({
      url: '/course-schedules/versions',
      method: 'GET',
      params: { semester },
    }),

  teacherStatus: (params?: { semester?: string; college_id?: number; page?: number; page_size?: number }) =>
    request<PageData<Record<string, unknown>>>({
      url: '/course-schedules/teacher-status',
      method: 'GET',
      params,
    }),

  stats: (semester: string) =>
    request<Record<string, unknown>>({
      url: '/course-schedules/stats',
      method: 'GET',
      params: { semester },
    }),

  semesterConfigs: () =>
    request<SemesterConfig[]>({ url: '/course-schedules/semester-configs', method: 'GET' }),

  currentSemester: () =>
    request<SemesterConfig>({ url: '/course-schedules/semester-configs/current', method: 'GET' }),

  semesterConfig: (semester: string) =>
    request<SemesterConfig>({
      url: `/course-schedules/semester-configs/${semester}`,
      method: 'GET',
    }),

  createSemesterConfig: (data: { semester: string; start_date: string; is_current?: boolean }) =>
    request<SemesterConfig>({ url: '/course-schedules/semester-configs', method: 'POST', data }),

  updateSemesterConfig: (
    semester: string,
    data: { start_date?: string; is_current?: boolean }
  ) =>
    request<SemesterConfig>({
      url: `/course-schedules/semester-configs/${semester}`,
      method: 'PUT',
      data,
    }),

  deleteSemesterConfig: (semester: string) =>
    request<null>({
      url: `/course-schedules/semester-configs/${semester}`,
      method: 'DELETE',
    }),
}
