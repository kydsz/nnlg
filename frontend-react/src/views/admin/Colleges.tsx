import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, Button, Modal, Form, Input, Select, Space, App, Popconfirm } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { orgApi } from '@/api/modules/org'
import type { College } from '@/api/types'

export default function Colleges() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<College | null>(null)
  const [campusFilter, setCampusFilter] = useState<number | undefined>()
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({
    queryKey: ['colleges', campusFilter],
    queryFn: () => orgApi.collegeList({ campus_id: campusFilter }),
  })
  const { data: campuses } = useQuery({
    queryKey: ['campuses'],
    queryFn: () => orgApi.campusList(),
  })

  const saveMut = useMutation({
    mutationFn: (values: { name: string; code: string; campus_id?: number }) =>
      editing ? orgApi.collegeUpdate(editing.id, values) : orgApi.collegeCreate(values),
    onSuccess: () => {
      message.success('保存成功')
      setModalOpen(false)
      qc.invalidateQueries({ queryKey: ['colleges'] })
    },
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => orgApi.collegeDelete(id),
    onSuccess: () => {
      message.success('删除成功')
      qc.invalidateQueries({ queryKey: ['colleges'] })
    },
    onError: (e) => message.error(e.message),
  })

  const campusOptions = (campuses?.list || []).map((c) => ({
    label: c.name,
    value: c.id,
  }))

  const columns: ColumnsType<College> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '编码', dataIndex: 'code', width: 110 },
    { title: '学院名称', dataIndex: 'name' },
    { title: '所属校区', dataIndex: 'campus_name' },
    { title: '教研室数', dataIndex: 'research_room_count', width: 90 },
    { title: '用户数', dataIndex: 'user_count', width: 90 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => (v === 1 ? '启用' : '禁用'),
    },
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
      <h3 className="page-title">学院管理</h3>
      <div className="filter-bar">
        <Select
          allowClear
          placeholder="按校区筛选"
          style={{ width: 180 }}
          options={campusOptions}
          value={campusFilter}
          onChange={setCampusFilter}
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
          新增学院
        </Button>
      </div>
      <Table<College>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={false}
      />
      <Modal
        title={editing ? '编辑学院' : '新增学院'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMut.mutate(v)}>
          <Form.Item name="name" label="学院名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="code"
            label="学院编码"
            rules={[{ required: true, message: '请输入编码' }]}
          >
            <Input disabled={!!editing} />
          </Form.Item>
          <Form.Item name="campus_id" label="所属校区">
            <Select allowClear options={campusOptions} placeholder="选择校区" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
