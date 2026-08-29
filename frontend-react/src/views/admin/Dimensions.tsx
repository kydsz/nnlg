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
    mutationFn: (values: { name: string; sort_order?: number }) =>
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
    const ids = groupList.map((g) => g.id)
    ;[ids[index], ids[target]] = [ids[target], ids[index]]
    dimensionApi.groupSort(ids).then(
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
    const ids = dimList.map((d) => d.id)
    ;[ids[index], ids[target]] = [ids[target], ids[index]]
    dimensionApi.sort(ids).then(
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
    { title: '配置', dataIndex: 'field_config', ellipsis: true, render: (v: unknown) => JSON.stringify(v) },
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
              dimForm.setFieldsValue(record)
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
        <Form form={dimForm} layout="vertical" onFinish={(v) => saveDimMut.mutate(v)}>
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
          <Form.Item name="is_required" label="必填" valuePropName="checked">
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
          <InputNumber min={1} />
        </Form.Item>
      </Space>
    )
  }
  if (fieldType === 'single_choice' || fieldType === 'multiple_choice') {
    return (
      <Form.Item
        name={['field_config', 'options']}
        label="选项（每行一个：label|value）"
        tooltip="如：优秀|excellent"
      >
        <Input.TextArea
          rows={3}
          placeholder={'优秀|excellent\n良好|good'}
          onBlur={(e) => {
            // 存为 JSON 结构
            const lines = e.target.value.split('\n').filter(Boolean)
            const options = lines.map((l) => {
              const [label, value] = l.split('|')
              return { label: label.trim(), value: (value || label).trim() }
            })
            form.setFieldValue(['field_config', 'options'], JSON.stringify(options))
          }}
        />
      </Form.Item>
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

/** 必填下拉（后端 is_required 为 bool） */
function SwitchAdapter() {
  return (
    <Select
      options={[
        { label: '必填', value: true },
        { label: '选填', value: false },
      ]}
      style={{ width: 120 }}
    />
  )
}

export type { FieldType }
