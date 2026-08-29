import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import { isMobileDevice } from '@/hooks/useIsMobile'
import type { ReactNode } from 'react'

/** 需要登录 */
export function RequireAuth() {
  const token = useAuthStore((s) => s.token)
  const location = useLocation()
  if (!token) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />
  }
  return <Outlet />
}

/** 已登录访问 /login → 按角色回对应端首页（对齐旧版逻辑） */
export function RedirectIfAuthed({ children }: { children: ReactNode }) {
  const token = useAuthStore((s) => s.token)
  const user = useAuthStore((s) => s.user)
  if (token) {
    const roles = user?.roles || []
    const isAdmin = roles.some((r) =>
      ['system_admin', 'college_admin', 'school_admin'].includes(r)
    )
    // 桌面管理员进管理端；移动端管理员默认进移动端（可在“我的”切换管理端）
    if (isAdmin && !isMobileDevice()) return <Navigate to="/admin/dashboard" replace />
    return <Navigate to="/mobile/home" replace />
  }
  return <>{children}</>
}

/** 管理域：仅拦截非管理员角色，移动设备管理员也允许切换进入管理端 */
export function AdminGate() {
  const user = useAuthStore((s) => s.user)
  const roles = user?.roles || []
  const isAdminRole = roles.some((r) =>
    ['system_admin', 'college_admin', 'school_admin'].includes(r)
  )
  if (!isAdminRole) {
    return <Navigate to="/mobile/home" replace />
  }
  return <Outlet />
}

/** 按权限码渲染 */
export function PermissionGate({ perm, children }: { perm: string; children: ReactNode }) {
  const hasPermission = useAuthStore((s) => s.hasPermission)
  return hasPermission(perm) ? <>{children}</> : null
}
