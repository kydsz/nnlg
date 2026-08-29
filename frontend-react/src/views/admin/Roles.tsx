import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  App,
  Popconfirm,
  Tag,
  Checkbox,
} from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { roleApi } from '@/api/modules/roles'
import type { RoleInfo, PermissionGroup } from '@/api/types'

const DATA_SCOPES = [
  { label: '全部数据', value: 'all' },
  { label: '本学院数据', value: 'college' },
  { label: '仅个人数据', value: 'self' },
]

export default function Roles() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<RoleInfo | null>(null)
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({ queryKey: ['roles'], queryFn: () => roleApi.list() })
  const { data: permGroups } = useQuery({
    queryKey: ['role-permissions'],
    queryFn: () => roleApi.permissionGroups(),
  })

  const permNameMap = useMemo(() => {
    const m = new Map<string, string>()
    ;(permGroups || []).flatMap((g) => g.permissions).forEach((p) => m.set(p.code, p.name))
    return m
  }, [permGroups])

  const saveMut = useMutation({
    mutationFn: (values: {
      name: string
      code?: string
      description?: string
      level?: number
      data_scope?: 'all' | 'college' | 'self'
      permissions?: string[]
    }) => (editing ? roleApi.update(editing.id, values) : roleApi.create(values)),
    onSuccess: () => {
      message.success('保存成功')
      setModalOpen(false)
      qc.invalidateQueries({ queryKey: ['roles'] })
    },
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => roleApi.remove(id),
    onSuccess: () => {
      message.success('删除成功')
      qc.invalidateQueries({ queryKey: ['roles'] })
    },
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<RoleInfo> = [
    { title: '角色名称', dataIndex: 'name', width: 140 },
    { title: '编码', dataIndex: 'code', width: 160 },
    {
      title: '权限',
      dataIndex: 'permissions',
      render: (perms: string[]) => (
        <Space size={4} wrap>
          {(perms || []).map((p) => (
            <Tag key={p}>{permNameMap.get(p) || p}</Tag>
          ))}
        </Space>
      ),
    },
    { title: '数据范围', dataIndex: 'data_scope', width: 110, render: (v: string) => DATA_SCOPES.find((s) => s.value === v)?.label || v },
    { title: '优先级', dataIndex: 'level', width: 80 },
    {
      title: '内置',
      dataIndex: 'is_system',
      width: 70,
      render: (v: number) => (v === 1 ? <Tag color="orange">内置</Tag> : '-'),
    },
    {
      title: '操作',
      width: 130,
      render: (_, record) => (
        <Space>
          {record.code !== 'system_admin' && (
            <Button
              size="small"
              onClick={() => {
                setEditing(record)
                form.setFieldsValue(record)
                setModalOpen(true)
              }}
            >
              编辑
            </Button>
          )}
          {record.is_system !== 1 && (
            <Popconfirm title="确认删除？" onConfirm={() => delMut.mutate(record.id)}>
              <Button size="small" danger>
                删除
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <h3 className="page-title">角色管理</h3>
      <Button
        type="primary"
        icon={<PlusOutlined />}
        style={{ marginBottom: 12 }}
        onClick={() => {
          setEditing(null)
          form.resetFields()
          setModalOpen(true)
        }}
      >
        新增角色
      </Button>
      <Table<RoleInfo>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={false}
      />
      <Modal
        title={editing ? '编辑角色' : '新增角色'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
        width={620}
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMut.mutate(v)}>
          <Form.Item name="name" label="角色名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          {!editing && (
            <Form.Item name="code" label="角色编码" rules={[{ required: true, message: '请输入编码' }]}>
              <Input placeholder="如 department_admin" />
            </Form.Item>
          )}
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} />
          </Form.Item>
          <Form.Item name="level" label="优先级（数字越小权限越高）">
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="data_scope" label="数据范围" initialValue="self">
            <Select options={DATA_SCOPES} />
          </Form.Item>
          <Form.Item name="permissions" label="权限">
            <PermissionCheckboxes groups={permGroups || []} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

function PermissionCheckboxes({
  groups,
  value,
  onChange,
}: {
  groups: PermissionGroup[]
  value?: string[]
  onChange?: (checked: string[]) => void
}) {
  // 透传 Form.Item 注入的 value/onChange，保证回显与勾选受控
  return (
    <Checkbox.Group
      value={value}
      onChange={onChange}
      style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', rowGap: 8 }}
      options={groups.flatMap((g) =>
        g.permissions.map((p) => ({ label: `${g.group}·${p.name}`, value: p.code }))
      )}
    />
  )
}
