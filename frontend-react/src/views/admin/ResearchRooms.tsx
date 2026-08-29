import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, Button, Modal, Form, Input, Select, Space, App, Popconfirm } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { orgApi } from '@/api/modules/org'
import type { ResearchRoom } from '@/api/types'

export default function ResearchRooms() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ResearchRoom | null>(null)
  const [collegeFilter, setCollegeFilter] = useState<number | undefined>()
  const [form] = Form.useForm()

  const { data, isLoading } = useQuery({
    queryKey: ['research-rooms', collegeFilter],
    queryFn: () => orgApi.roomList({ college_id: collegeFilter }),
  })
  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
  })

  const collegeOptions = (colleges?.list || []).map((c) => ({ label: c.name, value: c.id }))

  const saveMut = useMutation({
    mutationFn: (values: { name: string; college_id: number }) =>
      editing ? orgApi.roomUpdate(editing.id, values) : orgApi.roomCreate(values),
    onSuccess: () => {
      message.success('保存成功')
      setModalOpen(false)
      qc.invalidateQueries({ queryKey: ['research-rooms'] })
    },
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => orgApi.roomDelete(id),
    onSuccess: () => {
      message.success('删除成功')
      qc.invalidateQueries({ queryKey: ['research-rooms'] })
    },
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<ResearchRoom> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '教研室名称', dataIndex: 'name' },
    { title: '所属学院', dataIndex: 'college_name' },
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
      <h3 className="page-title">教研室管理</h3>
      <div className="filter-bar">
        <Select
          allowClear
          placeholder="按学院筛选"
          style={{ width: 200 }}
          options={collegeOptions}
          value={collegeFilter}
          onChange={setCollegeFilter}
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
          新增教研室
        </Button>
      </div>
      <Table<ResearchRoom>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={false}
      />
      <Modal
        title={editing ? '编辑教研室' : '新增教研室'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMut.mutate(v)}>
          <Form.Item name="name" label="教研室名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="college_id"
            label="所属学院"
            rules={[{ required: true, message: '请选择学院' }]}
          >
            <Select options={collegeOptions} placeholder="选择学院" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
