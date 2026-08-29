import { useEffect, useRef, useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  Card,
  Button,
  Input,
  Select,
  DatePicker,
  Space,
  Tag,
  Tabs,
  List,
  App,
  Spin,
  Empty,
} from 'antd'
import { DownloadOutlined, SearchOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { statsApi, type RecordQuery } from '@/api/modules/stats'
import { orgApi } from '@/api/modules/org'
import { scheduleApi } from '@/api/modules/schedule'
import ExportFieldSelector from '@/components/ExportFieldSelector'
import { useDebounce } from '@/hooks/useDebounce'
import { downloadBlob, exportFilename } from '@/utils/download'
import { roleNamesStr } from '@/utils/roleNames'
import { formatDate } from '@/utils/format'
import type { TeacherEvaluationSummary, EvaluationRecord } from '@/api/types'

const ROLE_OPTIONS = [
  { label: '教师', value: 'teacher' },
  { label: '督导', value: 'supervisor' },
  { label: '校级督导', value: 'school_supervisor' },
  { label: '院级督导', value: 'college_supervisor' },
  { label: '学院管理员', value: 'college_admin' },
]

const SORT_FIELDS = [
  { label: '评教次数', value: 'given_count' },
  { label: '评教均分', value: 'given_avg_score' },
  { label: '被评次数', value: 'received_count' },
  { label: '被评均分', value: 'received_avg_score' },
  { label: '评教率', value: 'evaluation_rate' },
]

export default function TeacherEvaluationSummary() {
  const { message } = App.useApp()
  const [collegeIds, setCollegeIds] = useState<string[]>([])
  const [roles, setRoles] = useState<string[]>([])
  const [hasSchedule, setHasSchedule] = useState<boolean | undefined>(undefined)
  const [dates, setDates] = useState<[string?, string?]>([])
  const datesInitRef = useRef(false)
  const [sortBy, setSortBy] = useState<string | undefined>()
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')
  const [search, setSearch] = useState('')
  const keyword = useDebounce(search, 300)
  const [page, setPage] = useState(1)
  const [exportOpen, setExportOpen] = useState(false)

  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
  })
  const { data: currentSemester } = useQuery({
    queryKey: ['current-semester'],
    queryFn: () => scheduleApi.currentSemester(),
  })

  // 默认日期区间：当前学期开学日 ~ 今天
  useEffect(() => {
    if (!datesInitRef.current && currentSemester?.start_date) {
      datesInitRef.current = true
      const start = dayjs(currentSemester.start_date)
      setDates([start.format('YYYY-MM-DD'), dayjs().format('YYYY-MM-DD')])
    }
  }, [currentSemester]) // eslint-disable-line react-hooks/exhaustive-deps

  const params: RecordQuery = {
    page,
    page_size: 20,
    college_ids: collegeIds.join(','),
    roles: roles.join(','),
    has_schedule: hasSchedule,
    start_date: dates[0],
    end_date: dates[1],
    sort_by: sortBy,
    sort_order: sortOrder,
    keyword: keyword || undefined,
  }

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['teacher-summary', params],
    queryFn: () => statsApi.teacherSummary(params),
  })

  useEffect(() => setPage(1), [collegeIds, roles, hasSchedule, dates, sortBy, sortOrder, keyword])

  const exportMut = useMutation({
    mutationFn: (fields: string[]) =>
      statsApi.exportTeacherSummary({ ...params, fields, format: 'xlsx' }),
    onSuccess: (blob) => downloadBlob(blob, exportFilename('教师评教汇总')),
    onError: (e) => message.error(e.message),
  })

  const list = data?.list || []
  const total = data?.total || 0

  return (
    <div>
      <h3 className="page-title">教师评教汇总</h3>

      {/* 筛选区 */}
      <Card size="small" style={{ marginBottom: 16 }}>
        <div className="filter-bar">
          <Select
            mode="multiple"
            allowClear
            maxTagCount={2}
            placeholder="学院"
            style={{ minWidth: 200 }}
            options={(colleges?.list || []).map((c) => ({ label: c.name, value: String(c.id) }))}
            value={collegeIds}
            onChange={setCollegeIds}
          />
          <Select
            mode="multiple"
            allowClear
            maxTagCount={2}
            placeholder="角色"
            style={{ minWidth: 160 }}
            options={ROLE_OPTIONS}
            value={roles}
            onChange={setRoles}
          />
          <Select
            allowClear
            placeholder="是否有课"
            style={{ width: 120 }}
            options={[
              { label: '有课', value: 'yes' },
              { label: '无课', value: 'no' },
            ]}
            onChange={(v) => setHasSchedule(v === 'yes' ? true : v === 'no' ? false : undefined)}
          />
          <DatePicker.RangePicker
            onChange={(v) =>
              setDates(v ? [v[0]!.format('YYYY-MM-DD'), v[1]!.format('YYYY-MM-DD')] : [])
            }
          />
          <Select
            allowClear
            placeholder="排序字段"
            style={{ width: 130 }}
            options={SORT_FIELDS}
            value={sortBy}
            onChange={setSortBy}
          />
          <Select
            style={{ width: 100 }}
            value={sortOrder}
            onChange={setSortOrder}
            options={[
              { label: '降序', value: 'desc' },
              { label: '升序', value: 'asc' },
            ]}
          />
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder="教师姓名/工号"
            style={{ width: 180 }}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Button
            icon={<DownloadOutlined />}
            loading={exportMut.isPending}
            onClick={() => setExportOpen(true)}
          >
            导出
          </Button>
        </div>
      </Card>

      {/* 列表 */}
      <Spin spinning={isLoading || isFetching}>
        {list.length === 0 && !isLoading ? (
          <Empty description="暂无数据" />
        ) : (
          <List
            dataSource={list}
            pagination={{
              current: page,
              pageSize: 20,
              total,
              onChange: setPage,
              showTotal: (t) => `共 ${t} 条`,
            }}
            renderItem={(item) => <TeacherCard item={item} dates={dates} />}
          />
        )}
      </Spin>

      <ExportFieldSelector
        open={exportOpen}
        defaultFields={[
          'teacher_name',
          'user_no',
          'college_name',
          'role_names',
          'given_count',
          'given_avg_score',
          'received_count',
          'received_avg_score',
          'task_count',
          'evaluation_rate',
        ]}
        onCancel={() => setExportOpen(false)}
        onConfirm={(fields) => {
          setExportOpen(false)
          exportMut.mutate(fields)
        }}
      />
    </div>
  )
}

function TeacherCard({
  item,
  dates,
}: {
  item: TeacherEvaluationSummary
  dates: [string?, string?]
}) {
  const [active, setActive] = useState<string | null>(null)

  return (
    <Card size="small" style={{ marginBottom: 8 }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 8,
          cursor: 'pointer',
        }}
        onClick={() => setActive(active ? null : 'given')}
      >
        <Space>
          <strong>{item.teacher_name}</strong>
          <span style={{ color: '#999' }}>{item.user_no}</span>
          <span>{item.college_name}</span>
          <Tag>{roleNamesStr(item.role_names)}</Tag>
        </Space>
        <Space size={16}>
          <span>评教 {item.given_count ?? 0} 次</span>
          <span>均分 {item.given_avg_score ?? '-'}</span>
          <span>被评 {item.received_count ?? 0} 次</span>
          <span>均分 {item.received_avg_score ?? '-'}</span>
          <span>评教率 {item.evaluation_rate ?? '-'}%</span>
        </Space>
      </div>
      {active && (
        <Tabs
          activeKey={active}
          onChange={setActive}
          items={[
            {
              key: 'given',
              label: '评教记录',
              children: <RecordsInner evaluatorId={item.teacher_id} dates={dates} />,
            },
            {
              key: 'received',
              label: '被评教记录',
              children: <RecordsInner teacherId={item.teacher_id} dates={dates} />,
            },
          ]}
        />
      )}
    </Card>
  )
}

function RecordsInner({
  evaluatorId,
  teacherId,
  dates,
}: {
  evaluatorId?: number
  teacherId?: number
  dates: [string?, string?]
}) {
  const { data, isLoading } = useQuery({
    queryKey: ['summary-records', evaluatorId, teacherId, dates],
    queryFn: () =>
      statsApi.evaluationRecords({
        page: 1,
        page_size: 20,
        evaluator_id: evaluatorId,
        teacher_id: teacherId,
        start_date: dates[0],
        end_date: dates[1],
      }),
    enabled: !!evaluatorId || !!teacherId,
  })

  if (isLoading) return <Spin size="small" style={{ display: 'block', margin: 12 }} />
  const records = data?.list || []
  if (records.length === 0) return <Empty description="暂无记录" imageStyle={{ height: 40 }} />

  return (
    <List
      size="small"
      dataSource={records}
      renderItem={(r: EvaluationRecord) => (
        <List.Item>
          <Space size={12} wrap>
            <span>{r.course_name}</span>
            <span>{r.teacher_name}</span>
            <span style={{ color: '#999' }}>{formatDate(r.submit_time)}</span>
            <span>
              总分 {r.total_score ?? '-'} / {r.max_total_score ?? '-'}
            </span>
          </Space>
        </List.Item>
      )}
    />
  )
}
