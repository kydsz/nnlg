import { request, requestBlob } from '../http'
import type {
  CollegeStat,
  EvaluationRecord,
  OverviewStats,
  PageData,
  SemesterInfo,
  SupervisorStat,
  TeacherEvaluationSummary,
  TeacherStat,
} from '../types'

export interface RecordQuery {
  college_id?: number
  college_ids?: string
  teacher_id?: number
  evaluator_id?: number
  evaluator_role?: string
  evaluator_roles?: string
  has_courses?: boolean
  has_schedule?: boolean
  role?: string
  roles?: string
  start_date?: string
  end_date?: string
  keyword?: string
  semester?: string
  sort_by?: string
  sort_order?: string
  fields?: string[]
  page?: number
  page_size?: number
}

export const statsApi = {
  currentSemester: () =>
    request<SemesterInfo>({ url: '/stats/current-semester', method: 'GET' }),

  overview: () => request<OverviewStats>({ url: '/stats/overview', method: 'GET' }),

  teachers: (params?: RecordQuery) =>
    request<PageData<TeacherStat>>({ url: '/stats/teachers', method: 'GET', params }),

  colleges: (params?: RecordQuery & { page?: number; page_size?: number }) =>
    request<PageData<CollegeStat>>({ url: '/stats/colleges', method: 'GET', params }),

  campus: (params?: RecordQuery) =>
    request<unknown[]>({ url: '/stats/campus', method: 'GET', params }),

  supervisors: (params?: RecordQuery) =>
    request<PageData<SupervisorStat>>({ url: '/stats/supervisors', method: 'GET', params }),

  evaluators: (params?: RecordQuery) =>
    request<PageData<Record<string, unknown>>>({ url: '/stats/evaluators', method: 'GET', params }),

  unteachedTeachers: (params?: RecordQuery) =>
    request<PageData<Record<string, unknown>>>({
      url: '/stats/unteached-teachers',
      method: 'GET',
      params,
    }),

  evaluationRecords: (params?: RecordQuery) =>
    request<PageData<EvaluationRecord>>({
      url: '/stats/evaluation-records',
      method: 'GET',
      params,
    }),

  teacherSummary: (params?: RecordQuery) =>
    request<PageData<TeacherEvaluationSummary>>({
      url: '/stats/teacher-evaluation-summary',
      method: 'GET',
      params,
    }),

  // 导出
  exportTeachers: (params?: RecordQuery & { format?: string }) =>
    requestBlob({ url: '/stats/export/teachers', method: 'GET', params }),

  exportColleges: (params?: RecordQuery & { format?: string }) =>
    requestBlob({ url: '/stats/export/colleges', method: 'GET', params }),

  exportSupervisors: (params?: RecordQuery & { format?: string }) =>
    requestBlob({ url: '/stats/export/supervisors', method: 'GET', params }),

  exportTeacherSummary: (params?: RecordQuery & { format?: string }) =>
    requestBlob({ url: '/stats/export/teacher-evaluation-summary', method: 'GET', params }),

  exportEvaluationRecords: (params?: RecordQuery & { fields?: string[] }) =>
    requestBlob({ url: '/stats/evaluation-records/export', method: 'POST', data: params }),
}
