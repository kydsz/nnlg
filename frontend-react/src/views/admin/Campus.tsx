import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, Button, Modal, Form, Input, InputNumber, Select, Space, App, Popconfirm } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { orgApi } from '@/api/modules/org'
import type { Campus } from '@/api/types'
import { formatDate } from '@/utils/format'

export default function Campus() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<Campus | null>(null)
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({
    queryKey: ['campuses'],
    queryFn: () => orgApi.campusList(),
  })

  const saveMut = useMutation({
    mutationFn: (values: { name: string; sort_order?: number; status?: number }) =>
      editing
        ? orgApi.campusUpdate(editing.id, values)
        : orgApi.campusCreate(values),
    onSuccess: () => {
      message.success('保存成功')
      setModalOpen(false)
      qc.invalidateQueries({ queryKey: ['campuses'] })
    },
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => orgApi.campusDelete(id),
    onSuccess: () => {
      message.success('删除成功')
      qc.invalidateQueries({ queryKey: ['campuses'] })
    },
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<Campus> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '校区名称', dataIndex: 'name' },
    { title: '排序', dataIndex: 'sort_order', width: 80 },
    { title: '学院数', dataIndex: 'college_count', width: 90 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: number) => (v === 1 ? '启用' : '禁用'),
    },
    { title: '创建时间', dataIndex: 'create_time', width: 170, render: (v: string) => formatDate(v) },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space>
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
      <h3 className="page-title">校区管理</h3>
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
        新增校区
      </Button>
      <Table<Campus>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={false}
      />
      <Modal
        title={editing ? '编辑校区' : '新增校区'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(v) => saveMut.mutate(v)}
          initialValues={{ status: 1, sort_order: 0 }}
        >
          <Form.Item name="name" label="校区名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="sort_order" label="排序号">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="status" label="状态">
            <Select
              options={[
                { label: '启用', value: 1 },
                { label: '禁用', value: 0 },
              ]}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
