import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { UserInfo } from '@/api/types'

// 强制改密弹窗"本次登录不再提醒"的存储 key，值为用户 id（区分用户）
export const PWD_FORCE_CHANGE_DISMISS_KEY = 'te_pwd_force_change_dismiss'

interface AuthState {
  token: string | null
  user: UserInfo | null
  setAuth: (token: string, user: UserInfo) => void
  setUser: (user: UserInfo) => void
  logout: () => void
  hasPermission: (perm: string) => boolean
  isAdmin: () => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      setAuth: (token, user) => set({ token, user }),
      setUser: (user) => set({ user }),
      logout: () => set({ token: null, user: null }),
      hasPermission: (perm) => {
        const user = get().user
        if (!user) return false
        // 系统管理员拥有所有权限
        if (user.roles?.includes('system_admin')) return true
        // 权限来自登录用户返回（后端 /auth/me 会带 permissions，如无则按角色估算）
        const perms = (user as UserInfo & { permissions?: string[] }).permissions || []
        if (perms.includes(perm)) return true
        // 无显式权限列表时，管理员类角色放行查看类权限
        const adminRoles = ['system_admin', 'school_admin', 'college_admin']
        if (adminRoles.some((r) => user.roles?.includes(r))) return true
        return false
      },
      isAdmin: () => {
        const user = get().user
        if (!user) return false
        return ['system_admin', 'school_admin', 'college_admin'].some((r) =>
          user.roles?.includes(r)
        )
      },
    }),
    { name: 'te-auth' }
  )
)
