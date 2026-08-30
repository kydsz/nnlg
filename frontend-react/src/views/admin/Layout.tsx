import { useEffect, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { App, Checkbox, Dropdown, Modal, Form, Input } from 'antd'
import {
  DashboardOutlined,
  BankOutlined,
  TeamOutlined,
  SafetyOutlined,
  TagsOutlined,
  FileTextOutlined,
  BarChartOutlined,
  SyncOutlined,
  CalendarOutlined,
  TableOutlined,
  ApartmentOutlined,
  ExperimentOutlined,
  MobileOutlined,
  KeyOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons'
import { useAuthStore, PWD_FORCE_CHANGE_DISMISS_KEY } from '@/stores/auth'
import { authApi } from '@/api/modules/auth'
import './layout.css'

interface MenuItem {
  key: string
  icon: React.ReactNode
  label: string
  perm?: string
}

const MENU: MenuItem[] = [
  { key: '/admin/dashboard', icon: <DashboardOutlined />, label: '概览', perm: 'stats:view' },
  { key: '/admin/campus', icon: <BankOutlined />, label: '校区管理', perm: 'campus:manage' },
  { key: '/admin/colleges', icon: <ApartmentOutlined />, label: '学院管理', perm: 'college:manage' },
  {
    key: '/admin/research-rooms',
    icon: <ExperimentOutlined />,
    label: '教研室',
    perm: 'research_room:manage',
  },
  { key: '/admin/users', icon: <TeamOutlined />, label: '用户管理', perm: 'user:view' },
  { key: '/admin/roles', icon: <SafetyOutlined />, label: '角色管理', perm: 'role:manage' },
  {
    key: '/admin/course-schedule',
    icon: <CalendarOutlined />,
    label: '学期配置与课表查询',
    perm: 'schedule:view',
  },
  { key: '/admin/data-sync', icon: <SyncOutlined />, label: '数据同步', perm: 'sync:execute' },
  { key: '/admin/dimensions', icon: <TagsOutlined />, label: '评教维度', perm: 'dimension:manage' },
  { key: '/admin/tasks', icon: <FileTextOutlined />, label: '评教任务', perm: 'task:view' },
  {
    key: '/admin/evaluations',
    icon: <TableOutlined />,
    label: '评教记录',
    perm: 'evaluation:view',
  },
  { key: '/admin/stats', icon: <BarChartOutlined />, label: '统计报表', perm: 'stats:view' },
  {
    key: '/admin/teacher-evaluation-summary',
    icon: <BarChartOutlined />,
    label: '教师评教汇总',
    perm: 'stats:view',
  },
]

export default function AdminLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { message, modal } = App.useApp()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const hasPermission = useAuthStore((s) => s.hasPermission)

  const [pwdOpen, setPwdOpen] = useState(false)
  const [pwdLoading, setPwdLoading] = useState(false)
  const [pwdDismiss, setPwdDismiss] = useState(false)
  const [pwdForm] = Form.useForm()

  // 首次登录强制修改密码：用户勾选"本次登录不再提醒"后将跳过本次登录（刷新页面也不弹），下次登录重新提醒
  useEffect(() => {
    if (user?.must_change_password) {
      const dismissed = localStorage.getItem(PWD_FORCE_CHANGE_DISMISS_KEY)
      setPwdOpen(user.id === undefined || dismissed !== String(user.id))
    }
  }, [user?.must_change_password])

  const items = MENU.filter((m) => !m.perm || hasPermission(m.perm))
  const current = items.find((m) => location.pathname.startsWith(m.key))

  const handleLogout = () => {
    modal.confirm({
      title: '确认退出',
      content: '确定要退出登录吗？',
      okText: '退出',
      okButtonProps: { danger: true },
      onOk: () => {
        authApi.logout().catch(() => {})
        logout()
        navigate('/login', { replace: true })
      },
    })
  }

  const onPasswordChange = async () => {
    try {
      const values = await pwdForm.validateFields()
      setPwdLoading(true)
      await authApi.changePassword({
        old_password: values.old_password,
        new_password: values.new_password,
      })
      message.success('密码修改成功')
      localStorage.removeItem(PWD_FORCE_CHANGE_DISMISS_KEY)
      setPwdOpen(false)
      setPwdDismiss(false)
      pwdForm.resetFields()
    } catch {
      // 校验或请求失败，保持弹窗
    } finally {
      setPwdLoading(false)
    }
  }

  return (
    <div className="admin-layout">
      <aside className={`sidebar ${collapsed ? 'collapsed' : ''}`}>
        <div className="sidebar-logo">
          <img src="/nnlg_logo_wide.png" alt="" className="logo-img" />
        </div>

        <nav className="sidebar-nav">
          {items.map((m) => (
            <a
              key={m.key}
              className={`nav-item ${location.pathname.startsWith(m.key) ? 'active' : ''}`}
              onClick={() => navigate(m.key)}
              title={collapsed ? m.label : ''}
            >
              <span className="nav-icon">{m.icon}</span>
              {!collapsed && <span className="nav-label">{m.label}</span>}
            </a>
          ))}
        </nav>

        <div className="sidebar-bottom">
          <a className="bottom-link" onClick={() => navigate('/mobile/home')}>
            <MobileOutlined className="bottom-icon" />
            {!collapsed && <span className="bottom-text">移动端</span>}
          </a>
        </div>
      </aside>

      <div className={`main-wrapper ${collapsed ? 'sidebar-collapsed' : ''}`}>
        <header className="top-header">
          <div className="header-left">
            <button
              className="icon-btn"
              onClick={() => setCollapsed(!collapsed)}
              title={collapsed ? '展开侧栏' : '收起侧栏'}
            >
              {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            </button>
            <span className="page-title">{current?.label || '管理后台'}</span>
          </div>
          <div className="header-right">
            <Dropdown
              trigger={['click']}
              menu={{
                items: [
                  { key: 'mobile', icon: <MobileOutlined />, label: '切换移动端' },
                  { key: 'pwd', icon: <KeyOutlined />, label: '修改密码' },
                  { type: 'divider' },
                  {
                    key: 'logout',
                    icon: <LogoutOutlined />,
                    label: '退出登录',
                    danger: true,
                    onClick: handleLogout,
                  },
                ],
                onClick: ({ key }) => {
                  if (key === 'mobile') navigate('/mobile/home')
                  if (key === 'pwd') setPwdOpen(true)
                },
              }}
            >
              <button className="user-trigger">
                <span className="avatar">{(user?.username || 'U').charAt(0)}</span>
                <span className="user-name">{user?.username}</span>
              </button>
            </Dropdown>
          </div>
        </header>

        <main className="content">
          <Outlet />
        </main>
      </div>

      <Modal
        title="修改密码"
        open={pwdOpen}
        onCancel={() => {
          // 勾选"本次登录不再提醒"后关闭，仅跳过本次登录的强制提示
          if (pwdDismiss && user && user.id !== undefined) {
            localStorage.setItem(PWD_FORCE_CHANGE_DISMISS_KEY, String(user.id))
          }
          setPwdOpen(false)
        }}
        onOk={onPasswordChange}
        confirmLoading={pwdLoading}
        okText="保存"
        cancelText="暂不修改"
        maskClosable={false}
      >
        <Form form={pwdForm} layout="vertical" autoComplete="new-password">
          <Form.Item
            name="old_password"
            label="旧密码"
            rules={[{ required: true, message: '请输入旧密码' }]}
          >
            <Input.Password placeholder="请输入旧密码" />
          </Form.Item>
          <Form.Item
            name="new_password"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '密码至少8位' },
              {
                validator: (_, v) =>
                  !v || (/[a-zA-Z]/.test(v) && /[0-9]/.test(v))
                    ? Promise.resolve()
                    : Promise.reject(new Error('需包含字母和数字')),
              },
            ]}
          >
            <Input.Password placeholder="至少8位，含字母和数字" />
          </Form.Item>
          <Form.Item
            name="confirm_password"
            label="确认密码"
            dependencies={['new_password']}
            rules={[
              { required: true, message: '请确认新密码' },
              ({ getFieldValue }) => ({
                validator: (_, v) =>
                  !v || v === getFieldValue('new_password')
                    ? Promise.resolve()
                    : Promise.reject(new Error('两次密码不一致')),
              }),
            ]}
          >
            <Input.Password placeholder="再次输入新密码" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Checkbox
              checked={pwdDismiss}
              onChange={(e) => setPwdDismiss(e.target.checked)}
            >
              本次登录不再提醒
            </Checkbox>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
