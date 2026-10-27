import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router-dom'
import { ConfigProvider, App as AntApp } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import { router } from './routes'
import { setMustChangePasswordHandler, setUnauthorizedHandler } from './api/http'
import { authApi } from './api/modules/auth'
import { PWD_FORCE_CHANGE_DISMISS_KEY, useAuthStore } from './stores/auth'
import 'antd/dist/reset.css'
import './styles/global.css'

dayjs.locale('zh-cn')

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

// 401 统一处理：清登录态并回登录页
setUnauthorizedHandler(() => {
  useAuthStore.getState().logout()
  if (!window.location.pathname.startsWith('/login')) {
    window.location.href = '/login'
  }
})

// 强制改密统一处理：后端在初始密码未修改时只放行改密/登出/查看自己。
// 清掉「本次登录不再提醒」标记并通知布局层立即弹出改密弹窗。
setMustChangePasswordHandler(() => {
  localStorage.removeItem(PWD_FORCE_CHANGE_DISMISS_KEY)
  window.dispatchEvent(new Event('tev:must-change-password'))
})

// 启动时用 /auth/me 刷新当前用户：旧版本 localStorage 会话可能没有 permissions 权限清单，
// 且角色权限在服务端被修改后（不重新登录）也需要即时生效。仅在有 token 时发起；
// 失败静默——401 由上面的拦截器统一处理（过期会自动刷新一次），404/403 也不阻塞渲染。
if (useAuthStore.getState().token) {
  authApi
    .me()
    .then((user) => useAuthStore.getState().setUser(user))
    .catch(() => {
      /* 静默：保留本地登录态，交给后续请求的 401 处理 */
    })
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ConfigProvider
        locale={zhCN}
        theme={{ token: { colorPrimary: '#104186', borderRadius: 6 } }}
      >
        <AntApp>
          <RouterProvider router={router} />
        </AntApp>
      </ConfigProvider>
    </QueryClientProvider>
  </React.StrictMode>
)
