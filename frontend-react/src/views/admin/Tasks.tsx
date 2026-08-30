import { useEffect, useRef, useState } from 'react'
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
  type GetProp,
} from 'antd'
import dayjs from 'dayjs'
import { PlusOutlined, SearchOutlined, DownloadOutlined, LoadingOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { taskApi, type TaskPayload } from '@/api/modules/tasks'
import { userApi } from '@/api/modules/users'
import { downloadBlob, exportFilename } from '@/utils/download'
import type { Task, User } from '@/api/types'
import { taskStatusInfo, formatDate } from '@/utils/format'
import { useDebounce } from '@/hooks/useDebounce'
import { useSemesterRangePicker } from '@/hooks/useSemesterDates'

export default function Tasks() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [params, setParams] = useState<{
    page: number
    page_size: number
    keyword?: string
    status?: number
    start_date?: string
    end_date?: string
  }>({
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
  // 默认日期区间：当前学期（开学日 ~ 开学日+weeks×7天），手动选择后以手动值为准
  const { effective: dates, onRange } = useSemesterRangePicker()

  const effectiveKeyword = debouncedSearch

  const { data, isLoading } = useQuery({
    queryKey: ['tasks', params, effectiveKeyword, dates],
    queryFn: () => taskApi.list({ ...params, keyword: effectiveKeyword, start_date: dates[0], end_date: dates[1] }),
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
        <DatePicker.RangePicker
          value={dates[0] && dates[1] ? [dayjs(dates[0]), dayjs(dates[1])] : undefined}
          onChange={(v) =>
            onRange(v ? [v[0]!.format('YYYY-MM-DD'), v[1]!.format('YYYY-MM-DD')] : null)
          }
        />
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
          onClick={() =>
            exportMut.mutate({
              ...params,
              keyword: effectiveKeyword,
              start_date: dates[0],
              end_date: dates[1],
            })
          }
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
            <TeacherSelect />
          </Form.Item>
          <Form.Item name="classroom" label="教室">
            <Input />
          </Form.Item>
          <Form.Item name="class_time" label="上课时间">
            <DatePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <BatchCreateModal open={batchOpen} onClose={() => setBatchOpen(false)} onCreated={invalidate} />
    </div>
  )
}

/** 批量创建：一个教师多个课程名，一次生成多个任务 */
function BatchCreateModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
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
          <TeacherSelect />
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

/**
 * 教师选择器：远程搜索（姓名/工号）+ 下拉滚动分页动态加载，
 * 编辑时自动回显已选教师（不在当前已加载列表时单独拉取）。
 */
function TeacherSelect({
  value,
  onChange,
}: {
  value?: number
  onChange?: (v: number | undefined) => void
}) {
  const PAGE_SIZE = 50
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const [options, setOptions] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<User | null>(null)
  const reqId = useRef(0)

  const fetchPage = async (p: number, kw: string, append: boolean) => {
    const id = ++reqId.current
    setLoading(true)
    try {
      const res = await userApi.list({
        page: p,
        page_size: PAGE_SIZE,
        role: 'teacher',
        keyword: kw || undefined,
      })
      if (id !== reqId.current) return // 已有更新的请求，丢弃过期结果
      setOptions((prev) => (append ? [...prev, ...(res.list || [])] : res.list || []))
      setTotal(res.total ?? 0)
      setPage(p)
    } finally {
      if (id === reqId.current) setLoading(false)
    }
  }

  useEffect(() => {
    fetchPage(1, debouncedKeyword, false)
  }, [debouncedKeyword])

  // 编辑回显：选中教师不在当前已加载列表时，单独拉取该教师信息
  useEffect(() => {
    if (value == null) {
      setSelected(null)
      return
    }
    if (selected?.id === value || options.some((o) => o.id === value)) return
    userApi
      .get(value)
      .then((u) => setSelected(u || null))
      .catch(() => {})
  }, [value])

  const handlePopupScroll: GetProp<typeof Select, 'onPopupScroll'> = (e) => {
    const el = e.currentTarget
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 40 && !loading && options.length < total) {
      fetchPage(page + 1, debouncedKeyword, true)
    }
  }

  // 保持列表原始顺序；仅当已选中教师不在当前已加载列表时（如翻页后）追加到末尾用于回显
  const mergedOptions =
    selected && !options.some((o) => o.id === selected.id) ? [...options, selected] : options

  return (
    <Select
      showSearch
      allowClear
      filterOption={false}
      placeholder="选择教师（支持姓名/工号搜索）"
      value={value}
      loading={loading}
      onSearch={setKeyword}
      onPopupScroll={handlePopupScroll}
      onChange={(v) => {
        // 选中/清空后重置搜索词，下次打开恢复完整列表的原始顺序
        setKeyword('')
        if (v == null) {
          setSelected(null)
          onChange?.(undefined)
          return
        }
        const u = mergedOptions.find((o) => o.id === v)
        setSelected(u || null)
        onChange?.(v as number)
      }}
      notFoundContent={loading ? <LoadingOutlined spin /> : '暂无教师'}
      options={mergedOptions.map((t) => ({
        value: t.id,
        label: `${t.username}（${t.user_no}）`,
        desc: t.college_name || undefined,
      }))}
      optionRender={(option) => {
        const { label, desc } = option as unknown as { label?: React.ReactNode; desc?: string }
        return (
          <Space>
            <span>{label}</span>
            {desc && <span style={{ color: '#999', fontSize: 12 }}>{desc}</span>}
          </Space>
        )
      }}
    />
  )
}
