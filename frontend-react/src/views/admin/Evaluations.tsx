import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, Button, Space, App, Popconfirm, Tag, Drawer, Input, Descriptions, DatePicker, Image, Slider, InputNumber, Radio, Checkbox, Switch, Spin } from 'antd'
import { DownloadOutlined, PrinterOutlined, SearchOutlined, EditOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import type { ColumnsType } from 'antd/es/table'
import { evaluationApi, type UpdatePayload } from '@/api/modules/evaluations'
import { statsApi } from '@/api/modules/stats'
import { useSemesterRangePicker } from '@/hooks/useSemesterDates'
import { downloadBlob, exportFilename } from '@/utils/download'
import ExportFieldSelector from '@/components/ExportFieldSelector'
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
  const canDeleteRecord = (r: EvaluationRecord) =>
    canDeleteAny || (canDeleteOwn && userId != null && r.evaluator_id === userId)
  const [params, setParams] = useState<{ page: number; page_size: number; keyword?: string; teacher_id?: number }>({ page: 1, page_size: 20 })
  const [keyword, setKeyword] = useState('')
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
      message.success('删除成功')
      qc.invalidateQueries({ queryKey: ['evaluations'] })
      setDetailId(null)
    },
    onError: (e) => message.error(e.message),
  })

  const columns: ColumnsType<EvaluationRecord> = [
    { title: 'ID', dataIndex: 'id', width: 70 },
    { title: '教师', dataIndex: 'teacher_name', width: 100 },
    { title: '课程', dataIndex: 'course_name' },
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
    },
    {
      title: '操作',
      width: 210,
      render: (_, record) => (
        <Space>
          {canDeleteAny && (
            <Button size="small" icon={<EditOutlined />} onClick={() => setEditId(record.id)}>
              编辑
            </Button>
          )}
          <Button size="small" onClick={() => setDetailId(record.id)}>
            详情
          </Button>
          {canDeleteRecord(record) && (
            <Popconfirm
              title="确认删除该评教记录？"
              description="删除后该记录将从列表、统计与汇总中移除，被评教师的评分统计会随之变化，且该操作不可恢复。"
              okText="删除"
              cancelText="取消"
              okButtonProps={{ danger: true }}
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
      <h3 className="page-title">评教记录</h3>
      <div className="filter-bar">
        <DatePicker.RangePicker
          value={dates[0] && dates[1] ? [dayjs(dates[0]), dayjs(dates[1])] : undefined}
          onChange={(v) =>
            onRange(v ? [v[0]!.format('YYYY-MM-DD'), v[1]!.format('YYYY-MM-DD')] : null)
          }
        />
        <Button
          icon={<DownloadOutlined />}
          style={{ width: 220 }}
          onClick={() => setExportOpen(true)}
        >
          批量导出（xlsx）
        </Button>
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
              {canDeleteAny && (
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
                  title="确认删除该评教记录？"
                  description="删除后该记录将从列表、统计与汇总中移除，被评教师的评分统计会随之变化，且该操作不可恢复。"
                  okText="删除"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={() => delMut.mutate(detail.id)}
                >
                  <Button danger size="small">
                    删除
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
          <EvaluationEditForm evalData={editQ.data} onClose={() => setEditId(null)} />
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

/** 详情值渲染：图片预览 / 文件下载 / 文本 */
function ValueRender({
  value,
  displayValue,
}: {
  value: unknown
  displayValue?: string
}) {
  if (isImageArray(value)) {
    return (
      <Image.PreviewGroup>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {(value as string[]).map((u) => (
            <Image key={u} src={u} width={64} height={64} style={{ objectFit: 'cover' }} />
          ))}
        </div>
      </Image.PreviewGroup>
    )
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

/** 编辑评教表单：按维度 schema 动态渲染控件；image/file 维度只读保留现有附件 */
function EvaluationEditForm({
  evalData,
  onClose,
}: {
  evalData: EvaluationRecord
  onClose: () => void
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

  const editMut = useMutation({
    mutationFn: (data: UpdatePayload) => evaluationApi.update(evalData.id, data),
    onSuccess: () => {
      message.success('修改成功')
      qc.invalidateQueries({ queryKey: ['evaluations'] })
      qc.invalidateQueries({ queryKey: ['eval-detail'] })
      qc.invalidateQueries({ queryKey: ['eval-edit'] })
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
    editMut.mutate({ dimension_values: values, is_anonymous: isAnonymous })
  }

  return (
    <div>
      <Descriptions column={1} size="small" bordered style={{ marginBottom: 16 }}>
        <Descriptions.Item label="教师">{evalData.teacher_name}</Descriptions.Item>
        <Descriptions.Item label="课程">{evalData.course_name}</Descriptions.Item>
        <Descriptions.Item label="评教人">{evalData.evaluator_name}</Descriptions.Item>
        <Descriptions.Item label="提交时间">{formatDate(evalData.submit_time)}</Descriptions.Item>
      </Descriptions>

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
            onChange={(v) => onChange(v)}
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
    return (
      <Image.PreviewGroup>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {(value as string[]).map((u) => (
            <Image key={u} src={u} width={64} height={64} style={{ objectFit: 'cover' }} />
          ))}
        </div>
      </Image.PreviewGroup>
    )
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
