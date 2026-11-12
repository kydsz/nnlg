import { useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Card, Form, InputNumber, Typography } from 'antd'
import { settingApi } from '@/api/modules/settings'

export default function Settings() {
  const { message } = App.useApp()
  const qc = useQueryClient()
  const [form] = Form.useForm<{ task_max_participants: number }>()

  const { data } = useQuery({
    queryKey: ['settings'],
    queryFn: () => settingApi.get(),
  })

  useEffect(() => {
    if (data) form.setFieldsValue(data)
  }, [data, form])

  const saveMut = useMutation({
    mutationFn: (values: { task_max_participants: number }) => settingApi.update(values),
    onSuccess: (res) => {
      message.success('保存成功')
      form.setFieldsValue(res)
      qc.invalidateQueries({ queryKey: ['settings'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
      qc.invalidateQueries({ queryKey: ['mobile-tasks'] })
    },
    onError: (e: Error) => message.error(e.message),
  })

  return (
    <div>
      <h3 className="page-title">系统设置</h3>
      <Card style={{ maxWidth: 560 }}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
          人数上限对所有评教任务全局生效，包含创建人。修改后立即生效；调低不会移除已加入的参与人，只会阻止新增。
        </Typography.Paragraph>
        <Form
          form={form}
          layout="vertical"
          initialValues={{ task_max_participants: 4 }}
          onFinish={(values) => saveMut.mutate(values)}
        >
          <Form.Item
            label="单个评教任务最多参与人数"
            name="task_max_participants"
            rules={[{ required: true, message: '请输入参与人数上限' }]}
          >
            <InputNumber min={1} max={100} precision={0} style={{ width: 200 }} addonAfter="人" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0 }}>
            <Button type="primary" htmlType="submit" loading={saveMut.isPending}>
              保存
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}
