import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { List, Tag, Button, Dialog, Input, Toast, NavBar, Picker, Popup } from 'antd-mobile'
import { AppOutline } from 'antd-mobile-icons'
import { authApi } from '@/api/modules/auth'
import { userApi } from '@/api/modules/users'
import { orgApi } from '@/api/modules/org'
import { useAuthStore } from '@/stores/auth'
import { roleNamesStr } from '@/utils/roleNames'

export default function Profile() {
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const logout = useAuthStore((s) => s.logout)
  const isAdmin = useAuthStore((s) => s.isAdmin)

  const [pwdOpen, setPwdOpen] = useState(false)
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [saving, setSaving] = useState(false)

  const [roomOpen, setRoomOpen] = useState(false)
  const [roomSelected, setRoomSelected] = useState<string[] | null>(null)

  // 本学院启用教研室列表
  const { data: rooms } = useQuery({
    queryKey: ['research-rooms', 'my-college', user?.college_id],
    queryFn: () => orgApi.roomList({ college_id: user!.college_id!, status: 1 }),
    enabled: roomOpen && user?.college_id != null,
  })
  const roomColumns = [
    [
      { label: '未设置', value: '' },
      ...(rooms?.list || []).map((r) => ({ label: r.name, value: String(r.id) })),
    ],
  ]

  const saveRoom = async () => {
    const val = roomSelected?.[0] ?? ''
    try {
      await userApi.updateMyResearchRoom(val ? Number(val) : null)
      const picked = (rooms?.list || []).find((r) => String(r.id) === val)
      if (user) {
        setUser({
          ...user,
          research_room_id: picked?.id ?? null,
          research_room_name: picked?.name ?? null,
        })
      }
      Toast.show({ content: '教研室已更新', icon: 'success' })
      setRoomOpen(false)
    } catch (e) {
      Toast.show({ content: e instanceof Error ? e.message : '更新失败', icon: 'fail' })
    }
  }

  const handleLogout = () => {
    Dialog.confirm({
      content: '确定退出登录？',
      onConfirm: () => {
        authApi.logout().catch(() => {})
        logout()
        navigate('/login', { replace: true })
      },
    })
  }

  const changePwd = async () => {
    if (!oldPwd || !newPwd) {
      Toast.show({ content: '请填写完整', icon: 'fail' })
      return
    }
    setSaving(true)
    try {
      await authApi.changePassword({ old_password: oldPwd, new_password: newPwd })
      Toast.show({ content: '密码修改成功', icon: 'success' })
      setPwdOpen(false)
      setOldPwd('')
      setNewPwd('')
    } catch (e) {
      Toast.show({ content: e instanceof Error ? e.message : '修改失败', icon: 'fail' })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <NavBar backArrow={false}>个人中心</NavBar>
      <div style={{ padding: 16, textAlign: 'center' }}>
        <div style={{ fontSize: 20, fontWeight: 600 }}>{user?.username}</div>
        <div style={{ color: '#999', marginTop: 4 }}>
          {user?.user_no} · {user?.college_name || '未分配学院'}
        </div>
        <div style={{ marginTop: 8 }}>
          {roleNamesStr(user?.roles)
            .split(' / ')
            .map((r) => (
              <Tag key={r} color="primary">
                {r}
              </Tag>
            ))}
        </div>
      </div>
      <List>
        {isAdmin() && (
          <List.Item
            prefix={<AppOutline />}
            clickable
            arrow
            onClick={() => navigate('/admin/dashboard')}
          >
            切换到管理端
          </List.Item>
        )}
        <List.Item
          clickable
          onClick={() => {
            setRoomSelected(user?.research_room_id ? [String(user.research_room_id)] : [''])
            setRoomOpen(true)
          }}
          extra={user?.research_room_name || '未设置'}
        >
          教研室
        </List.Item>
        <List.Item onClick={() => setPwdOpen(true)} arrow>
          修改密码
        </List.Item>
      </List>
      <div style={{ padding: 16 }}>
        <Button block color="danger" onClick={handleLogout}>
          退出登录
        </Button>
      </div>

      <Dialog
        visible={pwdOpen}
        title="修改密码"
        content={
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, paddingTop: 8 }}>
            <Input type="password" placeholder="旧密码" value={oldPwd} onChange={setOldPwd} />
            <Input type="password" placeholder="新密码" value={newPwd} onChange={setNewPwd} />
          </div>
        }
        actions={[
          [
            { key: 'cancel', text: '取消', onClick: () => setPwdOpen(false) },
            { key: 'ok', text: saving ? '保存中...' : '保存', bold: true, onClick: changePwd },
          ],
        ]}
        onClose={() => setPwdOpen(false)}
      />

      <Popup visible={roomOpen} onMaskClick={() => setRoomOpen(false)} destroyOnClose>
        <div style={{ padding: 12 }}>
          <Picker
            columns={roomColumns}
            value={roomSelected ?? ['']}
            onConfirm={(v) => {
              setRoomSelected(v as string[])
              saveRoom()
            }}
            onCancel={() => setRoomOpen(false)}
          />
        </div>
      </Popup>
    </div>
  )
}
