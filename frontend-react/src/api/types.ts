// 通用响应与分页类型
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface PageData<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

export interface PageParams {
  page?: number
  page_size?: number
}

// ===== 认证 =====
export interface UserInfo {
  id: number
  user_no: string
  username: string
  role: string
  role_name?: string
  roles: string[]
  user_roles?: { role: string; role_name: string; assign_time?: string }[]
  college_id: number | null
  college_name: string | null
  research_room_id: number | null
  research_room_name: string | null
  supervisor_college_count?: number
  supervisor_research_room_count?: number
  supervisor_colleges?: unknown[]
  status: number
  must_change_password?: boolean
  last_login_time: string | null
  create_time?: string
  update_time?: string
}

export interface LoginResult {
  access_token: string
  token_type: string
  user: UserInfo
}

// ===== 组织架构 =====
export interface Campus {
  id: number
  name: string
  sort_order: number
  status: number
  college_count?: number
  create_time?: string
}

export interface College {
  id: number
  code: string
  name: string
  campus_id: number | null
  campus_name?: string | null
  sort_order: number
  status: number
  research_room_count?: number
  user_count?: number
  create_time?: string
}

export interface ResearchRoom {
  id: number
  name: string
  college_id: number
  college_name?: string | null
  status: number
  user_count?: number
  create_time?: string
}

// ===== 用户 =====
export interface User extends UserInfo {
  password?: string
}

export interface RoleInfo {
  id: number
  name: string
  code: string
  description?: string
  level: number
  permissions: string[]
  data_scope: 'all' | 'college' | 'self'
  is_system: number
  status: number
  user_count?: number
}

export interface PermissionGroup {
  group: string
  permissions: { code: string; name: string }[]
}

// ===== 评教维度 =====
export interface DimensionGroup {
  id: number
  code: string
  name: string
  sort_order: number
  status: number
  dimension_count?: number
}

export type FieldType =
  | 'score'
  | 'single_choice'
  | 'multiple_choice'
  | 'text'
  | 'number'
  | 'date'
  | 'datetime'
  | 'rich_text'
  | 'image'
  | 'file'

export interface Dimension {
  id: number
  group_id: number | null
  group_name?: string | null
  code: string
  name: string
  field_type: FieldType
  field_type_name?: string
  field_config: Record<string, unknown>
  description?: string
  sort_order: number
  is_required: boolean
  status: number
}

// ===== 评教任务 =====
export interface Task {
  id: number
  teacher_id: number
  teacher_name: string
  teacher_college_id?: number | null
  teacher_college_name?: string | null
  course_name: string
  class_time: string | null
  classroom: string | null
  status: number
  status_name?: string
  evaluation_count?: number
  has_supervisor_eval?: boolean
  create_by?: number
  create_by_name?: string
  create_time?: string
}

// ===== 评教记录 =====
export interface EvaluationRecord {
  id: number
  task_id: number
  teacher_id: number
  teacher_name: string
  course_name: string
  class_time?: string | null
  classroom?: string | null
  college_name?: string | null
  evaluator_id: number
  evaluator_name: string
  evaluator_role?: string
  evaluator_role_name?: string
  dimension_values: Record<string, unknown>
  /** 新 Go 后端：维度 schema（用于前端组装分组明细） */
  dimension_groups?: DimensionSchemaGroup[]
  /** 旧 Python 后端：已组装的维度明细 */
  dimension_details?: EvaluationDimDetail[]
  total_score?: number
  max_total_score?: number
  is_anonymous: boolean
  listening_content?: string
  attendance_rate?: string
  submit_time: string
  files?: EvaluationFile[]
}

/** 新 Go 后端 /evaluations/:id 返回的维度 schema 组 */
export interface DimensionSchemaGroup {
  id?: number
  name?: string
  group_name?: string
  group_code?: string
  max_score?: number
  dimensions?: {
    code: string
    name: string
    field_type?: string
    group_name?: string
    [key: string]: unknown
  }[]
}

/** 旧 Python 后端 dimension_details 项 */
export interface EvaluationDimDetail {
  code?: string
  name?: string
  group_code?: string
  group_name?: string
  group_id?: number
  field_type?: string
  value?: unknown
  display_value?: string
  score?: number
  max_score?: number
}

export interface EvaluationFile {
  filename: string
  url: string
  dim_code?: string
}

// ===== 课表 =====
export interface SemesterConfig {
  id: number
  semester: string
  start_date: string
  is_current: boolean
}

export interface CourseItem {
  id?: number
  schedule_id?: number
  course_name: string
  teacher_id?: number
  teacher_name?: string
  week_day: number | string
  section: string
  classroom?: string
  week_pattern?: string
  class_info?: string
  student_count?: number | string | null
  college_name?: string
}

/** /course-schedules/teacher/:id 与 /my-schedule 的响应（对齐后端 scheduleDetailPayload） */
export interface TeacherSchedule {
  id: number
  teacher_id: number
  teacher_name: string
  semester: string
  version: number
  student_count: number | null
  is_current: boolean
  crawl_time: string
  change_summary: string | null
  details: CourseItem[]
}

// ===== 统计 =====
export interface OverviewStats {
  users: { total: number; teachers: number; supervisors: number }
  organization: { campuses: number; colleges: number }
  tasks: { total: number; evaluated: number; pending: number; evaluation_rate: number }
  evaluations: { total: number }
}

export interface TeacherStat {
  teacher_id: number
  teacher_name: string
  college_name?: string | null
  total_tasks: number
  evaluated_tasks: number
  pending_tasks: number
  total_evaluations: number
  average_score?: number
  evaluation_rate?: number
}

export interface CollegeStat {
  college_id: number
  college_name: string
  teacher_count: number
  total_tasks: number
  evaluated_tasks: number
  pending_tasks: number
  total_evaluations: number
  evaluation_rate?: number
}

export interface SupervisorStat {
  supervisor_id: number
  supervisor_name: string
  user_no?: string
  college_name?: string | null
  total_evaluations: number
  average_score?: number
}

export interface TeacherEvaluationSummary {
  teacher_id: number
  teacher_name: string
  user_no?: string
  college_name?: string | null
  role_names?: string[]
  given_count?: number
  given_avg_score?: number
  received_count?: number
  received_avg_score?: number
  task_count?: number
  evaluation_rate?: number
  has_schedule?: boolean
}

export interface SyncResult {
  total?: number
  new?: number
  updated?: number
  colleges_added?: number
  rooms_added?: number
  errors?: number
  semester?: string
  version?: number
  stats?: Record<string, unknown>
  [key: string]: unknown
}
