import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Spin, DatePicker } from 'antd'
import {
  TeamOutlined,
  FileTextOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  RightOutlined,
  PieChartOutlined,
  BarChartOutlined,
  RiseOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import { useNavigate } from 'react-router-dom'
import { statsApi } from '@/api/modules/stats'
import { scheduleApi } from '@/api/modules/schedule'
import { useAuthStore } from '@/stores/auth'
import { roleNamesStr } from '@/utils/roleNames'
import { formatSemester } from '@/utils/format'
import { useSemesterRangePicker } from '@/hooks/useSemesterDates'
import { MENU } from './Layout'
import type { ECharts } from 'echarts'
import './dashboard.css'

const ACCENTS = ['accent-blue', 'accent-indigo', 'accent-gold', 'accent-teal']

const TILE_GROUPS: { title: string; keys: string[] }[] = [
  { title: '评教业务', keys: ['/admin/tasks', '/admin/evaluations', '/admin/dimensions'] },
  { title: '统计分析', keys: ['/admin/stats', '/admin/teacher-evaluation-summary'] },
  {
    title: '组织与人员',
    keys: [
      '/admin/campus',
      '/admin/colleges',
      '/admin/research-rooms',
      '/admin/users',
      '/admin/roles',
    ],
  },
  { title: '数据与课表', keys: ['/admin/course-schedule', '/admin/data-sync'] },
]

function greeting() {
  const h = dayjs().hour()
  if (h < 6) return '凌晨好'
  if (h < 12) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
}

export default function Dashboard() {
  const user = useAuthStore((s) => s.user)
  const hasPermission = useAuthStore((s) => s.hasPermission)
  const navigate = useNavigate()
  const donutRef = useRef<HTMLDivElement>(null)
  const barRef = useRef<HTMLDivElement>(null)

  const { effective: dates, onRange } = useSemesterRangePicker()
  const { data, isLoading } = useQuery({
    queryKey: ['stats', 'overview', dates],
    queryFn: () => statsApi.overview({ start_date: dates[0], end_date: dates[1] }),
  })
  const { data: currentSemester } = useQuery({
    queryKey: ['current-semester'],
    queryFn: () => scheduleApi.currentSemester(),
  })

  useEffect(() => {
    if (!data) return
    let charts: ECharts[] = []
    let disposed = false
    ;(async () => {
      const echarts = await import('echarts')
      if (disposed) return
      if (donutRef.current) {
        const d = echarts.init(donutRef.current)
        d.setOption({
          tooltip: { trigger: 'item' },
          legend: { bottom: 0 },
          color: ['#104186', '#e1c46e'],
          series: [
            {
              type: 'pie',
              radius: ['48%', '72%'],
              avoidLabelOverlap: false,
              itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
              label: { show: false },
              data: [
                { value: data.tasks.evaluated, name: '已评' },
                { value: data.tasks.pending, name: '待评' },
              ],
            },
          ],
        })
        charts.push(d)
      }
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
              barWidth: '42%',
              itemStyle: {
                borderRadius: [6, 6, 0, 0],
                color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
                  { offset: 0, color: '#2f6fb5' },
                  { offset: 1, color: '#104186' },
                ]),
              },
              data: [data.users.teachers, data.users.supervisors, data.evaluations.total],
            },
          ],
        })
        charts.push(b)
      }
    })()
    const onResize = () => charts.forEach((c) => c.resize())
    window.addEventListener('resize', onResize)
    return () => {
      disposed = true
      window.removeEventListener('resize', onResize)
      charts.forEach((c) => c.dispose())
      charts = []
    }
  }, [data])

  if (isLoading || !data) return <Spin style={{ display: 'block', margin: '80px auto' }} />

  const rate = data.tasks.evaluation_rate
  const menuByKey = new Map(MENU.map((m) => [m.key, m]))

  const kpis = [
    { title: '教师总数', value: data.users.teachers, icon: <TeamOutlined />, accent: 'accent-blue' },
    {
      title: '评教记录',
      value: data.evaluations.total,
      icon: <FileTextOutlined />,
      accent: 'accent-gold',
    },
    {
      title: '待评任务',
      value: data.tasks.pending,
      icon: <ClockCircleOutlined />,
      accent: 'accent-teal',
    },
    {
      title: '完成率',
      value: `${rate}%`,
      icon: <CheckCircleOutlined />,
      accent: 'accent-indigo',
    },
  ]

  const roleText = roleNamesStr(user?.roles)
  const chips = [
    { text: roleText === '-' ? '' : roleText, gold: true },
    { text: user?.college_name || '' },
    { text: user?.research_room_name || '' },
    {
      text: `当前学期：${currentSemester ? formatSemester(currentSemester.semester) : '未设置'}`,
    },
  ].filter((c) => c.text)

  return (
    <div className="dash">
      <section className="dash-hero">
        <div className="dash-hero-main">
          <div className="dash-avatar">{(user?.username || 'U').charAt(0)}</div>
          <div className="dash-hero-info">
            <h2 className="dash-hero-name">
              {greeting()}，{user?.username}
              {user?.user_no && <span className="dash-hero-no">工号 {user.user_no}</span>}
            </h2>
            <div className="dash-hero-meta">
              {chips.map((c) => (
                <span key={c.text} className={`dash-chip ${c.gold ? 'is-gold' : ''}`}>
                  {c.text}
                </span>
              ))}
            </div>
          </div>
        </div>
        <div className="dash-hero-range">
          <DatePicker.RangePicker
            value={dates[0] && dates[1] ? [dayjs(dates[0]), dayjs(dates[1])] : undefined}
            onChange={(v) =>
              onRange(v ? [v[0]!.format('YYYY-MM-DD'), v[1]!.format('YYYY-MM-DD')] : null)
            }
          />
        </div>
      </section>

      <section className="dash-kpis">
        {kpis.map((k) => (
          <div className="dash-kpi" key={k.title}>
            <div className={`dash-kpi-icon ${k.accent}`}>{k.icon}</div>
            <div className="dash-kpi-body">
              <div className="dash-kpi-value">{k.value}</div>
              <div className="dash-kpi-label">{k.title}</div>
            </div>
          </div>
        ))}
      </section>

      {TILE_GROUPS.map((group, gi) => {
        const tiles = group.keys
          .map((key) => menuByKey.get(key))
          .filter((m): m is NonNullable<typeof m> => !!m && (!m.perm || hasPermission(m.perm)))
        if (!tiles.length) return null
        return (
          <section className="dash-group" key={group.title}>
            <div className="dash-group-head">
              <span className="dash-group-title">{group.title}</span>
              <span className="dash-group-count">{tiles.length} 项功能</span>
            </div>
            <div className="tile-grid">
              {tiles.map((m, ti) => (
                <div
                  className="tile"
                  key={m.key}
                  onClick={() => navigate(m.key)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') navigate(m.key)
                  }}
                >
                  <RightOutlined className="tile-arrow" />
                  <div className={`tile-icon ${ACCENTS[(gi + ti) % ACCENTS.length]}`}>{m.icon}</div>
                  <div className="tile-label">{m.label}</div>
                </div>
              ))}
            </div>
          </section>
        )
      })}

      <section className="dash-overview">
        <div className="dash-card">
          <div className="dash-card-title">
            <PieChartOutlined />
            任务完成情况
          </div>
          <div ref={donutRef} style={{ height: 260 }} />
        </div>
        <div className="dash-card">
          <div className="dash-card-title">
            <BarChartOutlined />
            评教数据概览
          </div>
          <div ref={barRef} style={{ height: 260 }} />
        </div>
      </section>

      <section className="dash-card">
        <div className="dash-card-title">
          <RiseOutlined />
          评教进度
        </div>
        <div className="dash-progress-track">
          <div className="dash-progress-bar" style={{ width: `${Math.min(rate, 100)}%` }} />
        </div>
        <div className="dash-progress-meta">
          已评 {data.tasks.evaluated} / {data.tasks.total}（{rate}%）
        </div>
        <div className="dash-facts">
          <div>
            <div className="dash-fact-value">{data.organization.campuses}</div>
            <div className="dash-fact-label">校区</div>
          </div>
          <div>
            <div className="dash-fact-value">{data.organization.colleges}</div>
            <div className="dash-fact-label">学院</div>
          </div>
          <div>
            <div className="dash-fact-value">{data.users.supervisors}</div>
            <div className="dash-fact-label">督导</div>
          </div>
          <div>
            <div className="dash-fact-value">{data.tasks.total}</div>
            <div className="dash-fact-label">任务总数</div>
          </div>
        </div>
      </section>

      <div className="dash-footnote">南宁理工学院 · 教学评价系统</div>
    </div>
  )
}
