import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { TabBar } from 'antd-mobile'
import { AppOutline, CalendarOutline, UnorderedListOutline, UserOutline } from 'antd-mobile-icons'
import { useAuthStore } from '@/stores/auth'

const TABS = [
  { key: '/mobile/home', title: '首页', icon: <AppOutline /> },
  { key: '/mobile/evaluated', title: '已评', icon: <UnorderedListOutline /> },
  { key: '/mobile/schedule', title: '课表', icon: <CalendarOutline /> },
  { key: '/mobile/profile', title: '我的', icon: <UserOutline /> },
]

export default function MobileLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const mustChange = useAuthStore((s) => s.user?.must_change_password)

  // 初始密码未修改：后端只放行改密/登出/查看自己，其它页面请求必然 403，
  // 直接把人送到「我的」页并自动弹出改密弹窗（见 Profile），避免在各页面撞错误提示。
  useEffect(() => {
    if (mustChange && location.pathname !== '/mobile/profile') {
      navigate('/mobile/profile', { replace: true })
    }
  }, [mustChange, location.pathname, navigate])

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        minHeight: '100vh',
        background: '#f5f5f5',
      }}
    >
      <div style={{ flex: 1, paddingBottom: 60 }}>
        <Outlet />
      </div>
      <div
        style={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          borderTop: '1px solid #eee',
          background: '#fff',
          zIndex: 100,
        }}
      >
        <TabBar
          activeKey={location.pathname}
          onChange={(key) => navigate(key)}
          safeArea
        >
          {TABS.map((t) => (
            <TabBar.Item key={t.key} icon={t.icon} title={t.title} />
          ))}
        </TabBar>
      </div>
    </div>
  )
}
