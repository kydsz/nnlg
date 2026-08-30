import { useMemo } from 'react'
import { Spin, Empty, Popover, Tag } from 'antd'
import { WEEK_DAYS, TIME_SLOTS, parseSections, formatSemester, formatDate } from '@/utils/format'
import type { CourseItem } from '@/api/types'

export interface ScheduleMeta {
  teacher_name?: string
  semester?: string
  version?: number
  student_count?: number | null
  crawl_time?: string | null
}

interface CellCourse {
  course_name: string
  classroom?: string
  week_pattern?: string
  class_info?: string
  student_count?: number | string | null
}

interface Props {
  courses: CourseItem[] | undefined
  loading?: boolean
  /** 课表元信息统计条（版本/人数/抓取时间），id=0 时不显示 */
  meta?: ScheduleMeta | null
  /** 点击课程单元回调（自定义弹层）；不传则用内置 Popover */
  onCellClick?: (courses: CellCourse[]) => void
}

/** 课表网格：7 天 × 6 时段 */
export default function ScheduleGrid({ courses, loading, meta, onCellClick }: Props) {
  const courseMap = useMemo(() => {
    const map = new Map<string, CellCourse[]>()
    if (!courses) return map
    for (const c of courses) {
      const sections = parseSections(String(c.section ?? ''))
      for (const s of sections) {
        const key = `${c.week_day}-${s}`
        const arr = map.get(key) || []
        const item: CellCourse = {
          course_name: c.course_name,
          classroom: c.classroom,
          week_pattern: c.week_pattern,
          class_info: c.class_info,
          student_count: c.student_count,
        }
        // 同一小节内按课程身份键去重（同课名不同班级/教师/教室视为不同课程）
        if (!arr.some((x) => cellKey(x) === cellKey(item))) {
          arr.push(item)
        }
        map.set(key, arr)
      }
    }
    return map
  }, [courses])

  if (loading) return <Spin style={{ display: 'block', margin: '40px auto' }} />

  // 统计条（对齐旧版：有课表数据时显示 版本/人数/节数/抓取时间）
  const statsBar = meta && meta.version ? (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: 8,
        alignItems: 'center',
        marginBottom: 8,
        fontSize: 13,
      }}
    >
      {meta.teacher_name && <strong>{meta.teacher_name}</strong>}
      {meta.semester && <Tag>{formatSemester(meta.semester)}</Tag>}
      <Tag color="blue">V{meta.version}</Tag>
      {meta.student_count != null && <span>应到 {meta.student_count} 人</span>}
      <span>共 {courses?.length ?? 0} 节</span>
      {meta.crawl_time && (
        <span style={{ color: '#999' }}>更新于 {formatDate(meta.crawl_time)}</span>
      )}
    </div>
  ) : null

  if (!courses || courses.length === 0) {
    return (
      <>
        {statsBar}
        <Empty description="无课程数据" style={{ margin: '40px 0' }} />
      </>
    )
  }

  const renderCell = (cells: CellCourse[] | undefined) => {
    if (!cells || cells.length === 0) return null
    if (cells.length === 1) {
      const c = cells[0]
      return (
        <>
          <div style={{ ...lineStyle, fontWeight: 500 }}>{c.course_name}</div>
          {c.classroom && (
            <div style={{ ...lineStyle, fontSize: 11, opacity: 0.8 }}>{c.classroom}</div>
          )}
          {c.week_pattern && (
            <div style={{ ...lineStyle, fontSize: 11, opacity: 0.7 }}>{c.week_pattern}</div>
          )}
        </>
      )
    }
    return (
      <>
        <div style={{ ...lineStyle, fontWeight: 500 }}>{cells.length} 门课程</div>
        {cells.slice(0, 2).map((c, i) => (
          <div key={i} style={{ ...lineStyle, fontSize: 11, opacity: 0.8 }}>
            {c.course_name}
          </div>
        ))}
      </>
    )
  }

  return (
    <div>
      {statsBar}
      <div style={{ overflowX: 'auto' }}>
      <table
        style={{
          minWidth: 900,
          width: '100%',
          tableLayout: 'fixed',
          borderCollapse: 'collapse',
          background: '#fff',
        }}
      >
        <thead>
          <tr>
            <th style={thStyle}>节次</th>
            {WEEK_DAYS.map((d) => (
              <th key={d} style={thStyle}>
                {d}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {TIME_SLOTS.map((slot, rowIdx) => (
            <tr key={slot.label}>
              <td style={{ ...tdStyle, fontSize: 12, background: '#fafafa' }}>
                <div>{slot.label}</div>
              </td>
              {WEEK_DAYS.map((_, colIdx) => {
                const weekDay = colIdx + 1
                const firstSection = rowIdx * 2 + 1
                // 聚合该时段（两小节）内的课程
                const cells: CellCourse[] = []
                for (const s of [firstSection, firstSection + 1]) {
                  const arr = courseMap.get(`${weekDay}-${String(s).padStart(2, '0')}`)
                  if (arr) {
                    // 两小节合成一个大节：按课程身份键去重，同课名不同班级/教师/教室不算重复
                    for (const c of arr) {
                      if (!cells.some((x) => cellKey(x) === cellKey(c))) cells.push(c)
                    }
                  }
                }
                const key = `${weekDay}-${rowIdx}`
                if (cells.length === 0) {
                  return <td key={key} style={tdStyle} />
                }
                const content = (
                  <div
                    style={{
                      ...cellBoxStyle,
                      background: cells.length > 1 ? '#fff7e6' : '#e6f4ff',
                      border: `1px solid ${cells.length > 1 ? '#ffd591' : '#91caff'}`,
                    }}
                  >
                    {renderCell(cells)}
                  </div>
                )
                return (
                  <td key={key} style={{ height: ROW_HEIGHT, padding: 2, border: 'none' }}>
                    {onCellClick ? (
                      <div style={{ height: '100%' }} onClick={() => onCellClick(cells)}>
                        {content}
                      </div>
                    ) : (
                      <Popover
                        content={
                          <div style={{ maxWidth: 280 }}>
                            {cells.map((c, i) => (
                              <div key={i} style={{ marginBottom: 8 }}>
                                <div style={{ fontWeight: 600 }}>{c.course_name}</div>
                                {c.class_info && <div>{c.class_info}</div>}
                                {c.classroom && <div>教室：{c.classroom}</div>}
                                {c.week_pattern && <div>周次：{c.week_pattern}</div>}
                                {c.student_count != null && <div>人数：{c.student_count}</div>}
                              </div>
                            ))}
                          </div>
                        }
                        trigger="click"
                      >
                        {content}
                      </Popover>
                    )}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
      </div>
    </div>
  )
}

const thStyle: React.CSSProperties = {
  border: '1px solid #eee',
  padding: 8,
  background: '#fafafa',
  fontWeight: 500,
}

/**
 * 课程身份去重键：用课程名 + 教室 + 班级信息 + 周次联合标识一门课。
 * 同课名不同班级/教师/教室视为不同课程；同一门课跨两小节拆出的多条记录键相同，可正确去重。
 */
const cellKey = (c: CellCourse) =>
  [c.course_name, c.classroom, c.class_info, c.week_pattern].join('|')

/** 统一行高：保证每个课程格尺寸一致，不随内容变化 */
const ROW_HEIGHT = 72

const tdStyle: React.CSSProperties = {
  border: '1px solid #eee',
  padding: 6,
  verticalAlign: 'top',
  height: ROW_HEIGHT,
  fontSize: 12,
}

/** 课程色块：填满单元格、超出截断 */
const cellBoxStyle: React.CSSProperties = {
  width: '100%',
  height: '100%',
  boxSizing: 'border-box',
  padding: 6,
  overflow: 'hidden',
  borderRadius: 4,
  display: 'flex',
  flexDirection: 'column',
  gap: 2,
  cursor: 'pointer',
}

/** 单行截断 */
const lineStyle: React.CSSProperties = {
  whiteSpace: 'nowrap',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
}
