export const ROLE_NAMES: Record<string, string> = {
  system_admin: '系统管理员',
  school_admin: '学校管理员',
  college_admin: '学院管理员',
  school_supervisor: '校级督导',
  supervisor: '督导老师',
  college_supervisor: '院级督导',
  teacher: '教师',
}

export function roleName(code: string | null | undefined): string {
  if (!code) return '-'
  return ROLE_NAMES[code] || code
}

export function roleNamesStr(codes: string[] | null | undefined): string {
  if (!codes || codes.length === 0) return '-'
  return codes.map(roleName).join(' / ')
}
