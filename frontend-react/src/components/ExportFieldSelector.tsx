import { useEffect, useState } from 'react'
import { Modal, Checkbox, Button, Space, App } from 'antd'

export const DEFAULT_EXPORT_FIELDS = [
  'record_id',
  'teacher_name',
  'course_name',
  'class_time',
  'classroom',
  'college_name',
  'evaluator_name',
  'evaluator_role',
  'submit_time',
  'total_score',
  'max_total_score',
  'is_anonymous',
  'listening_content',
  'attendance_rate',
  'TEI',
]

export const EXPORT_FIELD_LABELS: Record<string, string> = {
  record_id: '记录ID',
  teacher_name: '教师姓名',
  course_name: '课程名称',
  class_time: '上课时间',
  classroom: '教室',
  college_name: '学院',
  evaluator_name: '评教人',
  evaluator_role: '评教人角色',
  submit_time: '提交时间',
  total_score: '总分',
  max_total_score: '满分',
  is_anonymous: '是否匿名',
  listening_content: '听课内容',
  student_count: '上课人数',
  attendance_count: '出勤人数',
  attendance_rate: '出勤率(%)',
  TEI: '意见与建议',
  user_no: '工号',
  role_names: '角色',
  given_count: '评教次数',
  given_avg_score: '评教均分',
  received_count: '被评次数',
  received_avg_score: '被评均分',
  evaluation_rate: '评教率',
}

interface Props {
  open: boolean
  fields?: string[]
  defaultFields?: string[]
  onCancel: () => void
  onConfirm: (fields: string[]) => void
}

export default function ExportFieldSelector({
  open,
  fields,
  defaultFields = DEFAULT_EXPORT_FIELDS,
  onCancel,
  onConfirm,
}: Props) {
  const { message } = App.useApp()
  const [selected, setSelected] = useState<string[]>(defaultFields)

  useEffect(() => {
    if (open) setSelected(defaultFields)
  }, [open, defaultFields])

  const options = (fields || Object.keys(EXPORT_FIELD_LABELS)).map((f) => ({
    label: EXPORT_FIELD_LABELS[f] || f,
    value: f,
  }))

  return (
    <Modal
      title="选择导出字段"
      open={open}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        <Button
          key="ok"
          type="primary"
          onClick={() => {
            if (selected.length === 0) {
              message.warning('请至少选择一个字段')
              return
            }
            onConfirm(selected)
          }}
        >
          导出
        </Button>,
      ]}
    >
      <Space style={{ marginBottom: 12 }}>
        <Button size="small" onClick={() => setSelected(options.map((o) => o.value))}>
          全选
        </Button>
        <Button size="small" onClick={() => setSelected([])}>
          全不选
        </Button>
      </Space>
      <Checkbox.Group
        value={selected}
        onChange={(v) => setSelected(v as string[])}
        options={options}
        style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}
      />
    </Modal>
  )
}
