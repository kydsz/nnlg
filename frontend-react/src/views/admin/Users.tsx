import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  Select,
  Space,
  App,
  Popconfirm,
  Switch,
  Tag,
  Drawer,
  Card,
  Checkbox,
} from 'antd'
import { MinusOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { userApi, type UserListParams } from '@/api/modules/users'
import { orgApi } from '@/api/modules/org'
import { roleApi } from '@/api/modules/roles'
import type { User } from '@/api/types'
import { ROLE_NAMES } from '@/utils/roleNames'
import { formatDate } from '@/utils/format'

// 督导角色才允许配置督导负责范围
const SUPERVISOR_ROLES = ['school_supervisor', 'college_supervisor', 'supervisor']
const isSupervisorUser = (u: User) => (u.roles || []).some((r) => SUPERVISOR_ROLES.includes(r))

export default function Users() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [params, setParams] = useState<UserListParams>({ page: 1, page_size: 20 })
  const [keyword, setKeyword] = useState('')
  const [sortState, setSortState] = useState<{ field?: string; order?: 'ascend' | 'descend' }>({})
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [scopeUser, setScopeUser] = useState<User | null>(null)
  const [selectedIds, setSelectedIds] = useState<React.Key[]>([])
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({
    queryKey: ['users', params],
    queryFn: () => userApi.list(params),
  })
  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
  })
  const { data: rooms } = useQuery({
    queryKey: ['research-rooms', undefined],
    queryFn: () => orgApi.roomList(),
  })
  const { data: roles } = useQuery({
    queryKey: ['roles'],
    queryFn: () => roleApi.list(),
  })

  const collegeOptions = (colleges?.list || []).map((c) => ({ label: c.name, value: c.id }))
  const roomOptions = (rooms?.list || []).map((r) => ({ label: r.name, value: r.id }))
  const roleOptions = (roles?.list || []).map((r) => ({ label: r.name, value: r.code }))

  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] })

  const saveMut = useMutation({
    mutationFn: (values: {
      user_no: string
      username: string
      password?: string
      roles: string[]
      college_id?: number
      research_room_id?: number
      status?: boolean
    }) => {
      const payload = {
        username: values.username,
        roles: values.roles,
        college_id: values.college_id,
        research_room_id: values.research_room_id,
        // Switch 通过 valuePropName="checked" 绑定，值为 boolean，需转为 0/1
        status: values.status ? 1 : 0,
      }
      if (editing) {
        return userApi.update(editing.id, {
          ...payload,
          ...(values.password ? { password: values.password } : {}),
        })
      }
      return userApi.create({
        user_no: values.user_no,
        ...payload,
        role: values.roles[0] || 'teacher',
        ...(values.password ? { password: values.password } : {}),
      })
    },
    onSuccess: () => {
      message.success('保存成功')
      setModalOpen(false)
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const statusMut = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) =>
      userApi.updateStatus(id, status),
    onSuccess: () => {
      message.success('状态已更新')
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const batchMut = useMutation({
    mutationFn: ({ status }: { status: number }) =>
      userApi.batchStatus(selectedIds as number[], status),
    onSuccess: () => {
      message.success('批量操作成功')
      setSelectedIds([])
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => userApi.remove(id),
    onSuccess: () => {
      message.success('删除成功')
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<User> = [
    {
      title: '工号',
      dataIndex: 'user_no',
      width: 110,
      sorter: true,
      sortOrder: sortState.field === 'user_no' ? sortState.order : null,
    },
    {
      title: '姓名',
      dataIndex: 'username',
      width: 110,
      sorter: true,
      sortOrder: sortState.field === 'username' ? sortState.order : null,
    },
    {
      title: '学院',
      dataIndex: 'college_name',
      width: 140,
      sorter: true,
      sortOrder: sortState.field === 'college_name' ? sortState.order : null,
      render: (v) => v || '-',
    },
    {
      title: '角色',
      dataIndex: 'roles',
      render: (roles: string[]) => (
        <Space size={4} wrap>
          {(roles || []).map((r) => (
            <Tag key={r} color="blue">
              {ROLE_NAMES[r] || r}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number, record) => (
        <Switch
          checked={v === 1}
          size="small"
          onChange={(checked) =>
            statusMut.mutate({ id: record.id, status: checked ? 1 : 0 })
          }
        />
      ),
    },
    {
      title: '最后登录',
      dataIndex: 'last_login_time',
      width: 160,
      sorter: true,
      sortOrder: sortState.field === 'last_login_time' ? sortState.order : null,
      render: (v: string) => formatDate(v),
    },
    {
      title: '操作',
      width: 230,
      render: (_, record) => (
        <Space>
          <Button
            size="small"
            onClick={() => {
              setEditing(record)
              form.setFieldsValue({
                ...record,
                password: undefined,
              })
              setModalOpen(true)
            }}
          >
            编辑
          </Button>
          <Button
            size="small"
            disabled={!isSupervisorUser(record)}
            onClick={() => setScopeUser(record)}
          >
            督导范围
          </Button>
          <Popconfirm title="确认删除？" onConfirm={() => delMut.mutate(record.id)}>
            <Button size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <h3 className="page-title">用户管理</h3>
      <div className="filter-bar">
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="工号/姓名"
          style={{ width: 200 }}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          onPressEnter={() => setParams((p) => ({ ...p, page: 1, keyword }))}
        />
        <Select
          allowClear
          placeholder="角色"
          style={{ width: 150 }}
          options={roleOptions}
          onChange={(v) => setParams((p) => ({ ...p, page: 1, role: v }))}
        />
        <Select
          allowClear
          placeholder="学院"
          style={{ width: 180 }}
          options={collegeOptions}
          onChange={(v) => setParams((p) => ({ ...p, page: 1, college_id: v }))}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 120 }}
          options={[
            { label: '启用', value: 1 },
            { label: '禁用', value: 0 },
          ]}
          onChange={(v) => setParams((p) => ({ ...p, page: 1, status: v }))}
        />
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditing(null)
            form.resetFields()
            setModalOpen(true)
          }}
        >
          新增用户
        </Button>
        {selectedIds.length > 0 && (
          <Space>
            <span style={{ color: '#999' }}>已选 {selectedIds.length} 项</span>
            <Popconfirm title="确认批量启用？" onConfirm={() => batchMut.mutate({ status: 1 })}>
              <Button size="small">批量启用</Button>
            </Popconfirm>
            <Popconfirm title="确认批量禁用？" onConfirm={() => batchMut.mutate({ status: 0 })}>
              <Button size="small" danger>
                批量禁用
              </Button>
            </Popconfirm>
          </Space>
        )}
      </div>
      <Table<User>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        rowSelection={{
          selectedRowKeys: selectedIds,
          onChange: setSelectedIds,
        }}
        onChange={(_p, _f, sorter, extra) => {
          // 仅排序触发时处理排序；换页/筛选变化由 pagination.onChange 单独处理，避免把页码重置回 1
          if (extra?.action !== 'sort') return
          const s = Array.isArray(sorter) ? sorter[0] : sorter
          const field = (s?.field as string) || undefined
          const order =
            s?.order === 'ascend' ? 'asc' : s?.order === 'descend' ? 'desc' : undefined
          setSortState(field && order ? { field, order: s!.order as 'ascend' | 'descend' } : {})
          setParams((p) => ({ ...p, page: 1, order_by: field, order }))
        }}
        pagination={{
          current: params.page,
          pageSize: params.page_size,
          total: data?.total,
          showSizeChanger: true,
          onChange: (page, pageSize) => {
            setKeyword(keyword)
            setParams((p) => ({ ...p, page, page_size: pageSize, keyword }))
          },
        }}
      />

      <Modal
        title={editing ? '编辑用户' : '新增用户'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
        width={520}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(v) => saveMut.mutate(v)}
          initialValues={{ status: 1, roles: ['teacher'] }}
        >
          <Form.Item name="user_no" label="工号" rules={[{ required: true, message: '请输入工号' }]}>
            <Input disabled={!!editing} />
          </Form.Item>
          <Form.Item name="username" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="password"
            label={editing ? '密码（留空不修改）' : '密码'}
            rules={editing ? [] : [{ required: true, message: '请输入密码' }]}
          >
            <Input.Password />
          </Form.Item>
          <Form.Item name="roles" label="角色（多选）" rules={[{ required: true, message: '请选择角色' }]}>
            <Select mode="multiple" options={roleOptions} placeholder="选择角色" />
          </Form.Item>
          <Form.Item name="college_id" label="所属学院">
            <Select allowClear options={collegeOptions} />
          </Form.Item>
          <Form.Item name="research_room_id" label="所属教研室">
            <Select allowClear options={roomOptions} />
          </Form.Item>
          <Form.Item name="status" label="状态" valuePropName="checked">
            <Switch checkedChildren="启用" unCheckedChildren="禁用" />
          </Form.Item>
        </Form>
      </Modal>

      <SupervisorScopeDrawer user={scopeUser} onClose={() => setScopeUser(null)} />
    </div>
  )
}

function SupervisorScopeDrawer({ user, onClose }: { user: User | null; onClose: () => void }) {
  const { message } = App.useApp()
  const [collegeIds, setCollegeIds] = useState<number[]>([])
  const [roomIds, setRoomIds] = useState<number[]>([])
  const [loading, setLoading] = useState(false)
  // 卡片折叠（列表太长时可缩回）
  const [collegeCollapsed, setCollegeCollapsed] = useState(false)
  const [roomCollapsed, setRoomCollapsed] = useState(false)

  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
  })
  const { data: rooms } = useQuery({
    queryKey: ['research-rooms', undefined],
    queryFn: () => orgApi.roomList(),
  })

  // 每次打开都从服务端拉取最新范围，避免命中旧缓存导致“保存成功但重开为空”
  useEffect(() => {
    if (!user) {
      setCollegeIds([])
      setRoomIds([])
      return
    }
    let alive = true
    setLoading(true)
    userApi
      .getSupervisorScope(user.id)
      .then((d) => {
        if (!alive) return
        setCollegeIds(d.supervisor_college_ids || [])
        setRoomIds(d.supervisor_research_room_ids || [])
      })
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [user?.id])

  const saveMut = useMutation({
    mutationFn: () =>
      userApi.updateSupervisorScope(user!.id, {
        college_ids: collegeIds,
        research_room_ids: roomIds,
      }),
    onSuccess: () => {
      message.success('督导范围已保存')
      onClose()
    },
    onError: (e) => message.error(e.message),
  })

  return (
    <Drawer title={`督导范围 - ${user?.username || ''}`} open={!!user} onClose={onClose} width={420}>
      <Card
        size="small"
        title="负责学院"
        style={{ marginBottom: 16 }}
        extra={
          <Button
            type="text"
            size="small"
            icon={collegeCollapsed ? <PlusOutlined /> : <MinusOutlined />}
            onClick={() => setCollegeCollapsed((v) => !v)}
          />
        }
      >
        {!collegeCollapsed && (
          <Checkbox.Group
            value={collegeIds}
            onChange={(v) => setCollegeIds(v as number[])}
            options={(colleges?.list || []).map((c) => ({ label: c.name, value: c.id }))}
            style={{ display: 'grid', gridTemplateColumns: '1fr 1fr' }}
          />
        )}
      </Card>
      <Card
        size="small"
        title="负责教研室"
        style={{ marginBottom: 16 }}
        extra={
          <Button
            type="text"
            size="small"
            icon={roomCollapsed ? <PlusOutlined /> : <MinusOutlined />}
            onClick={() => setRoomCollapsed((v) => !v)}
          />
        }
      >
        {!roomCollapsed && (
          <Checkbox.Group
            value={roomIds}
            onChange={(v) => setRoomIds(v as number[])}
            options={(rooms?.list || []).map((r) => ({ label: r.name, value: r.id }))}
            style={{ display: 'grid', gridTemplateColumns: '1fr' }}
          />
        )}
      </Card>
      <Button type="primary" block loading={saveMut.isPending || loading} onClick={() => saveMut.mutate()}>
        保存
      </Button>
    </Drawer>
  )
}
