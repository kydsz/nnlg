import { beforeEach, describe, expect, it } from 'vitest'
import type { UserInfo } from '@/api/types'
import { PWD_FORCE_CHANGE_DISMISS_KEY, useAuthStore } from '@/stores/auth'

function makeUser(overrides: Partial<UserInfo> = {}): UserInfo {
  return {
    id: 1,
    user_no: 'T001',
    username: '张老师',
    role: 'teacher',
    roles: ['teacher'],
    college_id: null,
    college_name: null,
    research_room_id: null,
    research_room_name: null,
    status: 1,
    last_login_time: null,
    ...overrides,
  }
}

type UserWithPerms = UserInfo & { permissions?: string[] }

beforeEach(() => {
  localStorage.clear()
  useAuthStore.setState({ token: null, user: null })
})

describe('store 基础操作', () => {
  it('setAuth 同时写入 token 与 user 并持久化', () => {
    useAuthStore.getState().setAuth('tk-123', makeUser())
    const { token, user } = useAuthStore.getState()
    expect(token).toBe('tk-123')
    expect(user?.username).toBe('张老师')

    const persisted = JSON.parse(localStorage.getItem('te-auth') || '{}')
    expect(persisted.state.token).toBe('tk-123')
    expect(persisted.state.user.username).toBe('张老师')
  })

  it('setUser 只更新用户信息', () => {
    useAuthStore.getState().setAuth('tk-123', makeUser())
    useAuthStore.getState().setUser(makeUser({ username: '李老师' }))
    expect(useAuthStore.getState().token).toBe('tk-123')
    expect(useAuthStore.getState().user?.username).toBe('李老师')
  })

  it('logout 清空登录态', () => {
    useAuthStore.getState().setAuth('tk-123', makeUser())
    useAuthStore.getState().logout()
    expect(useAuthStore.getState().token).toBeNull()
    expect(useAuthStore.getState().user).toBeNull()
  })

  it('刷新后从 localStorage 恢复登录态', async () => {
    localStorage.setItem(
      'te-auth',
      JSON.stringify({
        state: { token: 'tk-persist', user: makeUser({ username: '恢复用户' }) },
        version: 0,
      })
    )
    await useAuthStore.persist.rehydrate()
    expect(useAuthStore.getState().token).toBe('tk-persist')
    expect(useAuthStore.getState().user?.username).toBe('恢复用户')
  })

  it('强制改密弹窗的存储 key 固定', () => {
    expect(PWD_FORCE_CHANGE_DISMISS_KEY).toBe('te_pwd_force_change_dismiss')
  })
})

describe('hasPermission', () => {
  it('未登录返回 false', () => {
    expect(useAuthStore.getState().hasPermission('task:view')).toBe(false)
  })

  it('system_admin 拥有所有权限', () => {
    useAuthStore.getState().setAuth('tk', makeUser({ roles: ['system_admin'] }))
    expect(useAuthStore.getState().hasPermission('anything:anything')).toBe(true)
  })

  it('有显式权限列表时精确匹配', () => {
    const user: UserWithPerms = { ...makeUser(), permissions: ['task:view'] }
    useAuthStore.getState().setAuth('tk', user)
    expect(useAuthStore.getState().hasPermission('task:view')).toBe(true)
    expect(useAuthStore.getState().hasPermission('task:grade')).toBe(false)
  })

  it('无显式权限时管理员类角色放行', () => {
    for (const role of ['school_admin', 'college_admin']) {
      useAuthStore.getState().setAuth('tk', makeUser({ roles: [role] }))
      expect(useAuthStore.getState().hasPermission('college:view')).toBe(true)
    }
  })

  it('普通角色无权限时返回 false', () => {
    useAuthStore.getState().setAuth('tk', makeUser({ roles: ['teacher'] }))
    expect(useAuthStore.getState().hasPermission('college:view')).toBe(false)
    useAuthStore.getState().setAuth('tk', makeUser({ roles: ['supervisor'] }))
    expect(useAuthStore.getState().hasPermission('college:view')).toBe(false)
  })
})

describe('isAdmin', () => {
  it('未登录返回 false', () => {
    expect(useAuthStore.getState().isAdmin()).toBe(false)
  })

  it('三类管理员返回 true', () => {
    for (const role of ['system_admin', 'school_admin', 'college_admin']) {
      useAuthStore.getState().setAuth('tk', makeUser({ roles: [role] }))
      expect(useAuthStore.getState().isAdmin()).toBe(true)
    }
  })

  it('非管理员角色返回 false', () => {
    useAuthStore.getState().setAuth('tk', makeUser({ roles: ['teacher', 'supervisor'] }))
    expect(useAuthStore.getState().isAdmin()).toBe(false)
  })
})
