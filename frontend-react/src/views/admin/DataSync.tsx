import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Card,
  Button,
  Form,
  Input,
  Select,
  Space,
  App,
  Alert,
  Modal,
  Progress,
  Badge,
  Popconfirm,
  Table,
  Tooltip,
  Tag,
} from 'antd'
import {
  SyncOutlined,
  ScanOutlined,
  UserOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import { syncApi } from '@/api/modules/sync'
import { scheduleApi } from '@/api/modules/schedule'
import { userApi } from '@/api/modules/users'
import { orgApi } from '@/api/modules/org'
import { formatSemester } from '@/utils/format'

export default function DataSync() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [crawlOpen, setCrawlOpen] = useState(false)
  const [llsykbOpen, setLlsykbOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)

  const { data: semesters } = useQuery({
    queryKey: ['semesters'],
    queryFn: () => scheduleApi.semesters(),
  })
  const { data: currentSemester } = useQuery({
    queryKey: ['current-semester'],
    queryFn: () => scheduleApi.currentSemester(),
  })
  const { data: syncStatus } = useQuery({
    queryKey: ['sync-status'],
    queryFn: () => syncApi.teachersSyncStatus(),
  })

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['sync-status'] })
    qc.invalidateQueries({ queryKey: ['semesters'] })
    qc.invalidateQueries({ queryKey: ['colleges'] })
    qc.invalidateQueries({ queryKey: ['users'] })
  }

  const mkSync = (name: string) => ({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      message.success(`${name}完成`)
      addSyncHistory(name, 'success', '同步完成')
      invalidate()
    },
    onError: (e: Error) => {
      message.error(`${name}失败：${e.message}`)
      addSyncHistory(name, 'failed', e.message)
    },
  })

  const allMut = useMutation({ ...mkSync('全量同步'), mutationFn: () => syncApi.syncAll(currentSemester?.semester) })
  const unitsTeachersMut = useMutation({ ...mkSync('同步单位和教师'), mutationFn: () => syncApi.syncUnitsAndTeachers() })
  const collegesMut = useMutation({ ...mkSync('同步单位信息'), mutationFn: () => syncApi.syncColleges() })
  const teachersMut = useMutation({ ...mkSync('同步教师'), mutationFn: () => syncApi.syncTeachers() })
  const scheduleMut = useMutation({
    ...mkSync('同步课表'),
    mutationFn: () => syncApi.syncCourseSchedule(currentSemester?.semester),
  })

  // 无效用户扫描
  const [invalidResult, setInvalidResult] = useState<Record<string, unknown> | null>(null)
  const scanMut = useMutation({
    mutationFn: () => syncApi.scanInvalidUsers(),
    onSuccess: (res) => {
      setInvalidResult(res)
      message.success('扫描完成')
    },
    onError: (e) => message.error(e.message),
  })

  const semesterOptions = (semesters || []).map((s) => ({
    label: formatSemester(s),
    value: s,
  }))

  // 同步历史（localStorage 最近 10 条）
  const [history, setHistory] = useState<{ time: string; action: string; status: string; message: string }[]>(
    () => {
      try {
        return JSON.parse(localStorage.getItem('sync_history') || '[]')
      } catch {
        return []
      }
    }
  )
  function addSyncHistory(action: string, status: string, msg: string) {
    setHistory((prev) => {
      const next = [{ time: new Date().toLocaleString('zh-CN'), action, status, message: msg }, ...prev].slice(0, 10)
      localStorage.setItem('sync_history', JSON.stringify(next))
      return next
    })
  }

  return (
    <div>
      <h3 className="page-title">数据同步</h3>

      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message={
          currentSemester
            ? `当前学期：${formatSemester(currentSemester.semester)}`
            : '未设置当前学期，请先在课表管理中配置'
        }
      />

      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message="课表同步入口说明"
        description={
          <ul style={{ margin: 0, paddingLeft: 18 }}>
            <li>
              <Tag color="green">推荐</Tag>「按学院批量同步课表」—— 按学院逐个老师同步，没课的老师也能同步，带进度条，适合整批刷新。
            </li>
            <li>
              <Tag color="green">推荐</Tag>「选择教师同步课表」—— 只同步你勾选的老师，同样带进度条，适合单独更新某几位老师。
            </li>
            <li>
              <Tag color="green">推荐</Tag>「同步单位+教师」等—— 同步院系/单位、教师档案，是基础数据，一般优先同步。
            </li>
            <li>
              <Tag color="orange">旧方式</Tag>「同步课表」「教务系统抓取课表」—— 整页抓取按姓名匹配，易漏没课老师、无进度，且同名教师会匹配错，建议用推荐的方式代替。
            </li>
            <li>
              「一键全量同步」「工号异常清理」—— 前者是整套全量刷新（含旧方式课表），后者用于定位异常工号。
            </li>
          </ul>
        }
      />

      <Card title="同步操作" style={{ marginBottom: 16 }}>
        <Space wrap>
          <Tooltip title="推荐：按学院逐个老师同步，覆盖没课老师，带进度条，适合整批刷新">
            <Button
              type="primary"
              icon={<TeamOutlined />}
              onClick={() => setBatchOpen(true)}
            >
              按学院批量同步课表（推荐）
            </Button>
          </Tooltip>
          <Tooltip title="推荐：只同步你选中的老师，后台运行并显示进度条，适合单独更新某几位老师">
            <Button type="primary" icon={<UserOutlined />} onClick={() => setLlsykbOpen(true)}>
              选择教师同步课表（推荐）
            </Button>
          </Tooltip>
          <Popconfirm title="确认同步单位+教师？" onConfirm={() => unitsTeachersMut.mutate()}>
            <Button type="primary" loading={unitsTeachersMut.isPending}>
              同步单位+教师
            </Button>
          </Popconfirm>
          <Popconfirm title="确认仅同步单位？" onConfirm={() => collegesMut.mutate()}>
            <Button type="primary" loading={collegesMut.isPending}>
              仅同步单位
            </Button>
          </Popconfirm>
          <Popconfirm title="确认仅同步教师？" onConfirm={() => teachersMut.mutate()}>
            <Button type="primary" loading={teachersMut.isPending}>
              仅同步教师
            </Button>
          </Popconfirm>
          <Tooltip title="旧方式：整页抓取按姓名匹配，易漏没课老师、无进度，同名教师易匹配错，建议用推荐的方式代替">
            <Popconfirm
              title="确认同步课表？"
              description="旧方式同步，易漏没课老师且同名教师易匹配错，建议改用「按学院批量同步课表（推荐）」"
              onConfirm={() => scheduleMut.mutate()}
            >
              <Button loading={scheduleMut.isPending}>同步课表</Button>
            </Popconfirm>
          </Tooltip>
          <Tooltip title="旧方式：整页抓取按姓名匹配，易漏没课老师、无进度，同名教师易匹配错，建议用推荐的方式代替">
            <Button icon={<SyncOutlined />} onClick={() => setCrawlOpen(true)}>
              教务系统抓取课表
            </Button>
          </Tooltip>
          <Popconfirm title="确认一键全量同步？" description="将同步单位、教师档案和课表，耗时较长" onConfirm={() => allMut.mutate()}>
            <Button icon={<SyncOutlined />} loading={allMut.isPending}>
              一键全量同步
            </Button>
          </Popconfirm>
          <Badge count={invalidResult ? Number((invalidResult as { count?: number }).count || 0) : 0} size="small">
            <Button icon={<ScanOutlined />} loading={scanMut.isPending} onClick={() => scanMut.mutate()}>
              工号异常清理
            </Button>
          </Badge>
        </Space>
      </Card>

      {syncStatus != null && (
        <Card title="上次同步状态" size="small" style={{ marginBottom: 16 }}>
          <pre style={{ margin: 0, fontSize: 12 }}>
            {JSON.stringify(syncStatus, null, 2)}
          </pre>
        </Card>
      )}

      {history.length > 0 && (
        <Card
          title="同步历史"
          size="small"
          style={{ marginBottom: 16 }}
          extra={
            <Button
              size="small"
              onClick={() => {
                localStorage.removeItem('sync_history')
                setHistory([])
              }}
            >
              清空
            </Button>
          }
        >
          <Table
            rowKey={(_, i) => String(i)}
            size="small"
            pagination={false}
            dataSource={history}
            columns={[
              { title: '时间', dataIndex: 'time', width: 170 },
              { title: '操作', dataIndex: 'action', width: 140 },
              {
                title: '状态',
                dataIndex: 'status',
                width: 80,
                render: (v: string) => (v === 'success' ? '成功' : '失败'),
              },
              { title: '信息', dataIndex: 'message', ellipsis: true },
            ]}
          />
        </Card>
      )}

      {invalidResult && (
        <InvalidUsersPanel
          result={invalidResult}
          onCleaned={() => {
            setInvalidResult(null)
            invalidate()
          }}
        />
      )}

      <CrawlModal open={crawlOpen} onClose={() => setCrawlOpen(false)} />
      <LlsykbSelectModal open={llsykbOpen} onClose={() => setLlsykbOpen(false)} onRecord={addSyncHistory} />
      <LlsykbBatchModal open={batchOpen} onClose={() => setBatchOpen(false)} onRecord={addSyncHistory} />
    </div>
  )
}

/** 工号异常清理面板：左滑式列表 → 每行"删除"按钮 */
function InvalidUsersPanel({
  result,
  onCleaned,
}: {
  result: Record<string, unknown>
  onCleaned: () => void
}) {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const records =
    ((result.records || result.users || result.list) as
      | { id: number; user_no: string; username: string; reason?: string }[]
      | undefined) || []

  const cleanMut = useMutation({
    mutationFn: (userId: number) => syncApi.cleanupInvalidUser(userId),
    onSuccess: () => {
      message.success('已清理')
      qc.invalidateQueries({ queryKey: ['sync-status'] })
      if (records.length <= 1) onCleaned()
    },
    onError: (e) => message.error(e.message),
  })

  return (
    <Card title="工号异常用户（逐个清理）" size="small" style={{ marginBottom: 16 }}>
      {records.length === 0 ? (
        <div style={{ color: '#999' }}>无异常用户</div>
      ) : (
        <Table
          rowKey="id"
          size="small"
          pagination={false}
          dataSource={records}
          columns={[
            { title: '工号', dataIndex: 'user_no', width: 120 },
            { title: '姓名', dataIndex: 'username', width: 120 },
            { title: '原因', dataIndex: 'reason' },
            {
              title: '操作',
              width: 90,
              render: (_, r) => (
                <Popconfirm title="确认从系统清理该用户？" onConfirm={() => cleanMut.mutate(r.id)}>
                  <Button size="small" danger loading={cleanMut.isPending}>
                    清理
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      )}
    </Card>
  )
}

/** 选择教师同步课表：选中教师 → 异步后台任务 + 3 秒轮询进度 */
function LlsykbSelectModal({
  open,
  onClose,
  onRecord,
}: {
  open: boolean
  onClose: () => void
  onRecord: (action: string, status: string, msg: string) => void
}) {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [semester, setSemester] = useState<string | undefined>()
  const [keyword, setKeyword] = useState('')
  const [selectedNos, setSelectedNos] = useState<string[]>([])
  const [taskId, setTaskId] = useState<string | null>(null)

  const { data: currentSemester } = useQuery({
    queryKey: ['current-semester'],
    queryFn: () => scheduleApi.currentSemester(),
  })
  const { data: semesters } = useQuery({
    queryKey: ['semesters'],
    queryFn: () => scheduleApi.semesters(),
    enabled: open,
  })
  const { data: teachers } = useQuery({
    queryKey: ['llsykb-teachers', keyword],
    queryFn: () =>
      userApi.list({ page: 1, page_size: 50, role: 'teacher', keyword: keyword || undefined }),
    enabled: open,
  })

  useEffect(() => {
    if (open && currentSemester) setSemester(currentSemester.semester)
  }, [open, currentSemester])

  const startMut = useMutation({
    mutationFn: () => syncApi.llsykbBatch(semester!, undefined, selectedNos),
    onSuccess: (res) => {
      message.success('同步任务已启动')
      setTaskId(res.task_id)
    },
    onError: (e) => {
      message.error(e.message)
      onRecord('选择教师同步课表', 'failed', e.message)
    },
  })

  // 3 秒轮询进度：按已处理教师数/总教师数计算
  const { data: progress } = useQuery({
    queryKey: ['llsykb-progress', taskId],
    queryFn: () => syncApi.llsykbProgress(taskId!),
    refetchInterval: 3000,
    enabled: !!taskId,
  })

  const status = (progress as { status?: string } | undefined)?.status
  useEffect(() => {
    if (status === 'completed' || status === 'success') {
      message.success('所选教师课表同步完成')
      onRecord('选择教师同步课表', 'success', `所选教师课表同步完成（${selectedNos.length} 人）`)
      qc.invalidateQueries({ queryKey: ['semesters'] })
      qc.invalidateQueries({ queryKey: ['schedule'] })
      setTaskId(null)
      onClose()
    } else if (status === 'failed') {
      const reason = (progress as { error?: string } | undefined)?.error || '同步失败'
      message.error(reason)
      onRecord('选择教师同步课表', 'failed', reason)
      setTaskId(null)
    }
  }, [status]) // eslint-disable-line react-hooks/exhaustive-deps

  const percent = Number((progress as { percent?: number } | undefined)?.percent || 0)

  return (
    <Modal
      title="选择教师同步课表"
      open={open}
      onCancel={() => {
        setTaskId(null)
        onClose()
      }}
      width={640}
      footer={[
        <Button
          key="close"
          onClick={() => {
            if (taskId) {
              setTaskId(null) // 任务仍后台运行，仅隐藏进度
            } else {
              onClose()
            }
          }}
        >
          {taskId ? '隐藏' : '取消'}
        </Button>,
        <Button
          key="ok"
          type="primary"
          loading={startMut.isPending}
          disabled={selectedNos.length === 0 || !!taskId}
          onClick={() => startMut.mutate()}
        >
          同步所选（{selectedNos.length}）
        </Button>,
      ]}
    >
      <div className="filter-bar">
        <Select
          placeholder="学期"
          style={{ width: 200 }}
          value={semester}
          onChange={setSemester}
          options={(semesters || []).map((s) => ({ label: formatSemester(s), value: s }))}
        />
        <Input.Search
          allowClear
          placeholder="教师姓名/工号"
          style={{ width: 200 }}
          onSearch={setKeyword}
        />
      </div>
      <Table
        rowKey="user_no"
        size="small"
        loading={!teachers}
        pagination={false}
        scroll={{ y: 320 }}
        dataSource={teachers?.list || []}
        rowSelection={{
          selectedRowKeys: selectedNos,
          onChange: (keys) => setSelectedNos(keys as string[]),
        }}
        columns={[
          { title: '工号', dataIndex: 'user_no', width: 110 },
          { title: '姓名', dataIndex: 'username' },
          { title: '学院', dataIndex: 'college_name', render: (v) => v || '-' },
        ]}
      />
      {taskId && (
        <div style={{ marginTop: 8 }}>
          <Progress percent={percent} status="active" />
          <div style={{ color: '#999', fontSize: 12 }}>任务 {taskId}，每 3 秒刷新进度</div>
        </div>
      )}
    </Modal>
  )
}

/** 按学院批量同步（后台任务 + 3 秒轮询进度） */
function LlsykbBatchModal({
  open,
  onClose,
  onRecord,
}: {
  open: boolean
  onClose: () => void
  onRecord: (action: string, status: string, msg: string) => void
}) {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [semester, setSemester] = useState<string | undefined>()
  const [collegeId, setCollegeId] = useState<number | undefined>()
  const [taskId, setTaskId] = useState<string | null>(null)

  const { data: currentSemester } = useQuery({
    queryKey: ['current-semester'],
    queryFn: () => scheduleApi.currentSemester(),
  })
  const { data: semesters } = useQuery({
    queryKey: ['semesters'],
    queryFn: () => scheduleApi.semesters(),
    enabled: open,
  })
  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
    enabled: open,
  })

  useEffect(() => {
    if (open && currentSemester) setSemester(currentSemester.semester)
  }, [open, currentSemester])

  const startMut = useMutation({
    mutationFn: () => syncApi.llsykbBatch(semester!, collegeId),
    onSuccess: (res) => {
      message.success('批量同步任务已启动')
      setTaskId(res.task_id)
    },
    onError: (e) => {
      message.error(e.message)
      onRecord('按学院批量同步课表', 'failed', e.message)
    },
  })

  // 3 秒轮询进度：后台任务按「已处理用户数/总用户数」计算 percent，逐个教师完成时回推 completed
  const { data: progress } = useQuery({
    queryKey: ['llsykb-progress', taskId],
    queryFn: () => syncApi.llsykbProgress(taskId!),
    refetchInterval: 3000,
    enabled: !!taskId,
  })

  const status = (progress as { status?: string } | undefined)?.status
  useEffect(() => {
    if (status === 'completed' || status === 'success') {
      message.success('批量同步完成')
      onRecord('按学院批量同步课表', 'success', '批量同步完成')
      qc.invalidateQueries({ queryKey: ['semesters'] })
      qc.invalidateQueries({ queryKey: ['schedule'] })
      setTaskId(null)
      onClose()
    } else if (status === 'failed') {
      const reason = (progress as { error?: string } | undefined)?.error || '批量同步失败'
      message.error(reason)
      onRecord('按学院批量同步课表', 'failed', reason)
      setTaskId(null)
    }
  }, [status]) // eslint-disable-line react-hooks/exhaustive-deps

  const percent = Number((progress as { percent?: number } | undefined)?.percent || 0)

  return (
    <Modal
      title="按学院批量同步教师课表"
      open={open}
      onCancel={() => {
        setTaskId(null)
        onClose()
      }}
      footer={[
        <Button
          key="close"
          onClick={() => {
            if (taskId) {
              setTaskId(null) // 任务仍后台运行，仅隐藏进度
            } else {
              onClose() // 无任务时取消并关闭弹窗
            }
          }}
        >
          {taskId ? '隐藏' : '取消'}
        </Button>,
        <Button
          key="start"
          type="primary"
          loading={startMut.isPending}
          disabled={!!taskId}
          onClick={() => startMut.mutate()}
        >
          启动批量同步
        </Button>,
      ]}
    >
      <Form layout="vertical">
        <Form.Item label="学期">
          <Select
            value={semester}
            onChange={setSemester}
            options={(semesters || []).map((s) => ({ label: formatSemester(s), value: s }))}
          />
        </Form.Item>
        <Form.Item label="学院（不选则同步全部）">
          <Select
            allowClear
            value={collegeId}
            onChange={setCollegeId}
            options={(colleges?.list || []).map((c) => ({ label: c.name, value: c.id }))}
          />
        </Form.Item>
      </Form>
      {taskId && (
        <div style={{ marginTop: 8 }}>
          <Progress percent={percent} status="active" />
          <div style={{ color: '#999', fontSize: 12 }}>任务 {taskId}，每 3 秒刷新进度</div>
        </div>
      )}
    </Modal>
  )
}

function CrawlModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [form] = Form.useForm()

  const { data: semesters } = useQuery({
    queryKey: ['semesters'],
    queryFn: () => scheduleApi.semesters(),
    enabled: open,
  })

  const crawlMut = useMutation({
    mutationFn: (values: {
      username: string
      password: string
      semester: string
      college_code?: string
      teacher_name?: string
    }) => syncApi.crawlCourseSchedule(values),
    onSuccess: () => {
      message.success('课表抓取完成')
      qc.invalidateQueries({ queryKey: ['semesters'] })
      onClose()
    },
    onError: (e) => message.error(e.message),
  })

  // 旧方式：整页抓取，易漏没课老师、无进度，建议用「按学院批量同步课表（推荐）」
  return (
    <Modal
      title="教务系统抓取课表"
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      confirmLoading={crawlMut.isPending}
    >
      <Form form={form} layout="vertical" onFinish={(v) => crawlMut.mutate(v)}>
        <Form.Item
          name="username"
          label="教务系统用户名"
          tooltip="留空则使用 .env 配置的 JWXT_USERNAME"
        >
          <Input placeholder="留空使用系统配置账号" />
        </Form.Item>
        <Form.Item
          name="password"
          label="教务系统密码"
          tooltip="留空则使用 .env 配置的 JWXT_PASSWORD"
        >
          <Input.Password placeholder="留空使用系统配置账号" />
        </Form.Item>
        <Form.Item name="semester" label="学期" rules={[{ required: true }]}>
          <Select
            options={(semesters || []).map((s) => ({ label: formatSemester(s), value: s }))}
          />
        </Form.Item>
        <Form.Item name="college_code" label="学院编码（可选）">
          <Input />
        </Form.Item>
        <Form.Item name="teacher_name" label="教师姓名（可选）">
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  )
}

// llsykb 进度组件（预留批量同步进度展示）
export function LlsykbProgress({ taskId }: { taskId: string }) {
  const { data } = useQuery({
    queryKey: ['llsykb-progress', taskId],
    queryFn: () => syncApi.llsykbProgress(taskId),
    refetchInterval: 2000,
    enabled: !!taskId,
  })
  const percent = Number((data as { percent?: number } | undefined)?.percent || 0)
  return <Progress percent={percent} />
}
