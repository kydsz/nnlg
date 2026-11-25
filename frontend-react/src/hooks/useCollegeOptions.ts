import { useQuery } from '@tanstack/react-query'
import { orgApi } from '@/api/modules/org'
import { useAuthStore } from '@/stores/auth'

/**
 * 学院下拉选项：有 org:view 时取全量学院；
 * 无该权限的账号（如内置学院管理员）回退为本人所属学院，避免请求 403 且下拉为空。
 */
export function useCollegeOptions() {
  const hasOrgView = useAuthStore((s) => s.hasPermission('org:view'))
  const user = useAuthStore((s) => s.user)
  const { data } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
    enabled: hasOrgView,
  })
  if (hasOrgView) {
    return (data?.list || []).map((c) => ({ label: c.name, value: c.id }))
  }
  return user?.college_id != null
    ? [{ label: user.college_name || '本学院', value: user.college_id }]
    : []
}
