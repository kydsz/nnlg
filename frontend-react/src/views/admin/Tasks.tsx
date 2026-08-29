import { useState } from 'react'
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
  Tag,
  DatePicker,
} from 'antd'
import { PlusOutlined, SearchOutlined, DownloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { taskApi, type TaskPayload } from '@/api/modules/tasks'
import { userApi } from '@/api/modules/users'
import { downloadBlob, exportFilename } from '@/utils/download'
import type { Task } from '@/api/types'
import { taskStatusInfo, formatDate } from '@/utils/format'
import { useDebounce } from '@/hooks/useDebounce'

export default function Tasks() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [params, setParams] = useState<{ page: number; page_size: number; keyword?: string; status?: number }>({
    page: 1,
    page_size: 20,
  })
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)
  const [modalOpen, setModalOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)
  const [editing, setEditing] = useState<Task | null>(null)
  const [form] = Form.useForm()
  const [batchForm] = Form.useForm()

  const effectiveKeyword = debouncedSearch

  const { data, isLoading } = useQuery({
    queryKey: ['tasks', params, effectiveKeyword],
    queryFn: () => taskApi.list({ ...params, keyword: effectiveKeyword }),
  })

  // 教师选项（搜索）
  const { data: teachers, refetch: refetchTeachers } = useQuery({
    queryKey: ['teacher-options', ''],
    queryFn: () => userApi.list({ page: 1, page_size: 50, role: 'teacher' }),
    enabled: modalOpen,
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['tasks'] })

  const saveMut = useMutation({
    mutationFn: (values: { course_name: string; teacher_id: number; classroom?: string; class_time?: string }) => {
      const payload: TaskPayload = {
        ...values,
        class_time: values.class_time || undefined,
      }
      return editing ? taskApi.update(editing.id, payload) : taskApi.create(payload)
    },
    onSuccess: () => {
      message.success('保存成功')
      setModalOpen(false)
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const cancelMut = useMutation({
    mutationFn: (id: number) => taskApi.cancel(id),
    onSuccess: () => {
      message.success('已取消')
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => taskApi.remove(id),
    onSuccess: () => {
      message.success('删除成功')
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const exportMut = useMutation({
    mutationFn: (p: typeof params) => taskApi.export({ ...p, format: 'xlsx' }),
    onSuccess: (blob) => {
      downloadBlob(blob, exportFilename('评教任务'))
    },
    onError: (e) => message.error(e.message),
  })

  const teacherOptions = (teachers?.list || []).map((t) => ({
    label: `${t.username}（${t.user_no}）`,
    value: t.id,
  }))

  const columns: ColumnsType<Task> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '课程', dataIndex: 'course_name' },
    { title: '教师', dataIndex: 'teacher_name', width: 110 },
    { title: '学院', dataIndex: 'teacher_college_name', width: 140, render: (v) => v || '-' },
    { title: '教室', dataIndex: 'classroom', width: 100, render: (v) => v || '-' },
    {
      title: '上课时间',
      dataIndex: 'class_time',
      width: 160,
      render: (v: string) => formatDate(v),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: number) => {
        const info = taskStatusInfo(v)
        return <Tag color={info.color}>{info.text}</Tag>
      },
    },
    { title: '评教数', dataIndex: 'evaluation_count', width: 80 },
    { title: '创建人', dataIndex: 'create_by_name', width: 100 },
    {
      title: '操作',
      width: 200,
      render: (_, record) => (
        <Space>
          <Button
            size="small"
            onClick={() => {
              setEditing(record)
              form.setFieldsValue({
                course_name: record.course_name,
                teacher_id: record.teacher_id,
                classroom: record.classroom,
              })
              setModalOpen(true)
            }}
          >
            编辑
          </Button>
          {record.status === 1 && (
            <Popconfirm title="确认取消该任务？" onConfirm={() => cancelMut.mutate(record.id)}>
              <Button size="small">取消</Button>
            </Popconfirm>
          )}
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
      <h3 className="page-title">评教任务</h3>
      <div className="filter-bar">
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="课程名称"
          style={{ width: 220 }}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Select
          allowClear
          placeholder="状态"
          style={{ width: 120 }}
          options={[
            { label: '待评', value: 1 },
            { label: '已评', value: 2 },
            { label: '已取消', value: 3 },
          ]}
          onChange={(v) =>
            setParams((p) => ({
              ...p,
              page: 1,
              // status 存入 params
              ...(v != null ? { status: v } : {}),
            }))
          }
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
          创建任务
        </Button>
        <Button
          icon={<PlusOutlined />}
          onClick={() => {
            batchForm.resetFields()
            setBatchOpen(true)
          }}
        >
          批量创建
        </Button>
        <Button
          icon={<DownloadOutlined />}
          loading={exportMut.isPending}
          onClick={() => exportMut.mutate({ ...params, keyword: effectiveKeyword })}
        >
          导出
        </Button>
      </div>
      <Table<Task>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={{
          current: params.page,
          pageSize: params.page_size,
          total: data?.total,
          showSizeChanger: true,
          onChange: (page, pageSize) => setParams((p) => ({ ...p, page, page_size: pageSize })),
        }}
      />
      <Modal
        title={editing ? '编辑任务' : '创建任务'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMut.isPending}
        afterOpenChange={(open) => {
          if (open) refetchTeachers()
        }}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={(v) =>
            saveMut.mutate({
              ...v,
              class_time: v.class_time
                ? (v.class_time as { format: (f: string) => string }).format(
                    'YYYY-MM-DD HH:mm:ss'
                  )
                : undefined,
            })
          }
        >
          <Form.Item
            name="course_name"
            label="课程名称"
            rules={[{ required: true, message: '请输入课程名称' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="teacher_id"
            label="被评教师"
            rules={[{ required: true, message: '请选择教师' }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              options={teacherOptions}
              placeholder="选择教师"
            />
          </Form.Item>
          <Form.Item name="classroom" label="教室">
            <Input />
          </Form.Item>
          <Form.Item name="class_time" label="上课时间">
            <DatePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <BatchCreateModal
        open={batchOpen}
        onClose={() => setBatchOpen(false)}
        teacherOptions={teacherOptions}
        onCreated={invalidate}
      />
    </div>
  )
}

/** 批量创建：一个教师多个课程名，一次生成多个任务 */
function BatchCreateModal({
  open,
  onClose,
  teacherOptions,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  teacherOptions: { label: string; value: number }[]
  onCreated: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm()

  const batchMut = useMutation({
    mutationFn: (v: { teacher_id: number; course_names: string; classroom?: string }) => {
      const names = v.course_names
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean)
      return taskApi.batchCreate(
        names.map((course_name) => ({
          course_name,
          teacher_id: v.teacher_id,
          classroom: v.classroom,
        }))
      )
    },
    onSuccess: (res) => {
      message.success(`批量创建完成：成功 ${res.created_count ?? '-'} 个`)
      onCreated()
      onClose()
    },
    onError: (e) => message.error(e.message),
  })

  return (
    <Modal
      title="批量创建任务"
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={batchMut.isPending}
    >
      <Form form={form} layout="vertical" onFinish={(v) => batchMut.mutate(v)}>
        <Form.Item
          name="teacher_id"
          label="被评教师"
          rules={[{ required: true, message: '请选择教师' }]}
        >
          <Select showSearch optionFilterProp="label" options={teacherOptions} />
        </Form.Item>
        <Form.Item
          name="course_names"
          label="课程名称（每行一个）"
          rules={[{ required: true, message: '请输入课程名称' }]}
        >
          <Input.TextArea rows={5} placeholder={'高等数学\n线性代数\n概率论'} />
        </Form.Item>
        <Form.Item name="classroom" label="教室（可选）">
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  )
}
