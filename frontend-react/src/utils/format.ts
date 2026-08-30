import dayjs, { type Dayjs } from 'dayjs'

/** "2024-2025-1" → "2024-2025学年第1学期" */
export function formatSemester(semester: string): string {
  const m = semester.match(/^(\d{4})-(\d{4})-(\d)$/)
  if (!m) return semester
  return `${m[1]}-${m[2]}学年第${m[3]}学期`
}

/** 生成学年学期列表（如 2023-2030 各 2 学期），倒序 */
export function generateSemesters(startYear = 2023, endYear = 2030): string[] {
  const list: string[] = []
  for (let y = endYear; y >= startYear; y--) {
    list.push(`${y}-${y + 1}-1`, `${y}-${y + 1}-2`)
  }
  return list
}

export function formatDate(v: string | null | undefined, withTime = true): string {
  if (!v) return '-'
  return withTime ? dayjs(v).format('YYYY-MM-DD HH:mm') : dayjs(v).format('YYYY-MM-DD')
}

/** 拆分节次字符串 "0102" → ["01","02"] */
export function parseSections(section: string): string[] {
  if (!section) return []
  const out: string[] = []
  const re = /(\d{2})/g
  let m: RegExpExecArray | null
  while ((m = re.exec(section))) out.push(m[1])
  return out
}

export const WEEK_DAYS = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']

export const TIME_SLOTS = [
  { label: '第1-2节', time: '08:30-10:05' },
  { label: '第3-4节', time: '10:25-12:00' },
  { label: '第5-6节', time: '14:30-16:05' },
  { label: '第7-8节', time: '16:15-17:50' },
  { label: '第9-10节', time: '18:20-19:55' },
  { label: '第11-12节', time: '20:05-21:40' },
]

/** 由上课时间（datetime）推算节次，如 "第3-4节"；无法匹配返回 null */
export function formatClassPeriod(classTime: string | null | undefined): string | null {
  if (!classTime) return null
  const t = dayjs(classTime)
  if (!t.isValid()) return null
  const hm = t.format('HH:mm')
  for (const slot of TIME_SLOTS) {
    const [start, end] = slot.time.split('-')
    if (hm >= start && hm <= end) return slot.label
  }
  return null
}

export const TASK_STATUS = { PENDING: 1, DONE: 2, CANCELLED: 3 } as const

export function taskStatusInfo(status: number): { text: string; color: string } {
  switch (status) {
    case 1:
      return { text: '待评', color: 'orange' }
    case 2:
      return { text: '已评', color: 'green' }
    case 3:
      return { text: '已取消', color: 'red' }
    default:
      return { text: `状态${status}`, color: 'default' }
  }
}

/* ═══ 周次工具（对齐旧前端 / 后端 parse_week_pattern）═══ */

/** 解析周次模式为具体周集合；无法解析时返回 null 表示"恒真"（全周）
 *  weeks：学期总周数，单/双周按不超过该周数生成，默认 20 */
export function parseWeekPattern(
  pattern: string | null | undefined,
  weeks = 20
): number[] | null {
  if (!pattern) return null
  const p = pattern.trim()
  const total = weeks <= 0 ? 20 : weeks
  if (!p || p === '全周' || p === '全部' || p === '每周') return null
  if (p === '单周' || p === '奇数周') {
    return Array.from({ length: Math.ceil(total / 2) }, (_, i) => i * 2 + 1) // 1,3,...(≤ total)
  }
  if (p === '双周' || p === '偶数周') {
    return Array.from({ length: Math.floor(total / 2) }, (_, i) => i * 2 + 2) // 2,4,...(≤ total)
  }
  const weeksSet = new Set<number>()
  // 形如 "1-12周"、"1-2,4,7-8周"、"9周"（可能带"周"字后缀与逗号/顿号分隔）
  const re = /(\d+)\s*[-–~]\s*(\d+)|(\d+)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(p))) {
    if (m[1] && m[2]) {
      for (let i = Number(m[1]); i <= Number(m[2]); i++) weeksSet.add(i)
    } else if (m[3]) {
      weeksSet.add(Number(m[3]))
    }
  }
  return weeksSet.size > 0 ? [...weeksSet].sort((a, b) => a - b) : null
}

/** 课程在指定周是否上课（weeks 为学期总周数，默认 20） */
export function isCourseInWeek(
  weekPattern: string | null | undefined,
  week: number,
  weeks = 20
): boolean {
  const set = parseWeekPattern(weekPattern, weeks)
  return set === null || set.includes(week)
}

/** 按学期号推导默认起始日：第1学期 = 起始学年 09-01；第2学期 = 结束学年 02-17 */
export function defaultSemesterStart(semester: string): string {
  const m = semester.match(/^(\d{4})-(\d{4})-(\d)$/)
  if (!m) return dayjs().format('YYYY-MM-DD')
  const [, y1, y2, term] = m
  return term === '2' ? `${y2}-02-17` : `${y1}-09-01`
}

/** 当前教学周：起始日起第 1 周，限定在学期总周数内（默认 20） */
export function currentWeekOf(startDate: string, weeks = 20): number {
  const diff = dayjs().startOf('day').diff(dayjs(startDate).startOf('day'), 'day')
  const week = Math.floor(diff / 7) + 1
  return Math.min(Math.max(week, 1), weeks || 20)
}

/** 第 week 周中 weekDay(1=周一..7=周日) 的日期 */
export function getDateForWeekAndDay(startDate: string, week: number, weekDay: number): Dayjs {
  const start = dayjs(startDate)
  // 起始日所在周（周一起算）
  const mondayOffset = start.day() === 0 ? -6 : 1 - start.day()
  const monday = start.add(mondayOffset, 'day')
  return monday.add((week - 1) * 7 + (weekDay - 1), 'day')
}

/** "0102" → "第1-2节"；"05" → "第5节" */
export function formatSectionText(section: string | null | undefined): string {
  const nums = parseSections(section || '')
  if (nums.length === 0) return '-'
  const first = Number(nums[0])
  const last = Number(nums[nums.length - 1])
  return first === last ? `第${first}节` : `第${first}-${last}节`
}
