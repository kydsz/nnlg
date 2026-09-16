import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  NavBar,
  SearchBar,
  PullToRefresh,
  Card,
  Tag,
  Button,
  Dialog,
  Toast,
  ErrorBlock,
} from 'antd-mobile'
import { DeleteOutline } from 'antd-mobile-icons'
import { taskApi, type TaskListParams } from '@/api/modules/tasks'
import { useAuthStore } from '@/stores/auth'
import type { Task } from '@/api/types'
import {
  defaultFiltersFor,
  showMyCreatedHint,
  myCreatedEmptyHint,
  type HomeFilters,
  type StatusFilter,
  type CreatorFilter,
} from './homeFilters'
import { useSemesters } from './useSemesters'
import { semesterRangeOf } from '@/utils/semester'
import { formatDate, formatClassPeriod, formatSemester } from '@/utils/format'

export default function Home() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const userId = useAuthStore((s) => s.user?.id)
  const hasPermission = useAuthStore((s) => s.hasPermission)

  const [filters, setFilters] = useState<HomeFilters>(defaultFiltersFor)
  const { keyword, statusFilter, creatorFilter, semester } = filters
  const [page, setPage] = useState(1)

  // 学期：默认取管理员配置的当前学期；下拉可切换到其它学期
  const { configList, semesterOptions, currentSemester } = useSemesters()
  useEffect(() => {
    if (!semester && (currentSemester?.semester || semesterOptions[0])) {
      setFilters((f) =>
        f.semester
          ? f
          : { ...f, semester: currentSemester?.semester || semesterOptions[0] }
      )
    }
  }, [currentSemester, semesterOptions, semester])

  // 当前所选学期的日期区间（start_date ~ end_date），用于过滤任务的上课时间
  const semesterRange = useMemo(
    () => semesterRangeOf(configList, semester),
    [configList, semester]
  )
  const canDelete = (t: Task) =>
    hasPermission('task:delete') ||
    (hasPermission('task:delete_own') && userId != null && t.create_by === userId)
  // 「查看他人评教任务」由管理员按角色分配；未分配时只能看与我相关的任务，
  // 后端已强制收敛，这里同步去掉会诱导去翻别人任务的视图切换。
  const canViewOthersTasks = hasPermission('task:view_all')

  const buildParams = useCallback(
    (p: number): TaskListParams => {
      const params: TaskListParams = { page: p, page_size: 20 }
      if (keyword.trim()) params.keyword = keyword.trim()
      if (statusFilter === '1' || statusFilter === '2') {
        params.status = Number(statusFilter)
      } else if (statusFilter === 'supervisor_yes') {
        params.has_supervisor_eval = true
      } else if (statusFilter === 'supervisor_no') {
        params.has_supervisor_eval = false
      }
      if (creatorFilter === 'my_created' && userId != null) {
        params.create_by = userId
      } else if (creatorFilter === 'other_created' && userId != null) {
        params.create_by_not = userId
      }
      if (semesterRange.length === 2) {
        params.start_date = semesterRange[0]
        params.end_date = semesterRange[1]
      }
      return params
    },
    [keyword, statusFilter, creatorFilter, userId, semesterRange]
  )

  const { data, isLoading, isError } = useQuery({
    queryKey: ['mobile-tasks', buildParams(page)],
    queryFn: () => taskApi.list(buildParams(page)),
    placeholderData: (prev) => prev,
  })

  const tasks: Task[] = data?.list || []
  const total = data?.total ?? 0
  const finished = tasks.length >= total || tasks.length < page * 20

  const resetAndReload = () => {
    setPage(1)
    qc.invalidateQueries({ queryKey: ['mobile-tasks'] })
  }

  const refresh = async () => {
    setPage(1)
    await qc.invalidateQueries({ queryKey: ['mobile-tasks'] })
  }

  const delTask = (t: Task) => {
    Dialog.confirm({
      title: '确认删除',
      content: `确定要删除任务"${t.course_name}"吗？`,
      confirmText: '删除',
      cancelText: '取消',
      onConfirm: async () => {
        try {
          await taskApi.remove(t.id)
          Toast.show({ content: '删除成功', icon: 'success' })
          qc.setQueryData<{ list: Task[]; total: number }>(
            ['mobile-tasks', buildParams(page)],
            (old) =>
              old
                ? { list: old.list.filter((x) => x.id !== t.id), total: old.total - 1 }
                : old
          )
        } catch (e) {
          Toast.show({ content: e instanceof Error ? e.message : '删除失败，请重试', icon: 'fail' })
        }
      },
    })
  }

  return (
    <div>
      <NavBar backArrow={false}>待评任务</NavBar>

      {/* 搜索 + 筛选（sticky） */}
      <div style={{ position: 'sticky', top: 0, zIndex: 10, background: '#f5f5f5', paddingBottom: 4 }}>
        <SearchBar
          placeholder="搜索课程/教师"
          value={keyword}
          onChange={(k) => setFilters((f) => ({ ...f, keyword: k }))}
          onSearch={resetAndReload}
          onClear={resetAndReload}
        />
        <div style={{ display: 'flex', gap: 8, padding: '4px 12px', overflowX: 'auto' }}>
          <select
            value={semester || ''}
            onChange={(e) => {
              // 学期为主轴：切学期即回到该学期的默认视图
              setFilters(defaultFiltersFor(e.target.value || undefined))
              resetAndReload()
            }}
            style={selectStyle}
          >
            {semesterOptions.map((s) => (
              <option key={s} value={s}>
                {formatSemester(s)}
              </option>
            ))}
          </select>
          <select
            value={statusFilter}
            onChange={(e) => {
              setFilters((f) => ({ ...f, statusFilter: e.target.value as StatusFilter }))
              resetAndReload()
            }}
            style={selectStyle}
          >
            <option value="">全部状态</option>
            <option value="1">待评</option>
            <option value="2">已评</option>
            <option value="supervisor_yes">本次督导已评</option>
            <option value="supervisor_no">本次督导未评</option>
          </select>
          {canViewOthersTasks && (
            <select
              value={creatorFilter}
              onChange={(e) => {
                setFilters((f) => ({ ...f, creatorFilter: e.target.value as CreatorFilter }))
                resetAndReload()
              }}
              style={selectStyle}
            >
              <option value="">全部任务</option>
              <option value="my_created">我创建的</option>
              <option value="other_created">其他创建的</option>
            </select>
          )}
        </div>
      </div>

      <PullToRefresh onRefresh={refresh}>
        {tasks.length === 0 && !isLoading && !isError ? (
          <ErrorBlock
            status="empty"
            title="暂无待评任务"
            description={showMyCreatedHint(creatorFilter) ? myCreatedEmptyHint(canViewOthersTasks) : ''}
          />
        ) : isError ? (
          <ErrorBlock status="default" title="获取任务列表失败" description="" />
        ) : (
          <div style={{ padding: 8 }}>
            {tasks.map((t) => {
              const period = formatClassPeriod(t.class_time)
              return (
              <Card
                key={t.id}
                style={{ marginBottom: 8 }}
                onClick={() => navigate(`/mobile/evaluation/${t.id}`)}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div style={{ fontWeight: 600, fontSize: 15 }}>{t.course_name}</div>
                  {canDelete(t) && (
                    <Button
                      size="mini"
                      color="danger"
                      fill="none"
                      onClick={(e) => {
                        e.stopPropagation()
                        delTask(t)
                      }}
                    >
                      <DeleteOutline /> 删除
                    </Button>
                  )}
                </div>
                <div style={{ color: '#999', fontSize: 12, marginTop: 2 }}>
                  {t.teacher_college_name || ''}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                  <Tag color="primary" fill="outline">{t.teacher_name}</Tag>
                  {t.classroom && <Tag color="success" fill="outline">{t.classroom}</Tag>}
                  {(t.evaluation_count ?? 0) > 0 && (
                    <Tag color="warning" fill="outline">已评 {t.evaluation_count} 人</Tag>
                  )}
                  {(t.course_evaluator_count ?? 0) > 0 && (
                    <Tag color="warning" fill="outline">本课程已评 {t.course_evaluator_count} 人</Tag>
                  )}
                  <Tag color={t.has_supervisor_eval ? 'success' : 'default'} fill="outline">
                    {t.has_supervisor_eval ? '本次督导已评' : '本次督导未评'}
                  </Tag>
                  {t.has_draft && <Tag color="danger" fill="outline">有草稿</Tag>}
                </div>
                <div style={{ color: '#999', fontSize: 12, marginTop: 8 }}>
                  {period && (
                    <span style={{ color: '#1677ff', marginRight: 6 }}>{period}</span>
                  )}
                  上课时间：{formatDate(t.class_time)}
                </div>
              </Card>
              )
            })}
          </div>
        )}
        {!finished && tasks.length > 0 && (
          <div style={{ textAlign: 'center', padding: 12 }}>
            <Button fill="none" loading={isLoading} onClick={() => setPage((p) => p + 1)}>
              加载更多
            </Button>
          </div>
        )}
        {finished && tasks.length > 0 && (
          <div style={{ textAlign: 'center', color: '#bbb', fontSize: 12, padding: 12 }}>
            没有更多了
          </div>
        )}
      </PullToRefresh>
    </div>
  )
}

const selectStyle: React.CSSProperties = {
  flexShrink: 0,
  padding: '6px 8px',
  borderRadius: 6,
  border: '1px solid #ddd',
  background: '#fff',
  fontSize: 13,
}
