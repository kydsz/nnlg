import { useEffect, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Card,
  Button,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  App,
  DatePicker,
  Tag,
  Checkbox,
  Popconfirm,
} from 'antd'
import type { GetProp } from 'antd'
import { PlusOutlined, DeleteOutlined, LoadingOutlined } from '@ant-design/icons'
import { scheduleApi } from '@/api/modules/schedule'
import { orgApi } from '@/api/modules/org'
import ScheduleGrid from '@/components/ScheduleGrid'
import { formatSemester, generateSemesters } from '@/utils/format'
import { useDebounce } from '@/hooks/useDebounce'
import type { SemesterConfig, User } from '@/api/types'

export default function CourseSchedule() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [selectedSemester, setSelectedSemester] = useState<string | undefined>(undefined)
  const [selectedTeacher, setSelectedTeacher] = useState<number | undefined>(undefined)
  const [selectedCollege, setSelectedCollege] = useState<number | undefined>(undefined)

  const { data: configs } = useQuery({
    queryKey: ['semester-configs'],
    queryFn: () => scheduleApi.semesterConfigs(),
  })
  const { data: dynamicSemesters } = useQuery({
    queryKey: ['semesters'],
    queryFn: () => scheduleApi.semesters(),
  })
  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
  })

  const effectiveSemester =
    selectedSemester ||
    configs?.find((c) => c.is_current)?.semester ||
    dynamicSemesters?.[0]

  // 课表数据
  const { data: scheduleData, isLoading: scheduleLoading } = useQuery({
    queryKey: ['schedule', 'teacher', selectedTeacher, effectiveSemester],
    queryFn: () => scheduleApi.byTeacher(selectedTeacher!, effectiveSemester),
    enabled: !!selectedTeacher && !!effectiveSemester,
  })

  const invalidateConfigs = () => {
    qc.invalidateQueries({ queryKey: ['semester-configs'] })
    qc.invalidateQueries({ queryKey: ['current-semester'] })
  }

  const updateMut = useMutation({
    mutationFn: ({ semester, data }: { semester: string; data: { start_date?: string; weeks?: number; is_current?: boolean } }) =>
      scheduleApi.updateSemesterConfig(semester, data),
    onSuccess: () => {
      message.success('保存成功')
      invalidateConfigs()
    },
    onError: (e) => message.error(e.message),
  })

  const createMut = useMutation({
    mutationFn: (values: { semester: string; start_date: string; weeks?: number; is_current?: boolean }) =>
      scheduleApi.createSemesterConfig(values),
    onSuccess: () => {
      message.success('学期配置已创建')
      invalidateConfigs()
    },
    onError: (e) => message.error(e.message),
  })

  const deleteMut = useMutation({
    mutationFn: (semester: string) => scheduleApi.deleteSemesterConfig(semester),
    onSuccess: () => {
      message.success('已删除学期配置')
      invalidateConfigs()
    },
    onError: (e) => message.error(e.message),
  })

  const configList: SemesterConfig[] = Array.isArray(configs)
    ? configs
    : (configs as { list?: SemesterConfig[] } | undefined)?.list || []

  const semesterOptions = Array.from(
    new Set([...(dynamicSemesters || []), ...configList.map((c) => c.semester)])
  )
    .sort()
    .reverse()
    .map((s) => ({ label: formatSemester(s), value: s }))

  return (
    <div>
      <h3 className="page-title">学期配置与课表查询</h3>

      {/* 学期配置 */}
      <Card size="small" title="学期配置" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 12 }}>
          {configList.map((c) => (
            <Card key={c.id} size="small" style={{ width: 250 }}>
              <Space direction="vertical" size={4} style={{ width: '100%' }}>
                <Space>
                  <strong>{formatSemester(c.semester)}</strong>
                  {c.is_current && <Tag color="green">当前学期</Tag>}
                </Space>
                <Space wrap>
                  <Input
                    size="small"
                    type="date"
                    defaultValue={c.start_date?.slice(0, 10)}
                    style={{ width: 150 }}
                    onBlur={(e) => {
                      if (e.target.value && e.target.value !== c.start_date?.slice(0, 10)) {
                        updateMut.mutate({
                          semester: c.semester,
                          data: { start_date: e.target.value },
                        })
                      }
                    }}
                  />
                  <span style={{ color: '#666', fontSize: 12 }}>周数</span>
                  <InputNumber
                    size="small"
                    min={1}
                    max={52}
                    step={1}
                    defaultValue={c.weeks || 20}
                    style={{ width: 76 }}
                    onBlur={(e) => {
                      const v = Number((e.target as HTMLInputElement).value)
                      const old = c.weeks || 20
                      if (Number.isInteger(v) && v >= 1 && v <= 52 && v !== old) {
                        updateMut.mutate({ semester: c.semester, data: { weeks: v } })
                      }
                    }}
                  />
                  {!c.is_current && (
                    <Button
                      size="small"
                      onClick={() =>
                        updateMut.mutate({ semester: c.semester, data: { is_current: true } })
                      }
                    >
                      设为当前
                    </Button>
                  )}
                  <Popconfirm
                    title="删除学期配置"
                    description={`确定删除 ${formatSemester(c.semester)} 的学期配置吗？`}
                    okText="删除"
                    cancelText="取消"
                    okButtonProps={{ danger: true }}
                    onConfirm={() => deleteMut.mutate(c.semester)}
                  >
                    <Button size="small" danger icon={<DeleteOutlined />}>
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              </Space>
            </Card>
          ))}
        </div>
        <CreateSemesterForm
          existing={configList.map((c) => c.semester)}
          onCreate={(v) => createMut.mutate(v)}
          loading={createMut.isPending}
        />
      </Card>

      {/* 课表查看 */}
      <Card size="small" title="课表查询">
        <div className="filter-bar">
          <Select
            showSearch
            optionFilterProp="label"
            placeholder="选择学期"
            style={{ width: 220 }}
            options={semesterOptions}
            value={effectiveSemester}
            onChange={setSelectedSemester}
          />
          <Select
            allowClear
            placeholder="按学院筛选教师"
            style={{ width: 200 }}
            options={(colleges?.list || []).map((c) => ({ label: c.name, value: c.id }))}
            value={selectedCollege}
            onChange={(v) => {
              setSelectedCollege(v)
              setSelectedTeacher(undefined)
            }}
          />
          <TeacherSelect
            collegeId={selectedCollege}
            value={selectedTeacher}
            onChange={setSelectedTeacher}
          />
        </div>
        {selectedTeacher ? (
          <ScheduleGrid
            courses={scheduleData?.details}
            meta={scheduleData}
            loading={scheduleLoading}
          />
        ) : (
          <div style={{ color: '#999', textAlign: 'center', padding: 40 }}>
            请选择教师查看课表
          </div>
        )}
      </Card>
    </div>
  )
}

/**
 * 教师选择器：远程搜索 + 学院筛选 + 下拉滚动分页动态加载
 * 选项展示 姓名（工号 · 学院），方便区分重名教师
 */
function TeacherSelect({
  collegeId,
  value,
  onChange,
}: {
  collegeId?: number
  value?: number
  onChange: (v: number | undefined) => void
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
      const res = await scheduleApi.teachers({
        page: p,
        page_size: PAGE_SIZE,
        keyword: kw || undefined,
        college_id: collegeId,
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
  }, [debouncedKeyword, collegeId])

  // 外部清空选中（如切换学院）时同步清除已选中教师信息
  useEffect(() => {
    if (value == null) setSelected(null)
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
      style={{ width: 280 }}
      value={value}
      loading={loading}
      onSearch={setKeyword}
      onPopupScroll={handlePopupScroll}
      onChange={(v) => {
        // 选中/清空后重置搜索词，下次打开恢复完整列表的原始顺序
        setKeyword('')
        if (v == null) {
          setSelected(null)
          onChange(undefined)
          return
        }
        const u = mergedOptions.find((o) => o.id === v)
        setSelected(u || null)
        onChange(v as number)
      }}
      notFoundContent={loading ? <LoadingOutlined spin /> : '暂无教师'}
      options={mergedOptions.map((t) => ({
        value: t.id,
        label: t.username,
        desc: [t.user_no, t.college_name].filter(Boolean).join(' · '),
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

function CreateSemesterForm({
  existing,
  onCreate,
  loading,
}: {
  existing: string[]
  onCreate: (v: { semester: string; start_date: string; weeks?: number; is_current?: boolean }) => void
  loading: boolean
}) {
  const [form] = Form.useForm()
  const yearSemesters = generateSemesters()

  return (
    <Form
      form={form}
      layout="inline"
      onFinish={(v) => {
        onCreate({
          semester: v.semester,
          start_date: v.start_date.format('YYYY-MM-DD'),
          weeks: v.weeks,
          is_current: !!v.is_current,
        })
        form.resetFields()
      }}
    >
      <Form.Item
        name="semester"
        rules={[{ required: true, message: '选择学期' }]}
      >
        <Select
          placeholder="选择学期"
          style={{ width: 220 }}
          options={yearSemesters
            .filter((s) => !existing.includes(s))
            .map((s) => ({ label: formatSemester(s), value: s }))}
        />
      </Form.Item>
      <Form.Item
        name="start_date"
        rules={[{ required: true, message: '选择开学日期' }]}
      >
        <DatePicker placeholder="开学日期" />
      </Form.Item>
      <Form.Item
        name="weeks"
        label="周数"
        initialValue={20}
        rules={[{ required: true, message: '填写周数' }]}
      >
        <InputNumber min={1} max={52} style={{ width: 90 }} />
      </Form.Item>
      <Form.Item name="is_current" valuePropName="checked">
        <Checkbox>设为当前学期</Checkbox>
      </Form.Item>
      <Form.Item>
        <Button type="primary" htmlType="submit" icon={<PlusOutlined />} loading={loading}>
          新增学期配置
        </Button>
      </Form.Item>
    </Form>
  )
}
