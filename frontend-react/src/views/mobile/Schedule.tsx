import { useEffect, useMemo, useState } from 'react'
import dayjs from 'dayjs'
import { useQuery, useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import {
  NavBar,
  Button,
  Popup,
  Toast,
  SearchBar,
  List,
  ErrorBlock,
  Dialog,
  InfiniteScroll,
} from 'antd-mobile'
import { SetOutline } from 'antd-mobile-icons'
import { scheduleApi } from '@/api/modules/schedule'
import { orgApi } from '@/api/modules/org'
import { userApi } from '@/api/modules/users'
import { taskApi } from '@/api/modules/tasks'
import { useAuthStore } from '@/stores/auth'
import {
  WEEK_DAYS,
  TIME_SLOTS,
  formatSemester,
  isCourseInWeek,
  defaultSemesterStart,
  currentWeekOf,
  getDateForWeekAndDay,
  formatSectionText,
} from '@/utils/format'
import type { CourseItem, SemesterConfig, TeacherSchedule, User } from '@/api/types'
import { useDebounce } from '@/hooks/useDebounce'
import './schedule.css'

export default function Schedule() {
  const qc = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const hasPermission = useAuthStore((s) => s.hasPermission)
  const canViewUsers = hasPermission('user:view')
  const userId = user?.id

  // 学期
  const { data: configs } = useQuery({
    queryKey: ['semester-configs'],
    queryFn: () => scheduleApi.semesterConfigs(),
  })
  const { data: dynamicSemesters } = useQuery({
    queryKey: ['semesters'],
    queryFn: () => scheduleApi.semesters(),
  })
  const configList: SemesterConfig[] = configs || []
  const semesterOptions = Array.from(
    new Set([...(dynamicSemesters || []), ...configList.map((c) => c.semester)])
  )
    .sort()
    .reverse()
  const currentCfg = configList.find((c) => c.is_current)
  const [semester, setSemester] = useState<string | undefined>(undefined)
  useEffect(() => {
    if (!semester && (currentCfg?.semester || dynamicSemesters?.[0])) {
      setSemester(currentCfg?.semester || dynamicSemesters?.[0])
    }
  }, [currentCfg, dynamicSemesters, semester])

  // 教师选择（有 user:view 权限时弹层选教师）
  const [selectedTeacher, setSelectedTeacher] = useState<{ id: number; name: string } | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  useEffect(() => {
    if (canViewUsers) setPickerOpen(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canViewUsers])

  // 课表数据：选了教师 → teacher/:id；否则本人 my-schedule
  const targetTeacherId = selectedTeacher?.id ?? (canViewUsers ? undefined : userId)
  const { data: scheduleData, isLoading } = useQuery({
    queryKey: ['schedule', 'mobile', targetTeacherId, semester],
    queryFn: () =>
      targetTeacherId
        ? scheduleApi.byTeacher(targetTeacherId, semester)
        : scheduleApi.mySchedule(semester),
    enabled: !!semester && (targetTeacherId != null || !canViewUsers),
  })

  // 起始日：API → localStorage → 默认值
  const lsKey = `schedule_semester_start_${semester}`
  const [startDate, setStartDate] = useState<string | null>(null)
  const [dateOpen, setDateOpen] = useState(false)
  const { data: semCfg } = useQuery({
    queryKey: ['semester-config', semester],
    queryFn: () => scheduleApi.semesterConfig(semester!),
    enabled: !!semester,
  })
  /** 学期总周数：按配置，未配置默认 20 */
  const semesterWeeks = semCfg?.weeks || 20
  useEffect(() => {
    if (!semester) return
    const fromLs = localStorage.getItem(lsKey)
    if (fromLs) {
      setStartDate(fromLs)
    } else if (semCfg && typeof semCfg === 'object' && 'start_date' in semCfg && (semCfg as { start_date?: string }).start_date) {
      setStartDate((semCfg as { start_date: string }).start_date)
    } else {
      setStartDate(defaultSemesterStart(semester))
    }
  }, [semester, semCfg, lsKey])

  const currentWeek = useMemo(
    () => (startDate ? currentWeekOf(startDate, semesterWeeks) : 1),
    [startDate, semesterWeeks]
  )
  const [selectedWeek, setSelectedWeek] = useState<number | null>(null)
  const week = selectedWeek ?? currentWeek

  // 建任务防重复：本地刚添加的记录 + 服务器已存在任务回显
  const [addedIds, setAddedIds] = useState<Set<string>>(new Set())
  const [detailCell, setDetailCell] = useState<{ weekDay: number; slotIdx: number } | null>(null)

  // 该教师已有的待评任务（status=1），用于刷新后仍能回显"已添加"
  const addedTeacherId = targetTeacherId ?? userId
  const { data: addedTasksData } = useQuery({
    queryKey: ['mobile-tasks', 'schedule-added', addedTeacherId],
    queryFn: () =>
      taskApi.list({ page: 1, page_size: 100, status: 1, teacher_id: addedTeacherId }),
    enabled: !!addedTeacherId,
  })

  // 单元格"已添加"集合：本地 + 服务器任务（按 课程名+日期 匹配，不同周日期不同）
  const addedSet = useMemo(() => {
    const s = new Set<string>(addedIds)
    for (const t of addedTasksData?.list || []) {
      if (!t.class_time) continue
      s.add(`${t.course_name}|${dayjs(t.class_time).format('YYYY-MM-DD')}`)
    }
    return s
  }, [addedIds, addedTasksData])

  // 课程任务键：包含具体上课日期，周次不同键即不同
  const courseTaskKey = (course: CourseItem, weekNum: number): string => {
    if (!startDate) return `${course.course_name}|w${weekNum}-d${course.week_day}`
    const date = getDateForWeekAndDay(startDate, weekNum, Number(course.week_day))
    return `${course.course_name}|${date.format('YYYY-MM-DD')}`
  }

  const details: CourseItem[] = scheduleData?.details || []

  // 单元格课程：week_day 匹配 + 节次交集 + 周次模式
  const cellCourses = (weekDay: number, slotIdx: number): CourseItem[] => {
    const firstSection = slotIdx * 2 + 1
    const sections = [firstSection, firstSection + 1]
    return details.filter((d) => {
      if (Number(d.week_day) !== weekDay) return false
      const nums: number[] = []
      const re = /(\d{2})/g
      let m: RegExpExecArray | null
      while ((m = re.exec(String(d.section ?? '')))) nums.push(Number(m[1]))
      if (!nums.some((n) => sections.includes(n))) return false
      return isCourseInWeek(d.week_pattern as string, week, semesterWeeks)
    })
  }

  const todayWeekDay = new Date().getDay() === 0 ? 7 : new Date().getDay()

  const addToTasks = async (course: CourseItem) => {
    const key = courseTaskKey(course, week)
    if (addedSet.has(key)) return
    let teacherId: number | undefined
    if (selectedTeacher) teacherId = selectedTeacher.id
    else if (!canViewUsers && userId != null) teacherId = userId
    else if (scheduleData?.teacher_id) teacherId = scheduleData.teacher_id
    if (!teacherId) {
      Toast.show({ content: '无法获取教师信息', icon: 'fail' })
      return
    }
    const slotIdx = detailCell?.slotIdx ?? 0
    const date = startDate
      ? getDateForWeekAndDay(startDate, week, Number(course.week_day))
      : null
    const startTime = TIME_SLOTS[slotIdx]?.time.split('-')[0] || '08:30'
    const classTime = date ? `${date.format('YYYY-MM-DD')} ${startTime}:00` : undefined
    try {
      await taskApi.create({
        teacher_id: teacherId,
        course_name: course.course_name,
        classroom: course.classroom || undefined,
        class_time: classTime,
      })
      setAddedIds((prev) => new Set(prev).add(key))
      qc.invalidateQueries({ queryKey: ['mobile-tasks'] })
      Toast.show({ content: '已添加至待评任务', icon: 'success' })
    } catch (e) {
      Toast.show({ content: e instanceof Error ? e.message : '添加失败', icon: 'fail' })
    }
  }

  return (
    <div>
      <NavBar
        backArrow={false}
        right={
          <Button
            size="small"
            fill="none"
            onClick={() => {
              if (canViewUsers) setPickerOpen(true)
            }}
          >
            {selectedTeacher ? `教师：${selectedTeacher.name}` : canViewUsers ? '选择教师' : ''}
          </Button>
        }
      >
        我的课表
      </NavBar>

      {/* 学期筛选 */}
      <div style={{ display: 'flex', gap: 8, padding: '8px 12px', flexWrap: 'wrap', alignItems: 'center' }}>
        <select
          value={semester || ''}
          onChange={(e) => {
            setSemester(e.target.value)
            setSelectedWeek(null)
            setStartDate(null)
          }}
          style={selectStyle}
        >
          {semesterOptions.map((s) => (
            <option key={s} value={s}>
              {formatSemester(s)}
            </option>
          ))}
        </select>
        <Button
          size="small"
          color="primary"
          fill="outline"
          onClick={() => {
            if (!semester) return
            // 刷新课表
            qc.invalidateQueries({ queryKey: ['schedule', 'mobile'] })
            Toast.show({ content: '已刷新' })
          }}
        >
          查询课表
        </Button>
      </div>

      {/* 信息行：教师名 + 起始日 + 设置 */}
      {scheduleData && scheduleData.id > 0 && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '4px 12px',
            fontSize: 13,
          }}
        >
          <strong>{scheduleData.teacher_name || selectedTeacher?.name || '我的课表'}</strong>
          {startDate && (
            <span style={{ color: '#666' }}>
              起始日：{dayjs(startDate).format('YYYY年M月D日')}
            </span>
          )}
          <SetOutline
            style={{ color: '#104186' }}
            onClick={() => setDateOpen(true)}
          />
        </div>
      )}

      {/* 周次横向 Tab（按学期配置周数） */}
      <div style={{ display: 'flex', gap: 6, overflowX: 'auto', padding: '8px 12px' }}>
        {Array.from({ length: semesterWeeks }, (_, i) => i + 1).map((w) => {
          const isActive = w === week
          const isCurrent = w === currentWeek
          return (
            <Button
              key={w}
              size="small"
              color="primary"
              fill={isActive ? 'solid' : isCurrent ? 'outline' : 'none'}
              style={{
                flexShrink: 0,
                minWidth: 40,
                ...(isActive ? {} : isCurrent ? { color: '#104186' } : { color: '#999' }),
              }}
              onClick={() => setSelectedWeek(w)}
            >
              第{w}周
            </Button>
          )
        })}
      </div>

      {/* 网格课表 */}
      {isLoading ? (
        <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>加载中...</div>
      ) : !scheduleData || scheduleData.id === 0 || details.length === 0 ? (
        <ErrorBlock status="empty" title="暂无课表数据" description="" />
      ) : (
        <div style={{ padding: '0 8px' }}>
          <table className="mobile-grid">
            <thead>
              <tr>
                <th>节次</th>
                {WEEK_DAYS.map((d, i) => (
                  <th key={d} className={todayWeekDay === i + 1 ? 'today' : ''}>
                    {d}
                    {startDate && (
                      <div style={{ fontSize: 10, fontWeight: 400 }}>
                        {getDateForWeekAndDay(startDate, week, i + 1).format('M/D')}
                      </div>
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {TIME_SLOTS.map((slot, slotIdx) => (
                <tr key={slot.label}>
                  <td className="slot-label">
                    <div>{slot.label}</div>
                    <div style={{ fontSize: 9, color: '#999' }}>{slot.time}</div>
                  </td>
                  {WEEK_DAYS.map((_, dayIdx) => {
                    const weekDay = dayIdx + 1
                    const courses = cellCourses(weekDay, slotIdx)
                    return (
                      <td
                        key={dayIdx}
                        className={todayWeekDay === weekDay ? 'today' : ''}
                        onClick={() => {
                          if (courses.length === 0) {
                            Toast.show({ content: '该时段暂无课程' })
                            return
                          }
                          setDetailCell({ weekDay, slotIdx })
                        }}
                      >
                        {courses.length > 0 ? (
                          <div className="course-cell">
                            <div className="course-name">{courses[0].course_name}</div>
                            {courses[0].classroom && (
                              <div className="course-room">{courses[0].classroom}</div>
                            )}
                            {courses.length > 1 && (
                              <div className="course-more">+{courses.length - 1}</div>
                            )}
                          </div>
                        ) : (
                          <span style={{ color: '#ddd' }}>-</span>
                        )}
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 课程详情弹窗 */}
      <Popup
        visible={!!detailCell}
        onMaskClick={() => setDetailCell(null)}
        bodyStyle={{
          borderTopLeftRadius: 8,
          borderTopRightRadius: 8,
          maxHeight: '60vh',
          overflowY: 'auto',
        }}
      >
        {detailCell && (
          <div style={{ padding: 16 }}>
            <div style={{ fontWeight: 600, marginBottom: 12 }}>
              第{week}周 周{WEEK_DAYS[detailCell.weekDay - 1]}{' '}
              {formatSectionText(
                String(detailCell.slotIdx * 2 + 1).padStart(2, '0') +
                  String(detailCell.slotIdx * 2 + 2).padStart(2, '0')
              )}
            </div>
            {cellCourses(detailCell.weekDay, detailCell.slotIdx).map((c) => {
              const key = courseTaskKey(c, week)
              const added = addedSet.has(key)
              return (
                <div key={key} className="course-detail-card">
                  <div style={{ fontWeight: 600, marginBottom: 6 }}>{c.course_name}</div>
                  {c.class_info && <div>班级：{c.class_info}</div>}
                  {c.student_count != null && <div>应到人数：{c.student_count}人</div>}
                  {c.classroom && <div>教室：{c.classroom}</div>}
                  {c.week_pattern && <div>周次：{c.week_pattern}</div>}
                  <div>节次：{formatSectionText(c.section as string)}</div>
                  <Button
                    block
                    size="small"
                    color="primary"
                    style={{ marginTop: 8 }}
                    disabled={added}
                    onClick={() => addToTasks(c)}
                  >
                    {added ? '已添加' : '加入待评任务'}
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </Popup>

      {/* 起始日设置弹窗 */}
      <Dialog
        visible={dateOpen}
        title="设置学期起始日"
        content={
          <input
            type="date"
            value={startDate || ''}
            onChange={(e) => setStartDate(e.target.value)}
            style={{ width: '100%', padding: 8, border: '1px solid #ddd', borderRadius: 6 }}
          />
        }
        actions={[
          [
            { key: 'cancel', text: '取消', onClick: () => setDateOpen(false) },
            {
              key: 'ok',
              text: '确定',
              bold: true,
              onClick: () => {
                if (semester && startDate) {
                  localStorage.setItem(lsKey, startDate)
                  setSelectedWeek(null)
                }
                setDateOpen(false)
              },
            },
          ],
        ]}
        onClose={() => setDateOpen(false)}
      />

      {/* 教师选择弹层 */}
      <TeacherPicker
        visible={pickerOpen}
        onClose={() => setPickerOpen(false)}
        onPick={(t) => {
          setSelectedTeacher({ id: t.id, name: t.username })
          setPickerOpen(false)
          setSelectedWeek(null)
          setAddedIds(new Set())
        }}
      />
    </div>
  )
}

/* ═══ 教师选择弹层：学院/教研室筛选 + 防抖搜索 + 滚动自动分页加载 ═══ */
function TeacherPicker({
  visible,
  onClose,
  onPick,
}: {
  visible: boolean
  onClose: () => void
  onPick: (t: User) => void
}) {
  const [collegeId, setCollegeId] = useState<number | undefined>()
  const [roomId, setRoomId] = useState<number | undefined>()
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)

  // 每次打开重置搜索词，保持列表原始顺序方便继续选择
  useEffect(() => {
    if (visible) setKeyword('')
  }, [visible])

  const { data: colleges } = useQuery({
    queryKey: ['colleges', undefined],
    queryFn: () => orgApi.collegeList(),
    enabled: visible,
  })
  const { data: rooms } = useQuery({
    queryKey: ['research-rooms', collegeId],
    queryFn: () => orgApi.roomList({ college_id: collegeId }),
    enabled: visible,
  })

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetching,
    isLoading,
  } = useInfiniteQuery({
    queryKey: ['picker-teachers', collegeId, roomId, debouncedKeyword || undefined],
    queryFn: ({ pageParam }) =>
      userApi.list({
        page: pageParam,
        page_size: 50,
        role: 'teacher',
        keyword: debouncedKeyword || undefined,
        college_id: collegeId,
        research_room_id: roomId,
      }),
    initialPageParam: 1,
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + (p.list?.length || 0), 0)
      return loaded < (last.total ?? 0) ? all.length + 1 : undefined
    },
    enabled: visible,
  })

  const list = (data?.pages || []).flatMap((p) => p.list || [])

  return (
    <Popup visible={visible} onMaskClick={onClose} bodyStyle={{ height: '70vh', overflowY: 'auto', borderTopLeftRadius: 8, borderTopRightRadius: 8 }}>
      <div style={{ padding: 12 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <strong>选择教师</strong>
          <Button size="small" fill="none" onClick={onClose}>
            取消
          </Button>
        </div>
        <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
          <select
            value={collegeId ?? ''}
            onChange={(e) => {
              setCollegeId(e.target.value ? Number(e.target.value) : undefined)
              setRoomId(undefined)
            }}
            style={selectStyle}
          >
            <option value="">全部学院</option>
            {(colleges?.list || []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
          <select
            value={roomId ?? ''}
            onChange={(e) => setRoomId(e.target.value ? Number(e.target.value) : undefined)}
            style={selectStyle}
          >
            <option value="">全部教研室</option>
            {(rooms?.list || []).map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </div>
        <SearchBar
          placeholder="输入教师姓名或工号搜索"
          value={keyword}
          onChange={setKeyword}
        />
        <List style={{ marginTop: 8 }}>
          {list.map((t) => (
            <List.Item
              key={t.id}
              arrow
              onClick={() => onPick(t)}
              description={
                <span style={{ fontSize: 12, color: '#999' }}>
                  {[t.user_no, t.college_name, t.research_room_name]
                    .filter(Boolean)
                    .join(' · ') || '未关联学院'}
                </span>
              }
            >
              {t.username}
            </List.Item>
          ))}
        </List>
        {list.length === 0 && !isFetching && !isLoading && (
          <ErrorBlock status="empty" title="未找到匹配的教师" description="" />
        )}
        {(hasNextPage || isFetching) && (
          <InfiniteScroll
            hasMore={!!hasNextPage}
            loadMore={async () => {
              await fetchNextPage()
            }}
          />
        )}
      </div>
    </Popup>
  )
}

const selectStyle: React.CSSProperties = {
  flex: 1,
  padding: '6px 8px',
  borderRadius: 6,
  border: '1px solid #ddd',
  background: '#fff',
  fontSize: 13,
}
