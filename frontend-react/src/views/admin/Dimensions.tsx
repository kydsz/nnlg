import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Tabs,
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  App,
  Popconfirm,
  Tag,
} from 'antd'
import { PlusOutlined, ArrowUpOutlined, ArrowDownOutlined, SettingOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { dimensionApi, type DimPayload } from '@/api/modules/dimensions'
import type { Dimension, DimensionGroup, FieldType } from '@/api/types'

interface DimOption {
  label: string
  value: string
}

/** 选项值归一化为 {label, value} 数组（兼容历史上被存成 JSON 字符串的情况） */
export function toOptionList(v: unknown): DimOption[] {
  let raw = v
  if (typeof raw === 'string') {
    try {
      raw = JSON.parse(raw)
    } catch {
      raw = null
    }
  }
  if (!Array.isArray(raw)) return []
  return raw
    .map((o): DimOption | null => {
      if (o && typeof o === 'object') {
        const label = String((o as Record<string, unknown>).label ?? '')
        return { label, value: String((o as Record<string, unknown>).value ?? label) }
      }
      const s = String(o).trim()
      return s ? { label: s, value: s } : null
    })
    .filter((o): o is DimOption => o !== null)
}

/** 规范化选项：value 留空（含空白）时自动取 label，过滤掉空文本项 */
export function normalizeOptions(v: unknown): DimOption[] {
  return toOptionList(v)
    .map((o) => ({ label: o.label.trim(), value: o.value.trim() }))
    .filter((o) => o.label !== '')
    .map((o) => (o.value === '' ? { ...o, value: o.label } : o))
}

/** field_config 摘要（列表页人话展示） */
export function configSummary(v: unknown): string {
  if (v == null || typeof v !== 'object') return v == null ? '-' : String(v)
  const cfg = v as Record<string, unknown>
  const parts: string[] = []
  const opts = toOptionList(cfg.options)
  if (opts.length > 0) parts.push(`选项：${opts.map((o) => o.label).join('、')}`)
  if (cfg.min_score != null || cfg.max_score != null)
    parts.push(`分值：${cfg.min_score ?? 0} ~ ${cfg.max_score ?? '-'}`)
  if (cfg.step != null && cfg.step !== 1) parts.push(`步长：${cfg.step}`)
  if (cfg.placeholder) parts.push(`提示：${cfg.placeholder}`)
  if (cfg.min_value != null || cfg.max_value != null)
    parts.push(`范围：${cfg.min_value ?? '-'} ~ ${cfg.max_value ?? '-'}`)
  if (parts.length === 0) {
    const s = JSON.stringify(cfg)
    return s === '{}' ? '-' : s
  }
  return parts.join('；')
}

const FIELD_TYPES = [
  { label: '评分', value: 'score' },
  { label: '单选', value: 'single_choice' },
  { label: '多选', value: 'multiple_choice' },
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '日期', value: 'date' },
  { label: '日期时间', value: 'datetime' },
  { label: '富文本', value: 'rich_text' },
  { label: '图片', value: 'image' },
  { label: '文件', value: 'file' },
]

export default function Dimensions() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [activeGroup, setActiveGroup] = useState<number | undefined>(undefined)
  const [dimModal, setDimModal] = useState(false)
  const [groupModal, setGroupModal] = useState(false)
  const [groupManageOpen, setGroupManageOpen] = useState(false)
  const [editingDim, setEditingDim] = useState<Dimension | null>(null)
  const [editingGroup, setEditingGroup] = useState<DimensionGroup | null>(null)
  const [dimForm] = Form.useForm()
  const [groupForm] = Form.useForm()

  const { data: groups } = useQuery({
    queryKey: ['dimension-groups'],
    queryFn: () => dimensionApi.groupList(),
  })
  const { data: dims, isLoading } = useQuery({
    queryKey: ['dimensions', activeGroup],
    queryFn: () => dimensionApi.list({ group_id: activeGroup }),
  })

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['dimensions'] })
    qc.invalidateQueries({ queryKey: ['dimension-groups'] })
  }

  const saveGroupMut = useMutation({
    mutationFn: (values: { code: string; name: string; sort_order?: number }) =>
      editingGroup
        ? dimensionApi.groupUpdate(editingGroup.id, values)
        : dimensionApi.groupCreate(values),
    onSuccess: () => {
      message.success('保存成功')
      setGroupModal(false)
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const delGroupMut = useMutation({
    mutationFn: (id: number) => dimensionApi.groupDelete(id),
    onSuccess: (_, id) => {
      message.success('删除成功')
      if (activeGroup === id) setActiveGroup(undefined)
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  const saveDimMut = useMutation({
    mutationFn: (values: DimPayload) =>
      editingDim ? dimensionApi.update(editingDim.id, values) : dimensionApi.create(values),
    onSuccess: () => {
      message.success('保存成功')
      setDimModal(false)
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  /** 提交前规范化：value 留空自动取 label，过滤空文本项 */
  const onSubmitDim = (values: DimPayload) => {
    const cfg = values.field_config
    if (cfg && Array.isArray(cfg.options)) cfg.options = normalizeOptions(cfg.options)
    saveDimMut.mutate(values)
  }

  const delDimMut = useMutation({
    mutationFn: (id: number) => dimensionApi.remove(id),
    onSuccess: () => {
      message.success('删除成功')
      invalidate()
    },
    onError: (e) => message.error(e.message),
  })

  // ── 排序 ──
  const groupList = groups?.list || []
  const swapGroup = (index: number, dir: -1 | 1) => {
    const target = index + dir
    if (target < 0 || target >= groupList.length) return
    const items = groupList.map((g) => ({ id: g.id, sort_order: g.sort_order ?? 0 }))
    const tmp = items[index].sort_order
    items[index].sort_order = items[target].sort_order
    items[target].sort_order = tmp
    dimensionApi
      .groupSort(items)
      .then(
        () => {
          message.success('已排序')
          invalidate()
        },
        (e) => message.error(e.message)
      )
  }

  const dimList = dims?.list || []
  const swapDim = (index: number, dir: -1 | 1) => {
    const target = index + dir
    if (target < 0 || target >= dimList.length) return
    const items = dimList.map((d) => ({ id: d.id, sort_order: d.sort_order ?? 0 }))
    const tmp = items[index].sort_order
    items[index].sort_order = items[target].sort_order
    items[target].sort_order = tmp
    dimensionApi
      .sort(items)
      .then(
        () => {
          message.success('已排序')
          invalidate()
        },
        (e) => message.error(e.message)
      )
  }

  const columns: ColumnsType<Dimension> = [
    { title: '排序', dataIndex: 'sort_order', width: 70 },
    { title: '名称', dataIndex: 'name', width: 180 },
    { title: '编码', dataIndex: 'code', width: 200 },
    {
      title: '类型',
      dataIndex: 'field_type',
      width: 110,
      render: (v: string) => (
        <Tag>{FIELD_TYPES.find((t) => t.value === v)?.label || v}</Tag>
      ),
    },
    {
      title: '必填',
      dataIndex: 'is_required',
      width: 70,
      render: (v: boolean) => (v ? '是' : '否'),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 70,
      render: (v: number) => (v === 1 ? '启用' : '禁用'),
    },
    { title: '配置', dataIndex: 'field_config', ellipsis: true, render: (v: unknown) => configSummary(v) },
    {
      title: '操作',
      width: 190,
      render: (_, record, index) => (
        <Space>
          <Button
            size="small"
            icon={<ArrowUpOutlined />}
            disabled={index === 0}
            onClick={() => swapDim(index, -1)}
          />
          <Button
            size="small"
            icon={<ArrowDownOutlined />}
            disabled={index === dimList.length - 1}
            onClick={() => swapDim(index, 1)}
          />
          <Button
            size="small"
            onClick={() => {
              setEditingDim(record)
              const cfg = record.field_config
              dimForm.setFieldsValue({
                ...record,
                field_config: cfg ? { ...cfg, options: toOptionList(cfg.options) } : { options: [] },
              })
              setDimModal(true)
            }}
          >
            编辑
          </Button>
          <Popconfirm title="确认删除？" onConfirm={() => delDimMut.mutate(record.id)}>
            <Button size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const groupItems = [
    { key: 'all', label: '全部' },
    ...groupList.map((g) => ({ key: String(g.id), label: g.name })),
  ]

  return (
    <div>
      <h3 className="page-title">评教维度</h3>
      <div className="filter-bar">
        <Button icon={<SettingOutlined />} onClick={() => setGroupManageOpen(true)}>
          分组管理
        </Button>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditingDim(null)
            dimForm.resetFields()
            dimForm.setFieldsValue({ group_id: activeGroup, field_type: 'score' })
            setDimModal(true)
          }}
        >
          新增维度
        </Button>
      </div>
      <Tabs
        activeKey={activeGroup ? String(activeGroup) : 'all'}
        items={groupItems}
        onChange={(key) => setActiveGroup(key === 'all' ? undefined : Number(key))}
      />
      <Table<Dimension>
        rowKey="id"
        loading={isLoading}
        columns={columns}
        dataSource={dims?.list}
        pagination={false}
      />

      {/* 分组管理弹窗 */}
      <Modal
        title="分组管理"
        open={groupManageOpen}
        footer={null}
        onCancel={() => setGroupManageOpen(false)}
        width={560}
      >
        <div style={{ marginBottom: 12, textAlign: 'right' }}>
          <Button
            icon={<PlusOutlined />}
            onClick={() => {
              setEditingGroup(null)
              groupForm.resetFields()
              setGroupModal(true)
            }}
          >
            新增分组
          </Button>
        </div>
        <Table<DimensionGroup>
          rowKey="id"
          size="small"
          dataSource={groupList}
          pagination={false}
          columns={[
            { title: '分组名称', dataIndex: 'name' },
            { title: '排序', dataIndex: 'sort_order', width: 70 },
            {
              title: '操作',
              width: 220,
              render: (_, g, gi) => (
                <Space>
                  <Button
                    size="small"
                    icon={<ArrowUpOutlined />}
                    disabled={gi === 0}
                    onClick={() => swapGroup(gi, -1)}
                  />
                  <Button
                    size="small"
                    icon={<ArrowDownOutlined />}
                    disabled={gi === groupList.length - 1}
                    onClick={() => swapGroup(gi, 1)}
                  />
                  <Button
                    size="small"
                    onClick={() => {
                      setEditingGroup(g)
                      groupForm.setFieldsValue(g)
                      setGroupModal(true)
                    }}
                  >
                    编辑
                  </Button>
                  <Popconfirm title="确认删除该分组？" onConfirm={() => delGroupMut.mutate(g.id)}>
                    <Button size="small" danger>
                      删除
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Modal>

      {/* 分组弹窗 */}
      <Modal
        title={editingGroup ? '编辑分组' : '新增分组'}
        open={groupModal}
        onCancel={() => setGroupModal(false)}
        onOk={() => groupForm.submit()}
        confirmLoading={saveGroupMut.isPending}
      >
        <Form form={groupForm} layout="vertical" onFinish={(v) => saveGroupMut.mutate(v)}>
          <Form.Item name="name" label="分组名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="分组编码" rules={[{ required: true, message: '请输入编码' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="sort_order" label="排序号">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      {/* 维度弹窗 */}
      <Modal
        title={editingDim ? '编辑维度' : '新增维度'}
        open={dimModal}
        onCancel={() => setDimModal(false)}
        onOk={() => dimForm.submit()}
        confirmLoading={saveDimMut.isPending}
        width={560}
      >
        <Form form={dimForm} layout="vertical" onFinish={onSubmitDim}>
          <Form.Item name="group_id" label="所属分组">
            <Select
              allowClear
              options={(groups?.list || []).map((g) => ({ label: g.name, value: g.id }))}
            />
          </Form.Item>
          <Form.Item name="name" label="维度名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="维度编码" rules={[{ required: true, message: '请输入编码' }]}>
            <Input disabled={!!editingDim} />
          </Form.Item>
          <Form.Item
            name="field_type"
            label="字段类型"
            rules={[{ required: true, message: '请选择类型' }]}
          >
            <Select options={FIELD_TYPES} />
          </Form.Item>
          <FieldTypeConfigForm form={dimForm} />
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} />
          </Form.Item>
          <Form.Item name="sort_order" label="排序号">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="is_required" label="必填">
            <SwitchAdapter />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select
              options={[
                { label: '启用', value: 1 },
                { label: '禁用', value: 0 },
              ]}
              style={{ width: 120 }}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

/** 根据字段类型动态渲染 field_config 表单项 */
function FieldTypeConfigForm({ form }: { form: ReturnType<typeof Form.useForm>[0] }) {
  const fieldType = Form.useWatch('field_type', form)
  if (fieldType === 'score') {
    return (
      <Space style={{ display: 'flex' }} align="start">
        <Form.Item name={['field_config', 'min_score']} label="最低分" initialValue={0}>
          <InputNumber min={0} />
        </Form.Item>
        <Form.Item name={['field_config', 'max_score']} label="最高分" initialValue={20}>
          <InputNumber min={0} />
        </Form.Item>
        <Form.Item name={['field_config', 'step']} label="步长" initialValue={1}>
          <InputNumber min={0.1} step={0.1} />
        </Form.Item>
      </Space>
    )
  }
  if (fieldType === 'single_choice' || fieldType === 'multiple_choice') {
    return (
      <Form.List name={['field_config', 'options']}>
        {(fields, { add, remove }) => (
          <>
            {fields.map((field, index) => (
              <div
                key={field.key}
                style={{ display: 'flex', gap: 8, alignItems: 'baseline' }}
              >
                <Form.Item
                  name={[field.name, 'label']}
                  label={index === 0 ? '选项文本' : ''}
                  style={{ flex: 1 }}
                  rules={[{ required: true, message: '请填写选项文本' }]}
                >
                  <Input placeholder="选项文本（显示给用户）" />
                </Form.Item>
                <Form.Item
                  name={[field.name, 'value']}
                  label={index === 0 ? '选项值（可留空）' : ''}
                  style={{ flex: 1 }}
                >
                  <Input placeholder="留空则同选项文本" />
                </Form.Item>
                <Form.Item>
                  <Button type="text" danger onClick={() => remove(field.name)}>
                    删除
                  </Button>
                </Form.Item>
              </div>
            ))}
            <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>
              添加选项
            </Button>
          </>
        )}
      </Form.List>
    )
  }
  if (fieldType === 'text' || fieldType === 'rich_text') {
    return (
      <Form.Item name={['field_config', 'placeholder']} label="占位提示">
        <Input />
      </Form.Item>
    )
  }
  if (fieldType === 'number') {
    return (
      <Space>
        <Form.Item name={['field_config', 'min_value']} label="最小值">
          <InputNumber />
        </Form.Item>
        <Form.Item name={['field_config', 'max_value']} label="最大值">
          <InputNumber />
        </Form.Item>
      </Space>
    )
  }
  return null
}

/** 必填下拉（后端 is_required 为 bool）；必须透传 props 供 Form.Item 注入 value/onChange */
function SwitchAdapter(props: Record<string, unknown>) {
  return (
    <Select
      {...props}
      options={[
        { label: '必填', value: true },
        { label: '选填', value: false },
      ]}
      style={{ width: 120 }}
    />
  )
}

export type { FieldType }
