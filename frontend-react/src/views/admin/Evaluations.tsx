import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, Button, Space, App, Popconfirm, Tag, Drawer, Input, Descriptions, DatePicker, Image, Slider, InputNumber, Radio, Checkbox, Switch, Spin, Select, Tooltip, Alert } from 'antd'
import { DownloadOutlined, PrinterOutlined, SearchOutlined, EditOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import type { ColumnsType } from 'antd/es/table'
import { evaluationApi, type UpdatePayload } from '@/api/modules/evaluations'
import { statsApi } from '@/api/modules/stats'
import { useSemesterRangePicker } from '@/hooks/useSemesterDates'
import { downloadAttachment, downloadBlob, exportFilename } from '@/utils/download'
import ExportFieldSelector from '@/components/ExportFieldSelector'
import TeacherSelect from '@/components/TeacherSelect'
import { buildTaskUpdatePayload, isTaskChanged, taskFieldsFromRecord, type TaskFormFields } from '@/utils/evalTaskEdit'
import {
  groupEvaluationDimensions,
  buildEvalPrintHtml,
  printHtml,
  isImageArray,
  isFileArray,
  formatValueText,
  getFileNameFromUrl,
} from '@/utils/evalDetail'
import type { EvaluationRecord } from '@/api/types'
import { formatDate } from '@/utils/format'
import { useAuthStore } from '@/stores/auth'

export default function Evaluations() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const userId = useAuthStore((s) => s.user?.id)
  const canDeleteAny = useAuthStore((s) => s.hasPermission('evaluation:delete'))
  const canDeleteOwn = useAuthStore((s) => s.hasPermission('evaluation:delete_own'))
  // 编辑记录（evaluation:update）：管理员可改评教内容；另持 task:update 时同页可改课堂课程信息
  const canUpdate = useAuthStore((s) => s.hasPermission('evaluation:update'))
  const canEditTaskInfo = useAuthStore((s) => s.hasPermission('task:update'))
  const canDeleteRecord = (r: EvaluationRecord) =>
    canDeleteAny || (canDeleteOwn && userId != null && r.evaluator_id === userId)
  const canRestoreAny = useAuthStore((s) => s.hasPermission('evaluation:restore'))
  const canRestoreOwn = useAuthStore((s) => s.hasPermission('evaluation:restore_own'))
  const [params, setParams] = useState<{ page: number; page_size: number; keyword?: string; teacher_id?: number; order_by?: string; order?: 'asc' | 'desc'; deleted?: 0 | 1 }>({ page: 1, page_size: 20 })
  const [keyword, setKeyword] = useState('')
  const [sortState, setSortState] = useState<{ field?: string; order?: 'ascend' | 'descend' }>({})
  const [detailId, setDetailId] = useState<number | null>(null)
  const [editId, setEditId] = useState<number | null>(null)
  const [exportOpen, setExportOpen] = useState(false)
  // 默认日期区间：当前学期（开学日 ~ 开学日+20周）
  const { effective: dates, onRange } = useSemesterRangePicker()

  const { data, isLoading } = useQuery({
    queryKey: ['evaluations', params, dates],
    queryFn: () => evaluationApi.list({ ...params, start_date: dates[0], end_date: dates[1] }),
  })

  // 详情走独立接口：含分组 schema（打印不再"未分组"）、格式化的提交时间、满分合计
  const detailQ = useQuery({
    queryKey: ['eval-detail', detailId],
    queryFn: () => evaluationApi.detail(detailId!),
    enabled: detailId != null,
  })
  const detail = detailQ.data ?? null

  // 编辑表单数据：复用详情接口（含分组 schema 与当前维度值）
  const editQ = useQuery({
    queryKey: ['eval-edit', editId],
    queryFn: () => evaluationApi.detail(editId!),
    enabled: editId != null,
  })

  const pdfMut = useMutation({
    mutationFn: (id: number) => evaluationApi.exportRecord(id, 'pdf'),
    onSuccess: (blob, id) => downloadBlob(blob, exportFilename(`评教记录_${id}`, 'pdf')),
    onError: (e) => message.error(e.message),
  })

  const exportMut = useMutation({
    mutationFn: (fields: string[]) =>
      statsApi.exportEvaluationRecords({ ...params, fields, start_date: dates[0], end_date: dates[1] }),
    onSuccess: (blob) => downloadBlob(blob, exportFilename('评教记录')),
    onError: (e) => message.error(e.message),
  })

  const delMut = useMutation({
    mutationFn: (id: number) => evaluationApi.remove(id),
    onSuccess: () => {
      message.success('作废成功')
      qc.invalidateQueries({ queryKey: ['evaluations'] })
      setDetailId(null)
    },
    onError: (e) => message.error(e.message),
  })

  const restoreMut = useMutation({
    mutationFn: (id: number) => evaluationApi.restore(id),
    onSuccess: () => {
      message.success('恢复成功')
      qc.invalidateQueries({ queryKey: ['evaluations'] })
    },
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<EvaluationRecord> = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 70,
      sorter: true,
      sortOrder: sortState.field === 'id' ? sortState.order : null,
    },
    { title: '教师', dataIndex: 'teacher_name', width: 100 },
    { title: '课程', dataIndex: 'course_name' },
    {
      title: '上课时间',
      dataIndex: 'class_time',
      width: 150,
      render: (v) => formatDate(v),
      sorter: true,
      sortOrder: sortState.field === 'class_time' ? sortState.order : null,
    },
    { title: '学院', dataIndex: 'college_name', width: 130, render: (v) => v || '-' },
    { title: '评教人', dataIndex: 'evaluator_name', width: 100 },
    {
      title: '匿名',
      dataIndex: 'is_anonymous',
      width: 70,
      render: (v: boolean) => (v ? <Tag color="gray">匿名</Tag> : '否'),
    },
    {
      title: '总分',
      dataIndex: 'total_score',
      width: 90,
      render: (v: number, r) => (v != null ? `${v} / ${r.max_total_score ?? '-'}` : '-'),
    },
    {
      title: '提交时间',
      dataIndex: 'submit_time',
      width: 160,
      render: (v: string) => formatDate(v),
      sorter: true,
      sortOrder: sortState.field === 'submit_time' ? sortState.order : null,
    },
    {
      title: '操作',
      width: 210,
      render: (_, record) =>
        params.deleted === 1 ? (
          <Space>
            {record.can_restore && (
              <Popconfirm
                title="确认恢复该评教记录？"
                description="恢复后该记录将重新计入列表、统计与汇总，被评教师的评分统计会随之变化。"
                okText="恢复"
                cancelText="取消"
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
            {canUpdate && (
              <Button size="small" icon={<EditOutlined />} onClick={() => setEditId(record.id)}>
                编辑
              </Button>
            )}
            <Button size="small" onClick={() => setDetailId(record.id)}>
              详情
            </Button>
            {canDeleteRecord(record) && (
              <Popconfirm
                title="确认作废该评教记录？"
                description="作废后该记录将从列表、统计与汇总中移除，被评教师的评分统计会随之变化；数据会保留可追溯，评教人可对同一任务重新提交。"
                okText="作废"
                cancelText="取消"
                okButtonProps={{ danger: true }}
                onConfirm={() => delMut.mutate(record.id)}
              >
                <Button size="small" danger>
                  作废
                </Button>
              </Popconfirm>
            )}
          </Space>
        ),
    },
  ]

  return (
    <div>
      <h3 className="page-title">评教记录</h3>
      <div className="filter-bar">
        <DatePicker.RangePicker
          value={dates[0] && dates[1] ? [dayjs(dates[0]), dayjs(dates[1])] : undefined}
          onChange={(v) =>
            onRange(v ? [v[0]!.format('YYYY-MM-DD'), v[1]!.format('YYYY-MM-DD')] : null)
          }
        />
        {(canRestoreAny || canRestoreOwn) && (
          <Select
            style={{ width: 130 }}
            value={params.deleted === 1 ? 1 : 0}
            options={[
              { label: '正常记录', value: 0 },
              { label: '已作废记录', value: 1 },
            ]}
            onChange={(v) =>
              setParams((p) => ({ ...p, page: 1, deleted: v === 1 ? 1 : undefined }))
            }
          />
        )}
        <Tooltip title={params.deleted === 1 ? '已作废视图不支持导出' : undefined}>
          <span>
            <Button
              icon={<DownloadOutlined />}
              style={{ width: 220 }}
              disabled={params.deleted === 1}
              onClick={() => setExportOpen(true)}
            >
              批量导出（xlsx）
            </Button>
          </span>
        </Tooltip>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="教师/课程/评教人"
          style={{ width: 220 }}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          onPressEnter={() => setParams((p) => ({ ...p, page: 1, keyword }))}
        />
      </div>
      <Table<EvaluationRecord>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={data?.list}
        onChange={(_p, _f, sorter, extra) => {
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
          onChange: (page, pageSize) =>
            setParams((p) => ({ ...p, page, page_size: pageSize, keyword })),
        }}
      />

      <Drawer
        title="评教详情"
        open={detailId != null}
        onClose={() => setDetailId(null)}
        width={480}
        extra={
          detail && (
            <Space>
              {canUpdate && (
                <Button
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => {
                    setDetailId(null)
                    setEditId(detail.id)
                  }}
                >
                  编辑
                </Button>
              )}
              <Button
                size="small"
                icon={<PrinterOutlined />}
                onClick={() => printHtml(buildEvalPrintHtml(detail))}
              >
                打印
              </Button>
              <Button
                size="small"
                icon={<DownloadOutlined />}
                loading={pdfMut.isPending && pdfMut.variables === detail.id}
                onClick={() => pdfMut.mutate(detail.id)}
              >
                导出
              </Button>
              {canDeleteRecord(detail) && (
                <Popconfirm
                  title="确认作废该评教记录？"
                  description="作废后该记录将从列表、统计与汇总中移除，被评教师的评分统计会随之变化；数据会保留可追溯，评教人可对同一任务重新提交。"
                  okText="作废"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={() => delMut.mutate(detail.id)}
                >
                  <Button danger size="small">
                    作废
                  </Button>
                </Popconfirm>
              )}
            </Space>
          )
        }
      >
        {detail && (
          <>
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="教师">{detail.teacher_name}</Descriptions.Item>
              <Descriptions.Item label="课程">{detail.course_name}</Descriptions.Item>
              <Descriptions.Item label="上课时间">{formatDate(detail.class_time)}</Descriptions.Item>
              <Descriptions.Item label="学院">{detail.college_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="评教人">{detail.evaluator_name}</Descriptions.Item>
              <Descriptions.Item label="评教角色">
                {detail.evaluator_role_name || detail.evaluator_role || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="是否匿名">
                {detail.is_anonymous ? '是' : '否'}
              </Descriptions.Item>
              <Descriptions.Item label="总分">
                {detail.total_score ?? '-'} / {detail.max_total_score ?? '-'}
              </Descriptions.Item>
              <Descriptions.Item label="提交时间">{formatDate(detail.submit_time)}</Descriptions.Item>
            </Descriptions>
            {detail.schedule && (
              <>
                <h4 style={{ margin: '16px 0 8px' }}>课表信息</h4>
                <Descriptions column={1} size="small" bordered>
                  <Descriptions.Item label="上课时间">
                    {detail.schedule.class_time_text || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="教室">
                    {detail.schedule.classroom || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="班级">
                    {detail.schedule.class_info || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="应到人数">
                    {detail.schedule.student_count != null
                      ? `${detail.schedule.student_count}人`
                      : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="周次">
                    {detail.schedule.week_pattern || '-'}
                  </Descriptions.Item>
                </Descriptions>
              </>
            )}
            <h4 style={{ margin: '16px 0 8px' }}>打分明细</h4>
            {groupEvaluationDimensions(detail).map((g) => (
              <div key={g.key} style={{ marginBottom: 12 }}>
                <div
                  style={{
                    fontWeight: 600,
                    padding: '4px 0',
                    borderBottom: '2px solid #104186',
                    marginBottom: 4,
                  }}
                >
                  {g.name}
                  {g.max_score > 0 && (
                    <span style={{ float: 'right', color: '#c00' }}>
                      {g.score}/{g.max_score}
                    </span>
                  )}
                </div>
                <Descriptions column={1} size="small" bordered>
                  {g.rows.map((row) => (
                    <Descriptions.Item key={row.code} label={row.name}>
                      <ValueRender value={row.value} displayValue={row.display_value} />
                    </Descriptions.Item>
                  ))}
                </Descriptions>
              </div>
            ))}
          </>
        )}
      </Drawer>

      {/* 编辑评教 */}
      <Drawer
        title="编辑评教"
        open={editId != null}
        onClose={() => setEditId(null)}
        width={560}
      >
        {editQ.isLoading || !editQ.data ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin />
          </div>
        ) : (
          <EvaluationEditForm
            evalData={editQ.data}
            canEditTaskInfo={canEditTaskInfo}
            onClose={() => setEditId(null)}
          />
        )}
      </Drawer>

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

/** 图片附件：点击缩略图看大图，大图工具栏与缩略图下方均可下载 */
function ImageAttachments({ urls }: { urls: string[] }) {
  const { message } = App.useApp()
  const [downloading, setDownloading] = useState<string | null>(null)

  const download = async (url: string) => {
    setDownloading(url)
    try {
      await downloadAttachment(url)
    } catch (e) {
      message.error(e instanceof Error ? e.message : '下载失败')
    } finally {
      setDownloading(null)
    }
  }

  return (
    <Image.PreviewGroup
      preview={{
        toolbarRender: (node, info) => (
          <>
            {node}
            <Button
              type="text"
              size="small"
              icon={<DownloadOutlined />}
              style={{ marginLeft: 8, color: '#fff' }}
              onClick={() => void download(info.image.url)}
            >
              下载
            </Button>
          </>
        ),
      }}
    >
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        {urls.map((u) => (
          <div
            key={u}
            style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}
          >
            <Image src={u} width={64} height={64} style={{ objectFit: 'cover' }} />
            <Button
              type="link"
              size="small"
              icon={<DownloadOutlined />}
              loading={downloading === u}
              onClick={() => void download(u)}
              style={{ padding: 0, height: 20, fontSize: 12 }}
            >
              下载
            </Button>
          </div>
        ))}
      </div>
    </Image.PreviewGroup>
  )
}

/** 详情值渲染：图片预览 / 文件下载 / 文本 */
function ValueRender({
  value,
  displayValue,
}: {
  value: unknown
  displayValue?: string
}) {
  if (isImageArray(value)) {
    return <ImageAttachments urls={value as string[]} />
  }
  if (isFileArray(value)) {
    return (
      <div>
        {(value as string[]).map((u) => (
          <div key={u}>
            <a href={u} target="_blank" rel="noreferrer" download={getFileNameFromUrl(u)}>
              {getFileNameFromUrl(u)}
            </a>
          </div>
        ))}
      </div>
    )
  }
  return <span>{displayValue || formatValueText(value)}</span>
}

/* ═══ 编辑评教表单 ═══ */

interface EditDim {
  code: string
  name: string
  field_type?: string
  field_config?: Record<string, unknown>
  is_required?: boolean
  group_name?: string
  [key: string]: unknown
}

/** 单选/多选选项归一化 */
const choiceOptions = (cfg: Record<string, unknown>) => {
  const opts = Array.isArray(cfg.options) ? cfg.options : []
  return opts.map((o) => {
    const rec = o as { label?: unknown; value?: unknown } | string
    if (typeof rec !== 'object' || rec === null) return { label: String(rec), value: String(rec) }
    const label = String(rec.label ?? rec.value ?? '')
    const value = rec.value ?? rec.label ?? ''
    return { label, value: String(value) }
  })
}

const toNum = (v: unknown, fallback: number) => {
  const n = Number(v)
  return Number.isFinite(n) ? n : fallback
}

/** 编辑评教表单：按维度 schema 动态渲染控件；image/file 维度只读保留现有附件。
 *  另持 task:update 时可改课堂课程信息（改任务本身，影响该任务下全部记录），
 *  仅回传发生变化的字段，未改动时不触碰任务。 */
function EvaluationEditForm({
  evalData,
  onClose,
  canEditTaskInfo,
}: {
  evalData: EvaluationRecord
  onClose: () => void
  canEditTaskInfo: boolean
}) {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const dims = useMemo<EditDim[]>(
    () =>
      (evalData.dimension_groups || []).flatMap((g) =>
        (g.dimensions || []).map((d) => ({
          ...d,
          group_name: g.name || g.group_name || '',
        }))
      ),
    [evalData]
  )
  const [values, setValues] = useState<Record<string, unknown>>(() => ({
    ...(evalData.dimension_values || {}),
  }))
  const [isAnonymous, setIsAnonymous] = useState(!!evalData.is_anonymous)
  const [task, setTask] = useState<TaskFormFields>(() => taskFieldsFromRecord(evalData))
  const patchTask = (patch: Partial<TaskFormFields>) =>
    setTask((prev) => ({ ...prev, ...patch }))

  const taskChanged = useMemo(
    () => canEditTaskInfo && isTaskChanged(evalData, task),
    [task, evalData, canEditTaskInfo]
  )

  /** 仅回传发生变化的课堂字段；未改动返回 undefined（避免误触任务侧校验） */
  const buildTaskPayload = () =>
    canEditTaskInfo ? buildTaskUpdatePayload(evalData, task) : undefined

  const editMut = useMutation({
    mutationFn: (data: UpdatePayload) => evaluationApi.update(evalData.id, data),
    onSuccess: () => {
      message.success('修改成功')
      qc.invalidateQueries({ queryKey: ['evaluations'] })
      qc.invalidateQueries({ queryKey: ['eval-detail'] })
      qc.invalidateQueries({ queryKey: ['eval-edit'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
      onClose()
    },
    onError: (e) => message.error(e.message),
  })

  const handleSave = () => {
    for (const d of dims) {
      if (!d.is_required) continue
      const v = values[d.code]
      if (
        v === undefined ||
        v === null ||
        v === '' ||
        (Array.isArray(v) && v.length === 0)
      ) {
        message.error(`请填写「${d.name}」`)
        return
      }
    }
    if (taskChanged && !task.teacherId) {
      message.error('请选择被评教师')
      return
    }
    if (taskChanged && !task.courseName.trim()) {
      message.error('请填写课程名称')
      return
    }
    editMut.mutate({
      dimension_values: values,
      is_anonymous: isAnonymous,
      task: buildTaskPayload(),
    })
  }

  return (
    <div>
      <Descriptions column={1} size="small" bordered style={{ marginBottom: 16 }}>
        <Descriptions.Item label="教师">{evalData.teacher_name}</Descriptions.Item>
        <Descriptions.Item label="课程">{evalData.course_name}</Descriptions.Item>
        <Descriptions.Item label="评教人">{evalData.evaluator_name}</Descriptions.Item>
        <Descriptions.Item label="提交时间">{formatDate(evalData.submit_time)}</Descriptions.Item>
      </Descriptions>

      {canEditTaskInfo && (
        <>
          <h4 style={{ margin: '0 0 8px' }}>课堂信息</h4>
          {taskChanged && (evalData.evaluation_count ?? 0) > 0 && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message={`该课堂下共 ${evalData.evaluation_count} 条评教记录`}
              description="保存后将同步修改全部记录显示的课程信息，统计、同课汇总与学期归属随之变化。"
            />
          )}
          <Field label="被评教师">
            <TeacherSelect
              value={task.teacherId}
              style={{ width: '100%' }}
              onChange={(v) => patchTask({ teacherId: v })}
            />
          </Field>
          <Field label="课程名称">
            <Input
              value={task.courseName}
              onChange={(e) => patchTask({ courseName: e.target.value })}
            />
          </Field>
          <Field label="上课时间">
            <DatePicker
              showTime
              allowClear={false}
              style={{ width: '100%' }}
              value={task.classTime}
              onChange={(d) => patchTask({ classTime: d ?? null })}
            />
          </Field>
          <Field label="教室">
            <Input
              value={task.classroom}
              onChange={(e) => patchTask({ classroom: e.target.value })}
            />
          </Field>
          <Field label="上课时间文本（课表）">
            <Input
              placeholder="如：周一 第1-2节"
              value={task.classTimeText}
              onChange={(e) => patchTask({ classTimeText: e.target.value })}
            />
          </Field>
          <Field label="班级">
            <Input
              value={task.classInfo}
              onChange={(e) => patchTask({ classInfo: e.target.value })}
            />
          </Field>
          <Field label="应到人数">
            <InputNumber
              style={{ width: '100%' }}
              min={0}
              value={task.studentCount}
              onChange={(v) => patchTask({ studentCount: v ?? null })}
            />
          </Field>
          <Field label="周次">
            <Input
              placeholder="如：第1-16周"
              value={task.weekPattern}
              onChange={(e) => patchTask({ weekPattern: e.target.value })}
            />
          </Field>
        </>
      )}

      {dims.map((d) => (
        <div key={d.code} style={{ marginBottom: 16 }}>
          <div style={{ fontWeight: 600, marginBottom: 6 }}>
            {d.group_name && (
              <span style={{ color: '#999', fontWeight: 400, marginRight: 6 }}>{d.group_name}</span>
            )}
            {d.name}
            {d.is_required && <span style={{ color: '#ff4d4f' }}> *</span>}
          </div>
          <EditDimControl
            dim={d}
            value={values[d.code]}
            onChange={(v) => setValues((prev) => ({ ...prev, [d.code]: v }))}
          />
        </div>
      ))}

      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '12px 0',
          borderTop: '1px solid #f0f0f0',
        }}
      >
        <span>匿名评教</span>
        <Switch checked={isAnonymous} onChange={setIsAnonymous} />
      </div>

      <div style={{ textAlign: 'right', marginTop: 16 }}>
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={editMut.isPending} onClick={handleSave}>
            保存
          </Button>
        </Space>
      </div>
    </div>
  )
}

/** 表单字段（标签 + 控件） */
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 12 }}>
      <div style={{ fontWeight: 600, marginBottom: 6 }}>{label}</div>
      {children}
    </div>
  )
}

/** 单个维度编辑控件 */
function EditDimControl({
  dim,
  value,
  onChange,
}: {
  dim: EditDim
  value: unknown
  onChange: (v: unknown) => void
}) {
  const cfg = dim.field_config || {}
  switch (dim.field_type) {
    case 'score': {
      const min = toNum(cfg.min_score, 0)
      const max = toNum(cfg.max_score, 20)
      const cur = typeof value === 'number' ? value : max
      return (
        <>
          <Slider
            min={min}
            max={max}
            step={toNum(cfg.step, 1)}
            value={cur}
            onChange={(v) => {
              // 浮点步进可能产生长小数（如 5.1000000000000005），统一保留一位
              const n = Array.isArray(v) ? v[0] : v
              onChange(Math.round(n * 10) / 10)
            }}
          />
          <div style={{ textAlign: 'right', color: '#104186', fontSize: 13 }}>{cur} 分</div>
        </>
      )
    }
    case 'single_choice':
      return (
        <Radio.Group
          value={value == null ? undefined : String(value)}
          options={choiceOptions(cfg)}
          onChange={(e) => onChange(e.target.value)}
        />
      )
    case 'multiple_choice':
      return (
        <Checkbox.Group
          value={(Array.isArray(value) ? value : value != null ? [value] : []) as (string | number)[]}
          options={choiceOptions(cfg)}
          onChange={(v) => onChange(v)}
        />
      )
    case 'text':
    case 'rich_text':
      return (
        <Input.TextArea
          rows={3}
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
          placeholder="请输入"
        />
      )
    case 'number': {
      const min = typeof cfg.min === 'number' ? cfg.min : undefined
      const max = typeof cfg.max === 'number' ? cfg.max : undefined
      return (
        <InputNumber
          style={{ width: '100%' }}
          min={min}
          max={max}
          value={typeof value === 'number' ? value : undefined}
          onChange={(v) => onChange(v ?? undefined)}
        />
      )
    }
    case 'date':
      return (
        <DatePicker
          value={typeof value === 'string' && value ? dayjs(value) : undefined}
          onChange={(d) => onChange(d ? d.format('YYYY-MM-DD') : undefined)}
        />
      )
    case 'image':
    case 'file':
      return <ReadonlyFiles value={value} />
    default:
      return (
        <Input
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
        />
      )
  }
}

/** 只读展示现有附件（编辑时不做增删） */
function ReadonlyFiles({ value }: { value: unknown }) {
  if (isImageArray(value)) {
    return <ImageAttachments urls={value as string[]} />
  }
  if (isFileArray(value)) {
    return (
      <div>
        {(value as string[]).map((u) => (
          <div key={u}>
            <a href={u} target="_blank" rel="noreferrer" download={getFileNameFromUrl(u)}>
              {getFileNameFromUrl(u)}
            </a>
          </div>
        ))}
      </div>
    )
  }
  return <span style={{ color: '#999' }}>无附件</span>
}
