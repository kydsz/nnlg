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
        // 权限码来自后端 /auth/login、/auth/refresh、/auth/me 返回的显式清单。
        // 有清单（非空数组）时以清单为准：清单里没有就是没有，不再按角色放行。
        const perms = user.permissions
        if (Array.isArray(perms) && perms.length > 0) return perms.includes(perm)
        // 清单缺失/为空（旧版本 localStorage 会话、历史数据）时才按角色兜底，
        // 避免这些老会话在清单补齐前菜单被全部隐藏。
        const adminRoles = ['system_admin', 'school_admin', 'college_admin']
        return adminRoles.some((r) => user.roles?.includes(r))
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
