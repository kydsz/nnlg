import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { Table, Tabs, Card, Col, Row, Statistic, Button, App, DatePicker } from 'antd'
import { DownloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { statsApi, type RecordQuery } from '@/api/modules/stats'
import ExportFieldSelector from '@/components/ExportFieldSelector'
import type { TeacherStat, CollegeStat, SupervisorStat, EvaluationRecord } from '@/api/types'
import { downloadBlob, exportFilename } from '@/utils/download'
import { formatDate } from '@/utils/format'

export default function Stats() {
  return (
    <div>
      <h3 className="page-title">统计分析</h3>
      <Tabs
        items={[
          { key: 'teachers', label: '教师统计', children: <TeacherStatsTab /> },
          { key: 'colleges', label: '学院统计', children: <CollegeStatsTab /> },
          { key: 'campus', label: '校区统计', children: <CampusStatsTab /> },
          { key: 'supervisors', label: '督导统计', children: <SupervisorStatsTab /> },
          { key: 'records', label: '听课明细', children: <RecordsTab /> },
          { key: 'unteached', label: '未被听课教师', children: <UnteachedTab /> },
        ]}
      />
    </div>
  )
}

function TeacherStatsTab() {
  const { message } = App.useApp()
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'teachers'],
    queryFn: () => statsApi.teachers({ page: 1, page_size: 100 }),
  })
  const exportMut = useMutation({
    mutationFn: () => statsApi.exportTeachers({ format: 'xlsx' }),
    onSuccess: (blob) => downloadBlob(blob, exportFilename('教师评教统计')),
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<TeacherStat> = [
    { title: '教师', dataIndex: 'teacher_name', width: 120 },
    { title: '学院', dataIndex: 'college_name' },
    { title: '总任务', dataIndex: 'total_tasks', width: 90 },
    { title: '已评', dataIndex: 'evaluated_tasks', width: 80 },
    { title: '待评', dataIndex: 'pending_tasks', width: 80 },
    { title: '评教记录数', dataIndex: 'total_evaluations', width: 100 },
    { title: '平均分', dataIndex: 'average_score', width: 90 },
    { title: '完成率(%)', dataIndex: 'evaluation_rate', width: 100 },
  ]

  return (
    <div>
      <Button
        icon={<DownloadOutlined />}
        loading={exportMut.isPending}
        style={{ marginBottom: 12 }}
        onClick={() => exportMut.mutate()}
      >
        导出 xlsx
      </Button>
      <Table<TeacherStat>
        rowKey="teacher_id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={{
          total: data?.total,
          showSizeChanger: true,
        }}
      />
    </div>
  )
}

function CollegeStatsTab() {
  const { message } = App.useApp()
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'colleges'],
    queryFn: () => statsApi.colleges({ page: 1, page_size: 100 }),
  })
  const exportMut = useMutation({
    mutationFn: () => statsApi.exportColleges({ format: 'xlsx' }),
    onSuccess: (blob) => downloadBlob(blob, exportFilename('学院评教统计')),
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<CollegeStat> = [
    { title: '学院', dataIndex: 'college_name' },
    { title: '教师数', dataIndex: 'teacher_count', width: 90 },
    { title: '总任务', dataIndex: 'total_tasks', width: 90 },
    { title: '已评', dataIndex: 'evaluated_tasks', width: 80 },
    { title: '待评', dataIndex: 'pending_tasks', width: 80 },
    { title: '评教记录数', dataIndex: 'total_evaluations', width: 100 },
    { title: '完成率(%)', dataIndex: 'evaluation_rate', width: 100 },
  ]

  return (
    <div>
      <Button
        icon={<DownloadOutlined />}
        loading={exportMut.isPending}
        style={{ marginBottom: 12 }}
        onClick={() => exportMut.mutate()}
      >
        导出 xlsx
      </Button>
      <Table<CollegeStat>
        rowKey="college_id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={false}
      />
    </div>
  )
}

function CampusStatsTab() {
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'campus'],
    queryFn: () => statsApi.campus(),
  })
  const rows = (Array.isArray(data) ? data : []) as Record<string, unknown>[]
  return (
    <Table<Record<string, unknown>>
      rowKey={(r) => String(r.campus_id ?? r.id)}
      loading={isLoading}
      size="small"
      pagination={false}
      dataSource={rows}
      columns={[
        { title: '校区', dataIndex: 'campus_name' },
        { title: '教师数', dataIndex: 'teacher_count', width: 90 },
        { title: '总任务', dataIndex: 'total_tasks', width: 90 },
        { title: '已评', dataIndex: 'evaluated_tasks', width: 80 },
        { title: '评教记录数', dataIndex: 'total_evaluations', width: 100 },
        { title: '完成率(%)', dataIndex: 'evaluation_rate', width: 100 },
      ]}
    />
  )
}

function UnteachedTab() {
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'unteached'],
    queryFn: () => statsApi.unteachedTeachers({ page: 1, page_size: 100 }),
  })
  const rows = (data?.list || []) as {
    id: number
    user_no?: string
    username: string
    college_name?: string
    research_room_name?: string
  }[]
  return (
    <Table<(typeof rows)[number]>
      rowKey="id"
      loading={isLoading}
      size="small"
      pagination={false}
      dataSource={rows}
      columns={[
        { title: '工号', dataIndex: 'user_no', width: 110 },
        { title: '姓名', dataIndex: 'username', width: 120 },
        { title: '学院', dataIndex: 'college_name', width: 200, ellipsis: true },
        { title: '教研室', dataIndex: 'research_room_name', width: 300, ellipsis: true },
      ]}
    />
  )
}

function SupervisorStatsTab() {
  const { message } = App.useApp()
  const [dates, setDates] = useState<[string?, string?]>([])
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'supervisors', dates],
    queryFn: () =>
      statsApi.supervisors({ page: 1, page_size: 100, start_date: dates[0], end_date: dates[1] }),
  })
  const exportMut = useMutation({
    mutationFn: () =>
      statsApi.exportSupervisors({ format: 'xlsx', start_date: dates[0], end_date: dates[1] }),
    onSuccess: (blob) => downloadBlob(blob, exportFilename('督导评教统计')),
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<SupervisorStat> = [
    { title: '督导', dataIndex: 'supervisor_name', width: 120 },
    { title: '工号', dataIndex: 'user_no', width: 110 },
    { title: '学院', dataIndex: 'college_name' },
    { title: '评教次数', dataIndex: 'total_evaluations', width: 100 },
    { title: '平均分', dataIndex: 'average_score', width: 90 },
  ]

  return (
    <div>
      <div className="filter-bar">
        <DatePicker.RangePicker
          onChange={(v) =>
            setDates(
              v ? [v[0]!.format('YYYY-MM-DD'), v[1]!.format('YYYY-MM-DD')] : []
            )
          }
        />
        <Button
          icon={<DownloadOutlined />}
          loading={exportMut.isPending}
          onClick={() => exportMut.mutate()}
        >
          导出 xlsx
        </Button>
      </div>
      <Table<SupervisorStat>
        rowKey="supervisor_id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={false}
      />
    </div>
  )
}

function RecordsTab() {
  const { message } = App.useApp()
  const [exportOpen, setExportOpen] = useState(false)
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'evaluation-records'],
    queryFn: () => statsApi.evaluationRecords({ page: 1, page_size: 20 }),
  })
  const exportMut = useMutation({
    mutationFn: (fields: string[]) =>
      statsApi.exportEvaluationRecords({ fields }),
    onSuccess: (blob) => downloadBlob(blob, exportFilename('听课明细')),
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<EvaluationRecord> = [
    { title: '教师', dataIndex: 'teacher_name', width: 100 },
    { title: '课程', dataIndex: 'course_name' },
    { title: '上课时间', dataIndex: 'class_time', width: 150, render: (v) => formatDate(v) },
    { title: '教室', dataIndex: 'classroom', width: 90 },
    { title: '学院', dataIndex: 'college_name', width: 130 },
    { title: '评教人', dataIndex: 'evaluator_name', width: 100 },
    { title: '角色', dataIndex: 'evaluator_role', width: 100 },
    { title: '总分', dataIndex: 'total_score', width: 80 },
    { title: '提交时间', dataIndex: 'submit_time', width: 150, render: (v) => formatDate(v) },
  ]

  return (
    <div>
      <Button
        icon={<DownloadOutlined />}
        style={{ marginBottom: 12 }}
        onClick={() => setExportOpen(true)}
      >
        选择字段导出
      </Button>
      <Table<EvaluationRecord>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        pagination={{
          total: data?.total,
          showSizeChanger: true,
        }}
      />
      <ExportFieldSelector
        open={exportOpen}
        onCancel={() => setExportOpen(false)}
        onConfirm={(fields) => {
          setExportOpen(false)
          exportMut.mutate(fields)
        }}
      />
    </div>
  )
}

// Overview 卡片（嵌入 Dashboard 之外的快捷概览）
export function StatsOverviewCards() {
  const { data } = useQuery({ queryKey: ['stats', 'overview'], queryFn: () => statsApi.overview() })
  if (!data) return null
  return (
    <Card size="small">
      <Row gutter={16}>
        <Col span={6}>
          <Statistic title="教师" value={data.users.teachers} />
        </Col>
        <Col span={6}>
          <Statistic title="任务" value={data.tasks.total} />
        </Col>
        <Col span={6}>
          <Statistic title="评教记录" value={data.evaluations.total} />
        </Col>
        <Col span={6}>
          <Statistic title="完成率" value={data.tasks.evaluation_rate} suffix="%" />
        </Col>
      </Row>
    </Card>
  )
}
