import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { App, Form, Input, Button } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { authApi } from '@/api/modules/auth'
import { useAuthStore, PWD_FORCE_CHANGE_DISMISS_KEY } from '@/stores/auth'
import './login.css'

const ADMIN_ROLES = ['system_admin', 'college_admin', 'school_admin']

export default function Login() {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { message } = App.useApp()
  const setAuth = useAuthStore((s) => s.setAuth)

  const onFinish = async (values: { user_no: string; password: string }) => {
    setLoading(true)
    try {
      const res = await authApi.login(values)
      // 兼容两种后端：Go 后端返回 access_token；旧后端通过 HttpOnly Cookie 鉴权
      setAuth(res.access_token || 'cookie-auth', res.user)
      // 重新登录时清除"本次登录不再提醒"标记，未改密则登录后重新弹窗提示
      localStorage.removeItem(PWD_FORCE_CHANGE_DISMISS_KEY)
      message.success('登录成功')
      const roles = res.user.roles || (res.user.role ? [res.user.role] : [])
      // 首次登录提示修改初始密码
      if (res.user.must_change_password) {
        message.warning('首次登录请修改初始密码', 3)
      }
      const from = (location.state as { from?: string } | null)?.from
      if (from) {
        navigate(from, { replace: true })
      } else if (roles.some((r) => ADMIN_ROLES.includes(r))) {
        navigate('/admin/dashboard', { replace: true })
      } else {
        navigate('/mobile/home', { replace: true })
      }
    } catch (e) {
      message.error(e instanceof Error ? e.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-container">
        <div className="login-header">
          <img src="/nnlg_logo.ico" alt="南理教评系统" className="login-logo" />
          <h1>南理教评系统v2</h1>
          <p>Teaching Evaluation System</p>
        </div>
        <Form onFinish={onFinish} className="login-form" requiredMark={false}>
          <Form.Item
            name="user_no"
            label="工号"
            rules={[{ required: true, message: '请输入工号' }]}
          >
            <Input prefix={<UserOutlined style={{ color: '#999' }} />} placeholder="请输入工号" size="large" autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password prefix={<LockOutlined style={{ color: '#999' }} />} placeholder="请输入密码" size="large" autoComplete="current-password" />
          </Form.Item>
          <div className="login-btn-wrapper">
            <Button type="primary" htmlType="submit" block size="large" shape="round" loading={loading}>
              登录
            </Button>
          </div>
        </Form>
      </div>
    </div>
  )
}
