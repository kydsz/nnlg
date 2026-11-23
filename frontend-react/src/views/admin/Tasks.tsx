import { useRef, useState } from 'react'
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
  Tooltip,
  DatePicker,
} from 'antd'
import dayjs from 'dayjs'
import { PlusOutlined, SearchOutlined, DownloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { taskApi, type TaskPayload } from '@/api/modules/tasks'
import { downloadBlob, exportFilename } from '@/utils/download'
import type { Task, User } from '@/api/types'
import { taskStatusInfo, formatDate } from '@/utils/format'
import { useDebounce } from '@/hooks/useDebounce'
import { useSemesterRangePicker } from '@/hooks/useSemesterDates'
import { useAuthStore } from '@/stores/auth'
import TeacherSelect from '@/components/TeacherSelect'

export default function Tasks() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const userId = useAuthStore((s) => s.user?.id)
  const authCollegeId = useAuthStore((s) => s.user?.college_id ?? undefined)
  const canDeleteAny = useAuthStore((s) => s.hasPermission('task:delete'))
  const canDeleteOwn = useAuthStore((s) => s.hasPermission('task:delete_own'))
  const canDeleteTask = (r: Task) =>
    canDeleteAny || (canDeleteOwn && userId != null && r.create_by === userId)
  const canRestoreAny = useAuthStore((s) => s.hasPermission('task:restore'))
  const canRestoreOwn = useAuthStore((s) => s.hasPermission('task:restore_own'))
  const canRestoreTask = (r: Task) =>
    canRestoreAny || (canRestoreOwn && userId != null && r.create_by === userId)
  const [params, setParams] = useState<{
    page: number
    page_size: number
    keyword?: string
    status?: number
    start_date?: string
    end_date?: string
    order_by?: string
    order?: 'asc' | 'desc'
    deleted?: 0 | 1
  }>({
    page: 1,
    page_size: 20,
  })
  const [sortState, setSortState] = useState<{ field?: string; order?: 'ascend' | 'descend' }>({})
  // 切到「已删除」视图时状态筛选无意义（已删数据多为历史快照），会临时清空；
  // 这里记住原选择，切回「正常任务」时原样回显，避免用户的筛选条件被静默丢弃
  const savedStatusRef = useRef<number | undefined>(undefined)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 300)
  const [modalOpen, setModalOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)
  const [editing, setEditing] = useState<Task | null>(null)
  const [teacherCollegeOptions, setTeacherCollegeOptions] = useState<
    { label: string; value: number }[]
  >([])
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

  const collegeOptionsOf = (u?: Partial<User> | null) => {
    if (!u) return [] as { label: string; value: number }[]
    const map = new Map<number, string>()
    if (u.college_id != null && u.college_name) map.set(u.college_id, u.college_name)
    for (const tc of u.teaching_colleges || []) map.set(tc.college_id, tc.college_name)
    return Array.from(map, ([value, label]) => ({ value, label }))
  }

  const handleTeacherSelect = (u?: User) => {
    const opts = collegeOptionsOf(u)
    setTeacherCollegeOptions(opts)
    const current = form.getFieldValue('college_id') as number | undefined
    if (!opts.length) {
      form.setFieldValue('college_id', undefined)
      return
    }
    if (current == null || !opts.some((o) => o.value === current)) {
      // 优先调用者本院（学院管理员/教师的操作范围），其次教师人事学院
      const preferred =
        opts.find((o) => o.value === authCollegeId)?.value ??
        u?.college_id ??
        opts[0].value
      form.setFieldValue('college_id', preferred)
    }
  }

  const saveMut = useMutation({
    mutationFn: (values: {
      course_name: string
      teacher_id: number
      classroom?: string
      class_time?: string
      college_id?: number
    }) => {
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

  const delMut = useMutation({
    mutationFn: (id: number) => taskApi.remove(id),
    onSuccess: () => {
      message.success('删除成功')
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const restoreMut = useMutation({
    mutationFn: (id: number) => taskApi.restore(id),
    onSuccess: () => {
      message.success('恢复成功')
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
    {
      title: 'ID',
      dataIndex: 'id',
      width: 70,
      sorter: true,
      sortOrder: sortState.field === 'id' ? sortState.order : null,
    },
    { title: '课程', dataIndex: 'course_name' },
    { title: '教师', dataIndex: 'teacher_name', width: 110 },
    {
      title: '授课学院',
      dataIndex: 'course_college_name',
      width: 150,
      render: (v, r) => v || r.teacher_college_name || '-',
    },
    { title: '教室', dataIndex: 'classroom', width: 100, render: (v) => v || '-' },
    {
      title: '上课时间',
      dataIndex: 'class_time',
      width: 160,
      render: (v: string) => formatDate(v),
      sorter: true,
      sortOrder: sortState.field === 'class_time' ? sortState.order : null,
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
    {
      title: '参与人',
      dataIndex: 'participant_count',
      width: 90,
      render: (v: number | undefined, r: Task) => (
        <Tooltip title="含创建人；满员后普通评教人不可加入">
          <span>
            {v ?? 0}/{r.max_participants ?? 4}
          </span>
        </Tooltip>
      ),
    },
    {
      title: '督导已评',
      dataIndex: 'course_supervisor_evaluated',
      width: 90,
      // 字段缺省=无法统计（任务无学期归属或查询降级），呈现「-」而非「未评」
      render: (v: boolean | undefined) =>
        v === undefined ? '-' : <Tag color={v ? 'success' : 'default'}>{v ? '已评' : '未评'}</Tag>,
    },
    {
      title: '课程评教人数',
      dataIndex: 'course_evaluator_count',
      width: 110,
      render: (v: number | undefined) => v ?? '-',
    },
    { title: '创建人', dataIndex: 'create_by_name', width: 100, render: (v: string) => v || '-' },
    {
      title: '操作',
      width: 200,
      render: (_, record) =>
        params.deleted === 1 ? (
          <Space>
            {canRestoreTask(record) && (
              <Popconfirm
                title="确认恢复该任务？恢复后将重新出现在任务列表中。"
                onConfirm={() => restoreMut.mutate(record.id)}
              >
                <Button
                  size="small"
                  loading={restoreMut.isPending && restoreMut.variables === record.id}
                >
                  恢复
                </Button>
              </Popconfirm>
            )}
          </Space>
        ) : (
          <Space>
            <Button
              size="small"
              onClick={() => {
                setEditing(record)
                const opts = (record.teacher_teaching_colleges || []).map((c) => ({
                  value: c.college_id,
                  label: c.college_name,
                }))
                if (!opts.length && record.course_college_id != null) {
                  opts.push({
                    value: record.course_college_id,
                    label: record.course_college_name || '当前授课学院',
                  })
                }
                setTeacherCollegeOptions(opts)
                form.setFieldsValue({
                  course_name: record.course_name,
                  teacher_id: record.teacher_id,
                  classroom: record.classroom,
                  class_time: record.class_time ? dayjs(record.class_time) : undefined,
                  college_id: record.course_college_id ?? undefined,
                })
                setModalOpen(true)
              }}
            >
              编辑
            </Button>
            {canDeleteTask(record) && (
              <Popconfirm
                title={
                  (record.evaluation_count ?? 0) > 0
                    ? `删除后任务将从列表移除，可由具备恢复权限的管理员在“已删除”筛选中恢复；该任务下 ${record.evaluation_count} 条评教记录会保留。确认删除？`
                    : '删除后任务将从列表移除，可由具备恢复权限的管理员在“已删除”筛选中恢复。确认删除？'
                }
                onConfirm={() => delMut.mutate(record.id)}
              >
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
          value={params.status}
          disabled={params.deleted === 1}
          options={[
            { label: '待评', value: 1 },
            { label: '已评', value: 2 },
            { label: '已取消', value: 3 },
          ]}
          onChange={(v) =>
            setParams((p) => ({
              ...p,
              page: 1,
              // status 存入 params；清空时置 undefined 以生效 allowClear/X
              status: v ?? undefined,
            }))
          }
        />
        {(canRestoreAny || canRestoreOwn) && (
          <Select
            style={{ width: 130 }}
            value={params.deleted === 1 ? 1 : 0}
            options={[
              { label: '正常任务', value: 0 },
              { label: '已删除任务', value: 1 },
            ]}
            onChange={(v) => {
              if (v === 1) savedStatusRef.current = params.status
              setParams((p) =>
                v === 1
                  ? // 切到已删除视图：暂存并清空 status 筛选
                    { ...p, page: 1, deleted: 1, status: undefined }
                  : // 切回正常视图：回显切走前的 status
                    { ...p, page: 1, deleted: undefined, status: savedStatusRef.current }
              )
            }}
          />
        )}
        <Tooltip title={params.deleted === 1 ? '已删除视图不支持新建任务' : undefined}>
          <span>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              disabled={params.deleted === 1}
              onClick={() => {
                setEditing(null)
                setTeacherCollegeOptions([])
                form.resetFields()
                setModalOpen(true)
              }}
            >
              创建任务
            </Button>
          </span>
        </Tooltip>
        <Tooltip title={params.deleted === 1 ? '已删除视图不支持批量创建' : undefined}>
          <span>
            <Button
              icon={<PlusOutlined />}
              disabled={params.deleted === 1}
              onClick={() => {
                batchForm.resetFields()
                setBatchOpen(true)
              }}
            >
              批量创建
            </Button>
          </span>
        </Tooltip>
        <Tooltip title={params.deleted === 1 ? '已删除视图不支持导出' : undefined}>
          <span>
            <Button
              icon={<DownloadOutlined />}
              loading={exportMut.isPending}
              disabled={params.deleted === 1}
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
          </span>
        </Tooltip>
      </div>
      <Table<Task>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        onChange={(_p, _f, sorter, extra) => {
          // 仅排序触发时处理排序；换页由 pagination.onChange 单独处理，避免把页码重置回 1
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
            <TeacherSelect
              onSelect={handleTeacherSelect}
              initialOption={
                editing
                  ? {
                      id: editing.teacher_id,
                      username: editing.teacher_name,
                      user_no: '',
                      college_id: editing.teacher_college_id ?? null,
                      college_name: editing.teacher_college_name ?? null,
                    }
                  : undefined
              }
            />
          </Form.Item>
          <Form.Item
            name="college_id"
            label="授课学院"
            tooltip="任务展示与统计的学院口径；为空时按教师授课学院与你的学院范围自动解析"
          >
            <Select
              allowClear
              placeholder="默认自动解析"
              options={teacherCollegeOptions}
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
  const authCollegeId = useAuthStore((s) => s.user?.college_id ?? undefined)
  const [form] = Form.useForm()
  const [collegeOptions, setCollegeOptions] = useState<{ label: string; value: number }[]>([])

  const handleTeacherSelect = (u?: User) => {
    if (!u) {
      setCollegeOptions([])
      form.setFieldValue('college_id', undefined)
      return
    }
    const map = new Map<number, string>()
    if (u.college_id != null && u.college_name) map.set(u.college_id, u.college_name)
    for (const tc of u.teaching_colleges || []) map.set(tc.college_id, tc.college_name)
    const opts = Array.from(map, ([value, label]) => ({ value, label }))
    setCollegeOptions(opts)
    const preferred =
      opts.find((o) => o.value === authCollegeId)?.value ?? u.college_id ?? opts[0]?.value
    form.setFieldValue('college_id', preferred)
  }

  const batchMut = useMutation({
    mutationFn: (v: {
      teacher_id: number
      course_names: string
      classroom?: string
      college_id?: number
    }) => {
      const names = v.course_names
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean)
      return taskApi.batchCreate(
        names.map((course_name) => ({
          course_name,
          teacher_id: v.teacher_id,
          classroom: v.classroom,
          college_id: v.college_id,
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
          <TeacherSelect onSelect={handleTeacherSelect} />
        </Form.Item>
        <Form.Item
          name="college_id"
          label="授课学院"
          tooltip="为空时按教师授课学院与你的学院范围自动解析"
        >
          <Select allowClear placeholder="默认自动解析" options={collegeOptions} />
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
