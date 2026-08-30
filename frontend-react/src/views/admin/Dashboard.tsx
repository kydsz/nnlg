import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, Col, Row, Statistic, Spin, Tag } from 'antd'
import {
  TeamOutlined,
  FileTextOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
} from '@ant-design/icons'
import { statsApi } from '@/api/modules/stats'
import { scheduleApi } from '@/api/modules/schedule'
import { useAuthStore } from '@/stores/auth'
import { roleNamesStr } from '@/utils/roleNames'
import { formatSemester } from '@/utils/format'
import { useNavigate } from 'react-router-dom'
import type { ECharts } from 'echarts'

const QUICK_LINKS: { path: string; label: string }[] = [
  { path: '/admin/course-schedule', label: '学期配置与课表查询' },
  { path: '/admin/data-sync', label: '数据同步' },
  { path: '/admin/users', label: '用户管理' },
  { path: '/admin/stats', label: '统计报表' },
  { path: '/admin/tasks', label: '评教任务' },
  { path: '/admin/dimensions', label: '评教维度' },
]

export default function Dashboard() {
  const user = useAuthStore((s) => s.user)
  const navigate = useNavigate()
  const donutRef = useRef<HTMLDivElement>(null)
  const barRef = useRef<HTMLDivElement>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'overview'],
    queryFn: () => statsApi.overview(),
  })
  const { data: currentSemester } = useQuery({
    queryKey: ['current-semester'],
    queryFn: () => scheduleApi.currentSemester(),
  })

  // 图表（动态 import echarts，对齐旧版按需加载）
  useEffect(() => {
    if (!data) return
    let chart: ECharts | null = null
    let disposed = false
    ;(async () => {
      const echarts = await import('echarts')
      if (disposed) return
      // 环形图：任务完成情况
      if (donutRef.current) {
        const d = echarts.init(donutRef.current)
        d.setOption({
          tooltip: { trigger: 'item' },
          legend: { bottom: 0 },
          series: [
            {
              type: 'pie',
              radius: ['45%', '70%'],
              avoidLabelOverlap: false,
              itemStyle: { borderRadius: 6 },
              label: { show: false },
              data: [
                { value: data.tasks.evaluated, name: '已评' },
                { value: data.tasks.pending, name: '待评' },
              ],
            },
          ],
        })
        chart = d
      }
      // 柱状图：评教数据概览
      if (barRef.current) {
        const b = echarts.init(barRef.current)
        b.setOption({
          tooltip: { trigger: 'axis' },
          grid: { left: 40, right: 20, top: 20, bottom: 30 },
          xAxis: { type: 'category', data: ['教师', '督导', '评教记录'] },
          yAxis: { type: 'value' },
          series: [
            {
              type: 'bar',
              barWidth: '40%',
              itemStyle: {
                borderRadius: [4, 4, 0, 0],
                color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                  { offset: 0, color: '#2563eb' },
                  { offset: 1, color: '#93c5fd' },
                ]),
              },
              data: [data.users.teachers, data.users.supervisors, data.evaluations.total],
            },
          ],
        })
      }
    })()
    const onResize = () => chart?.resize()
    window.addEventListener('resize', onResize)
    return () => {
      disposed = true
      window.removeEventListener('resize', onResize)
      chart?.dispose()
    }
  }, [data])

  if (isLoading || !data) return <Spin style={{ display: 'block', margin: '80px auto' }} />

  const rate = data.tasks.evaluation_rate

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <h3 className="page-title" style={{ marginBottom: 4 }}>
          你好，{user?.username}
        </h3>
        <Tag color="blue">{roleNamesStr(user?.roles)}</Tag>
        <Tag style={{ marginLeft: 8 }}>
          当前学期：{currentSemester ? formatSemester(currentSemester.semester) : '未设置'}
        </Tag>
        <Tag color="green">系统状态正常</Tag>
      </div>

      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="教师总数" value={data.users.teachers} prefix={<TeamOutlined />} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="评教记录" value={data.evaluations.total} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="待评任务"
              value={data.tasks.pending}
              prefix={<ClockCircleOutlined />}
            />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic
              title="完成率"
              value={rate}
              precision={1}
              suffix="%"
              prefix={<CheckCircleOutlined />}
            />
          </Card>
        </Col>

        <Col xs={24} md={12}>
          <Card title="任务完成情况" size="small">
            <div ref={donutRef} style={{ height: 280 }} />
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card title="评教数据概览" size="small">
            <div ref={barRef} style={{ height: 280 }} />
          </Card>
        </Col>

        <Col xs={24}>
          <Card size="small" title="评教进度">
            <div style={{ marginBottom: 8 }}>
              已评 {data.tasks.evaluated} / {data.tasks.total}（{rate}%）
            </div>
            <div
              style={{ height: 10, background: '#f0f0f0', borderRadius: 5, overflow: 'hidden' }}
            >
              <div
                style={{
                  width: `${Math.min(rate, 100)}%`,
                  height: '100%',
                  background: '#2563eb',
                  transition: 'width .4s',
                }}
              />
            </div>
            <div style={{ marginTop: 12, color: '#666', fontSize: 13 }}>
              校区 {data.organization.campuses} · 学院 {data.organization.colleges} · 督导{' '}
              {data.users.supervisors}
            </div>
          </Card>
        </Col>

        <Col xs={24}>
          <Card size="small" title="快捷入口">
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {QUICK_LINKS.map((l) => (
                <a
                  key={l.path}
                  onClick={() => navigate(l.path)}
                  style={{
                    padding: '6px 14px',
                    background: '#f0f5ff',
                    borderRadius: 6,
                    color: '#2563eb',
                    cursor: 'pointer',
                  }}
                >
                  {l.label}
                </a>
              ))}
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
