import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  NavBar,
  Card,
  Button,
  TextArea,
  Switch,
  Slider,
  Selector,
  Toast,
  Input,
  ErrorBlock,
  Dialog,
  ImageViewer,
  SpinLoading,
} from 'antd-mobile'
import { evaluationApi, type SubmitPayload } from '@/api/modules/evaluations'
import { taskApi } from '@/api/modules/tasks'
import { dimensionApi } from '@/api/modules/dimensions'
import { uploadApi } from '@/api/modules/upload'
import { scheduleApi } from '@/api/modules/schedule'
import FormulaEditor from '@/components/FormulaEditor'
import type { Dimension, CourseItem } from '@/api/types'
import { formatDate } from '@/utils/format'

export default function Evaluation() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const taskId = Number(id)

  const [values, setValues] = useState<Record<string, unknown>>({})
  const [isAnonymous, setIsAnonymous] = useState(false)
  const [formulaDim, setFormulaDim] = useState<Dimension | null>(null)

  // 任务详情
  const { data: task, isLoading: taskLoading } = useQuery({
    queryKey: ['task', taskId],
    queryFn: () => taskApi.get(taskId),
    enabled: !!taskId,
  })

  // 维度
  const { data: dims } = useQuery({
    queryKey: ['dimensions', 'active'],
    queryFn: () => dimensionApi.active(),
  })

  // 维度加载后，将 score 类维度默认初始化为满分，保证 UI 与统计一致
  useEffect(() => {
    if (!dims || dims.length === 0) return
    setValues((prev) => {
      const next = { ...prev }
      for (const d of dims) {
        if (d.field_type !== 'score') continue
        if (next[d.code] === undefined || next[d.code] === null) {
          const cfg = d.field_config || {}
          next[d.code] = Number(cfg.max_score ?? 20)
        }
      }
      return next
    })
  }, [dims])

  // 课表信息展示：优先用「加入待评时固化的快照」（周次已收敛到添加周），无快照才回退实时查课表
  const snapshot = task?.schedule
  // 课表自动匹配（用于应到人数预填/出勤率计算）：仍走实时课表，与展示解耦
  const { data: schedule } = useQuery({
    queryKey: ['schedule', 'teacher', task?.teacher_id, 'auto'],
    queryFn: () => scheduleApi.byTeacher(task!.teacher_id),
    enabled: !!task?.teacher_id,
    retry: false,
  })

  const matchedCourse: CourseItem | null = useMemo(() => {
    const details = schedule?.details || []
    if (!task || details.length === 0) return null
    return details.find((d) => d.course_name === task.course_name) || null
  }, [schedule, task])

  // 展示用的课表信息：快照优先，兜底实时课表（历史/PC 端任务无快照）
  const displaySchedule = useMemo(() => {
    if (snapshot) return snapshot
    if (!matchedCourse) return null
    return {
      class_time_text: undefined as string | undefined,
      classroom: matchedCourse.classroom || undefined,
      class_info: matchedCourse.class_info || undefined,
      student_count: matchedCourse.student_count != null ? Number(matchedCourse.student_count) : undefined,
      week_pattern: matchedCourse.week_pattern || undefined,
    }
  }, [snapshot, matchedCourse])

  // 应到人数预填：优先用「加入待评时固化的快照」（与顶部展示一致），无快照才回退实时课表匹配
  const prefilledCount = snapshot?.student_count ?? matchedCourse?.student_count
  useEffect(() => {
    if (!dims || dims.length === 0 || !prefilledCount) return
    setValues((prev) => {
      const dim = dims.find((d) => d.code === 'expected_count' && d.field_type === 'number')
      if (!dim) return prev
      const cur = prev[dim.code]
      if (cur !== undefined && cur !== null && cur !== '') return prev
      return { ...prev, [dim.code]: prefilledCount }
    })
  }, [dims, prefilledCount])

  // 出勤率自动计算：实到人数 ÷ 应到人数 × 100，保留一位小数（用户手动修改后不再覆盖）
  const lastAutoRate = useRef<number | null>(null)
  useEffect(() => {
    const dimsArr = dims || []
    const rateDim = dimsArr.find((d) => d.code === 'attendance_rate' && d.field_type === 'number')
    if (!rateDim) return
    const exp = values['expected_count'] ?? displaySchedule?.student_count
    const act = values['actual_count']
    if (typeof exp !== 'number' || typeof act !== 'number' || !Number.isFinite(exp) || !Number.isFinite(act) || exp <= 0 || act < 0) return
    const cfg = rateDim.field_config || {}
    const min = typeof cfg.min === 'number' ? cfg.min : 0
    const max = typeof cfg.max === 'number' ? cfg.max : 100
    const rate = Math.min(max, Math.max(min, Math.round((act / exp) * 1000) / 10))
    const cur = values[rateDim.code]
    if (cur === undefined || cur === null || cur === '' || cur === lastAutoRate.current) {
      lastAutoRate.current = rate
      setValues((prev) => ({ ...prev, [rateDim.code]: rate }))
    }
  }, [dims, values, displaySchedule])

  const scoreDims = (dims || []).filter(
    (d) => d.field_type === 'score' && values[d.code] != null
  )
  const totalCurrent = scoreDims.reduce((s, d) => s + Number(values[d.code] || 0), 0)
  const totalMax = (dims || [])
    .filter((d) => d.field_type === 'score')
    .reduce((s, d) => {
      const cfg = d.field_config || {}
      return s + Number(cfg.max_score ?? 20)
    }, 0)

  const submitMut = useMutation({
    mutationFn: (payload: SubmitPayload) => evaluationApi.submit(payload),
    onSuccess: () => {
      Toast.clear()
      Toast.show({ content: '评教成功', icon: 'success' })
      qc.invalidateQueries({ queryKey: ['mobile-tasks'] })
      qc.invalidateQueries({ queryKey: ['evaluations'] })
      navigate('/mobile/home', { replace: true })
    },
    onError: (e) => Toast.show({ content: e instanceof Error ? e.message : '提交评教失败', icon: 'fail' }),
  })

  const validate = (): string | null => {
    for (const d of dims || []) {
      if (!d.is_required) continue
      const v = values[d.code]
      if (v === undefined || v === null || v === '' || (Array.isArray(v) && v.length === 0)) {
        return `请填写「${d.name}」`
      }
    }
    return null
  }

  const handleSubmit = () => {
    const err = validate()
    if (err) {
      Toast.show({ content: err, icon: 'fail' })
      return
    }
    Dialog.confirm({
      title: '确认提交',
      content: '提交后将无法修改，是否确认？',
      confirmText: '提交',
      cancelText: '取消',
      onConfirm: () => {
        Toast.show({ content: '提交中...', icon: 'loading', duration: 0 })
        submitMut.mutate({
          task_id: taskId,
          dimension_values: values,
          is_anonymous: isAnonymous,
        })
      },
    })
  }

  if (taskLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 80 }}>
        <SpinLoading color="primary" />
      </div>
    )
  }
  if (!task) {
    return <ErrorBlock status="empty" title="任务不存在" description="" />
  }

  return (
    <div>
      <NavBar onBack={() => navigate(-1)}>评教</NavBar>

      {/* 任务信息 */}
      <Card style={{ margin: 12 }}>
        <div style={{ fontWeight: 600, fontSize: 16 }}>{task.course_name}</div>
        <div style={{ color: '#666', fontSize: 13, marginTop: 6 }}>
          教师：{task.teacher_name}
        </div>
        <div style={{ color: '#666', fontSize: 13 }}>
          教室：{task.classroom || '待定'}
        </div>
        <div style={{ color: '#666', fontSize: 13 }}>
          时间：{formatDate(task.class_time)}
        </div>
      </Card>

      {/* 课表信息（快照优先，回退实时课表；仅展示） */}
      {displaySchedule && (
        <Card style={{ margin: '0 12px 12px' }} title={<span style={{ fontSize: 14 }}>课表信息</span>}>
          <InfoRow label="上课时间">
            {displaySchedule.class_time_text || '-'}
          </InfoRow>
          {displaySchedule.classroom && <InfoRow label="教室">{displaySchedule.classroom}</InfoRow>}
          {displaySchedule.class_info && <InfoRow label="班级">{displaySchedule.class_info}</InfoRow>}
          {displaySchedule.student_count != null && (
            <InfoRow label="应到人数">{displaySchedule.student_count}人</InfoRow>
          )}
          {displaySchedule.week_pattern && <InfoRow label="周次">{displaySchedule.week_pattern}</InfoRow>}
        </Card>
      )}

      {/* 动态维度表单 */}
      <div style={{ padding: '0 12px' }}>
        {(dims || []).map((d) => (
          <DimensionInput
            key={d.id}
            dim={d}
            value={values[d.code]}
            onChange={(v) => setValues((prev) => ({ ...prev, [d.code]: v }))}
            taskId={taskId}
            onOpenFormula={() => setFormulaDim(d)}
          />
        ))}

        {/* 评分汇总 */}
        {totalMax > 0 && (
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '12px 16px',
              background: '#fff',
              borderRadius: 8,
              marginBottom: 12,
            }}
          >
            <span style={{ fontSize: 14 }}>当前得分</span>
            <span style={{ fontSize: 20, fontWeight: 600, color: '#ff3141' }}>
              {totalCurrent}
              <span style={{ fontSize: 13, color: '#999' }}> / {totalMax}</span>
            </span>
          </div>
        )}

        {/* 匿名开关 */}
        <Card style={{ marginBottom: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>匿名评教</span>
            <Switch checked={isAnonymous} onChange={setIsAnonymous} />
          </div>
        </Card>

        <Button
          block
          color="primary"
          size="large"
          shape="rounded"
          style={{ marginBottom: 24 }}
          loading={submitMut.isPending}
          onClick={handleSubmit}
        >
          提交评教
        </Button>
      </div>

      {/* 公式编辑器 */}
      {formulaDim && (
        <FormulaEditor
          visible
          initialValue={typeof values[formulaDim.code] === 'string' ? (values[formulaDim.code] as string) : ''}
          onClose={() => setFormulaDim(null)}
          onConfirm={(latex) => {
            const cur = typeof values[formulaDim.code] === 'string' ? (values[formulaDim.code] as string) : ''
            setValues((prev) => ({ ...prev, [formulaDim.code]: `${cur}$${latex}$` }))
          }}
        />
      )}
    </div>
  )
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', fontSize: 13, lineHeight: 1.9 }}>
      <span style={{ color: '#999', width: 72, flexShrink: 0 }}>{label}</span>
      <span>{children}</span>
    </div>
  )
}

/* ═══ 单个维度输入 ═══ */
function DimensionInput({
  dim,
  value,
  onChange,
  taskId,
  onOpenFormula,
}: {
  dim: Dimension
  value: unknown
  onChange: (v: unknown) => void
  taskId: number
  onOpenFormula: () => void
}) {
  const cfg = dim.field_config || {}

  const renderControl = () => {
    switch (dim.field_type) {
      case 'score': {
        const min = Number(cfg.min_score ?? 0)
        const max = Number(cfg.max_score ?? 20)
        const fallback = max
        return (
          <>
            <Slider
              min={min}
              max={max}
              step={Number(cfg.step ?? 1)}
              value={typeof value === 'number' ? value : fallback}
              onChange={(v) => onChange(Array.isArray(v) ? v[0] : v)}
            />
            <div style={{ textAlign: 'right', color: '#104186', fontSize: 13, marginTop: 4 }}>
              {typeof value === 'number' ? value : fallback} 分
            </div>
          </>
        )
      }
      case 'single_choice':
      case 'multiple_choice': {
        const options = ((cfg.options as { label: string; value: string }[]) || []).map((o) => ({
          label: o.label,
          value: o.value,
        }))
        return (
          <Selector
            options={options}
            multiple={dim.field_type === 'multiple_choice'}
            value={(Array.isArray(value) ? value : value != null ? [value] : []) as string[]}
            onChange={(v) => onChange(dim.field_type === 'multiple_choice' ? v : v[0])}
          />
        )
      }
      case 'text':
        return (
          <TextArea
            placeholder={String(cfg.placeholder || '请输入')}
            maxLength={Number(cfg.max_length || 500)}
            value={typeof value === 'string' ? value : ''}
            onChange={onChange}
            rows={3}
          />
        )
      case 'rich_text':
        return (
          <>
            <TextArea
              placeholder={String(cfg.placeholder || '请输入，可插入公式')}
              maxLength={Number(cfg.max_length || 2000)}
              value={typeof value === 'string' ? value : ''}
              onChange={onChange}
              rows={4}
            />
            <Button size="mini" fill="none" style={{ marginTop: 4 }} onClick={onOpenFormula}>
              插入公式
            </Button>
          </>
        )
      case 'number': {
        const min = typeof cfg.min === 'number' ? cfg.min : null
        const max = typeof cfg.max === 'number' ? cfg.max : null
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <Input
              type="number"
              placeholder={String(cfg.placeholder || '请输入')}
              value={value == null ? '' : String(value)}
              onChange={(v) => {
                if (v === '') return onChange(undefined)
                const n = Number(v)
                if (Number.isNaN(n)) return
                if (min !== null && n < min) {
                  Toast.show({ content: `不能小于 ${min}`, icon: 'fail' })
                  return
                }
                if (max !== null && n > max) {
                  Toast.show({ content: `不能大于 ${max}`, icon: 'fail' })
                  return
                }
                onChange(n)
              }}
              style={{ flex: 1 }}
            />
            {cfg.unit ? <span style={{ color: '#999', fontSize: 13 }}>{String(cfg.unit)}</span> : null}
          </div>
        )
      }
      case 'date':
        return (
          <Input type="date" value={typeof value === 'string' ? value : ''} onChange={onChange} />
        )
      case 'image':
      case 'file':
        return (
          <DimensionUpload
            taskId={taskId}
            dimCode={dim.code}
            accept={dim.field_type === 'image' ? 'image/*' : undefined}
            maxCount={Number(cfg.max_count ?? (dim.field_type === 'image' ? 9 : 5))}
            maxSizeMb={Number(cfg.max_file_size ?? (dim.field_type === 'image' ? 10 : 20))}
            description={cfg.file_type_description ? String(cfg.file_type_description) : undefined}
            urls={Array.isArray(value) ? (value as string[]) : []}
            onChange={(urls) => onChange(urls.length > 0 ? urls : undefined)}
          />
        )
      default:
        return (
          <Input
            placeholder={`请输入${dim.name}`}
            value={typeof value === 'string' ? value : ''}
            onChange={onChange}
          />
        )
    }
  }

  return (
    <Card style={{ marginBottom: 8 }}>
      <div style={{ marginBottom: 8 }}>
        <strong>{dim.name}</strong>
        {dim.is_required && <span style={{ color: '#ff3141' }}> *</span>}
        {dim.description && (
          <div style={{ color: '#999', fontSize: 12 }}>{dim.description}</div>
        )}
      </div>
      {renderControl()}
    </Card>
  )
}

/* ═══ 多文件即传上传 ═══ */
function DimensionUpload({
  taskId,
  dimCode,
  accept,
  maxCount,
  maxSizeMb,
  description,
  urls,
  onChange,
}: {
  taskId: number
  dimCode: string
  accept?: string
  maxCount: number
  maxSizeMb: number
  description?: string
  urls: string[]
  onChange: (urls: string[]) => void
}) {
  const [uploadingCount, setUploadingCount] = useState(0)

  const handleFiles = async (files: FileList) => {
    const room = maxCount - urls.length
    if (room <= 0) {
      Toast.show({ content: `最多上传 ${maxCount} 个`, icon: 'fail' })
      return
    }
    const list = Array.from(files).slice(0, room)
    for (const f of list) {
      if (f.size > maxSizeMb * 1024 * 1024) {
        Toast.show({ content: `${f.name} 超过 ${maxSizeMb}MB 限制`, icon: 'fail' })
        continue
      }
      setUploadingCount((c) => c + 1)
      try {
        const res = await uploadApi.uploadEvaluationFile(taskId, dimCode, f)
        const url = res.files?.[0]?.url
        if (!url) {
          Toast.show({ content: `${f.name} 上传失败（无返回地址）`, icon: 'fail' })
          continue
        }
        onChange([...urls, url])
        urls = [...urls, url]
        Toast.show({ content: `${f.name} 上传成功`, icon: 'success' })
      } catch (e) {
        Toast.show({ content: e instanceof Error ? e.message : '上传失败', icon: 'fail' })
      } finally {
        setUploadingCount((c) => c - 1)
      }
    }
  }

  const [viewerUrl, setViewerUrl] = useState<string | null>(null)

  return (
    <div>
      {description && (
        <div style={{ color: '#999', fontSize: 12, marginBottom: 4 }}>{description}</div>
      )}
      {urls.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 8 }}>
          {urls.map((u) => (
            <img
              key={u}
              src={u}
              style={{ width: 64, height: 64, objectFit: 'cover', borderRadius: 4 }}
              onClick={() => setViewerUrl(u)}
            />
          ))}
        </div>
      )}
      {viewerUrl && (
        <ImageViewer image={viewerUrl} visible onClose={() => setViewerUrl(null)} />
      )}
      <label style={{ fontSize: 12, color: '#104186' }}>
        <input
          type="file"
          hidden
          accept={accept}
          multiple
          onChange={(e) => {
            if (e.target.files?.length) handleFiles(e.target.files)
            e.target.value = ''
          }}
        />
        {uploadingCount > 0 ? `上传中(${uploadingCount})...` : `点击上传（最多${maxCount}个，单个≤${maxSizeMb}MB）`}
      </label>
      {urls.length > 0 && (
        <div style={{ marginTop: 4 }}>
          {urls.map((u, i) => (
            <div key={u} style={{ fontSize: 12, color: '#666', display: 'flex', justifyContent: 'space-between' }}>
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {u.split('/').pop()}
              </span>
              <span
                style={{ color: '#ff3141', cursor: 'pointer' }}
                onClick={() => onChange(urls.filter((_, j) => j !== i))}
              >
                删除
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
