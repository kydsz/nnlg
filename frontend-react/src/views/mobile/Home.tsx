import { useCallback, useState } from 'react'
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
import { formatDate, formatClassPeriod } from '@/utils/format'

type StatusFilter = '' | '1' | '2' | 'supervisor_yes' | 'supervisor_no'
type CreatorFilter = '' | 'my_created' | 'other_created'

export default function Home() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const userId = useAuthStore((s) => s.user?.id)
  const hasPermission = useAuthStore((s) => s.hasPermission)

  const [keyword, setKeyword] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('')
  const [creatorFilter, setCreatorFilter] = useState<CreatorFilter>('')
  const [page, setPage] = useState(1)
  const canDelete = (t: Task) =>
    hasPermission('task:delete') ||
    (hasPermission('task:delete_own') && userId != null && t.create_by === userId)

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
      return params
    },
    [keyword, statusFilter, creatorFilter, userId]
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
          onChange={setKeyword}
          onSearch={resetAndReload}
          onClear={resetAndReload}
        />
        <div style={{ display: 'flex', gap: 8, padding: '4px 12px', overflowX: 'auto' }}>
          <select
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value as StatusFilter)
              resetAndReload()
            }}
            style={selectStyle}
          >
            <option value="">全部状态</option>
            <option value="1">待评</option>
            <option value="2">已评</option>
            <option value="supervisor_yes">督导已评</option>
            <option value="supervisor_no">督导未评</option>
          </select>
          <select
            value={creatorFilter}
            onChange={(e) => {
              setCreatorFilter(e.target.value as CreatorFilter)
              resetAndReload()
            }}
            style={selectStyle}
          >
            <option value="">全部任务</option>
            <option value="my_created">我创建的</option>
            <option value="other_created">其他创建的</option>
          </select>
        </div>
      </div>

      <PullToRefresh onRefresh={refresh}>
        {tasks.length === 0 && !isLoading && !isError ? (
          <ErrorBlock status="empty" title="暂无待评任务" description="" />
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
                  <Tag color={t.has_supervisor_eval ? 'success' : 'default'} fill="outline">
                    {t.has_supervisor_eval ? '督导已评' : '督导未评'}
                  </Tag>
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
