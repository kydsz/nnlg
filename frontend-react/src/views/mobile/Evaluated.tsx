import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import {
  List,
  Tag,
  Popup,
  NavBar,
  Button,
  Toast,
  Tabs,
  PullToRefresh,
  Card,
  Dialog,
  ImageViewer,
  SearchBar,
} from 'antd-mobile'
import { evaluationApi } from '@/api/modules/evaluations'
import { useAuthStore } from '@/stores/auth'
import type { EvaluationRecord } from '@/api/types'
import { formatDate, formatSemester } from '@/utils/format'
import { semesterRangeOf } from '@/utils/semester'
import { downloadBlob } from '@/utils/download'
import {
  groupEvaluationDimensions,
  isImageArray,
  isFileArray,
  formatValueText,
  getFileNameFromUrl,
  type EvalDetailRow,
} from '@/utils/evalDetail'
import {
  buildEvaluatedListParams,
  defaultEvaluatedFiltersFor,
  type EvaluatedFilters,
} from './evaluatedFilters'
import { useSemesters } from './useSemesters'

type EvalType = 'received' | 'sent'

export default function Evaluated() {
  const [type, setType] = useState<EvalType>('received')
  const userId = useAuthStore((s) => s.user?.id)
  // 筛选状态两 tab 共享；学期 undefined=初始态（定位到当前学期）、''=全部学期
  const [filters, setFilters] = useState<EvaluatedFilters>(defaultEvaluatedFiltersFor)
  const { configList, semesterOptions, currentSemester } = useSemesters()

  // 学期为主轴：初始态定位到当前学期（「全部学期」空串不参与定位）
  useEffect(() => {
    if (filters.semester === undefined && (currentSemester?.semester || semesterOptions[0])) {
      setFilters((f) =>
        f.semester === undefined
          ? { ...f, semester: currentSemester?.semester || semesterOptions[0] }
          : f
      )
    }
  }, [currentSemester, semesterOptions, filters.semester])

  return (
    <div>
      <NavBar backArrow={false}>{type === 'received' ? '评给我的' : '已评记录'}</NavBar>
      <Tabs activeKey={type} onChange={(k) => setType(k as EvalType)}>
        <Tabs.Tab title="评给我的" key="received" />
        <Tabs.Tab title="我评的" key="sent" />
      </Tabs>
      {/* 搜索 + 筛选（sticky），对齐首页模式 */}
      <div style={{ position: 'sticky', top: 0, zIndex: 10, background: '#f5f5f5', paddingBottom: 4 }}>
        <SearchBar
          placeholder="搜索课程/教师/评教人"
          value={filters.keyword}
          onChange={(k) => setFilters((f) => ({ ...f, keyword: k }))}
        />
        <div style={{ display: 'flex', gap: 8, padding: '4px 12px', overflowX: 'auto' }}>
          <select
            value={filters.semester ?? ''}
            onChange={(e) => setFilters((f) => ({ ...f, semester: e.target.value }))}
            style={selectStyle}
          >
            <option value="">全部学期</option>
            {semesterOptions.map((s) => (
              <option key={s} value={s}>
                {formatSemester(s)}
              </option>
            ))}
          </select>
        </div>
      </div>
      {userId != null && (
        <RecordList key={type} type={type} userId={userId} filters={filters} configList={configList} />
      )}
    </div>
  )
}

function RecordList({
  type,
  userId,
  filters,
  configList,
}: {
  type: EvalType
  userId: number
  filters: EvaluatedFilters
  configList: ReturnType<typeof useSemesters>['configList']
}) {
  const qc = useQueryClient()
  const [detail, setDetail] = useState<EvaluationRecord | null>(null)
  const hasPermission = useAuthStore((s) => s.hasPermission)

  const range = useMemo(() => semesterRangeOf(configList, filters.semester), [configList, filters.semester])
  const baseParams = useMemo(
    () => buildEvaluatedListParams({ type, userId, filters, range, page: 1 }),
    [type, userId, filters, range]
  )

  // key 保留 'evaluations' 公共前缀：管理端删除、移动端提交后的 ['evaluations'] 失效才能命中本页
  const { data, isLoading, isFetchingNextPage, fetchNextPage, hasNextPage } = useInfiniteQuery({
    queryKey: ['evaluations', 'mobile', type, baseParams],
    queryFn: ({ pageParam }) => evaluationApi.list({ ...baseParams, page: pageParam }),
    initialPageParam: 1,
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + (p.list?.length ?? 0), 0)
      return loaded < last.total ? all.length + 1 : undefined
    },
    placeholderData: (prev) => prev,
  })

  const records = data?.pages.flatMap((p) => p.list ?? []) ?? []
  const finished = !hasNextPage

  const delMut = async (id: number) => {
    try {
      await evaluationApi.remove(id)
      Toast.show({ content: '删除成功', icon: 'success' })
      setDetail(null)
      await qc.invalidateQueries({ queryKey: ['evaluations', 'mobile', type] })
    } catch (e) {
      Toast.show({ content: e instanceof Error ? e.message : '删除失败', icon: 'fail' })
    }
  }

  const refresh = async () => {
    await qc.invalidateQueries({ queryKey: ['evaluations', 'mobile', type] })
  }

  const openDetail = async (id: number) => {
    try {
      const d = await evaluationApi.detail(id)
      setDetail(d)
    } catch (e) {
      Toast.show({ content: e instanceof Error ? e.message : '加载失败', icon: 'fail' })
    }
  }

  const doExport = async (r: EvaluationRecord) => {
    try {
      const blob = await evaluationApi.exportRecord(r.id, 'pdf')
      downloadBlob(blob, `评教记录_${r.id}.pdf`)
      Toast.show({ content: '导出成功', icon: 'success' })
    } catch (e) {
      Toast.show({ content: e instanceof Error ? e.message : '导出失败', icon: 'fail' })
    }
  }

  return (
    <PullToRefresh onRefresh={refresh}>
      <List>
        {records.map((r) => (
          <List.Item
            key={r.id}
            arrow
            onClick={() => openDetail(r.id)}
            description={
              <span style={{ fontSize: 12, color: '#999' }}>
                {type === 'received' ? `评教人: ${r.evaluator_name}` : `教师: ${r.teacher_name}`} ·{' '}
                {formatDate(r.submit_time)}
              </span>
            }
          >
            <span style={{ marginRight: 8 }}>{r.course_name}</span>
            {type === 'received' ? (
              r.evaluator_role_name && (
                <Tag color="success" fill="outline">
                  {r.evaluator_role_name}
                </Tag>
              )
            ) : (
              <Tag color="success" fill="outline">
                已评
              </Tag>
            )}
          </List.Item>
        ))}
      </List>
      {!finished && records.length > 0 && (
        <div style={{ textAlign: 'center', padding: 12 }}>
          <Button fill="none" loading={isFetchingNextPage || isLoading} onClick={() => void fetchNextPage()}>
            加载更多
          </Button>
        </div>
      )}
      {finished && records.length > 0 && (
        <div style={{ textAlign: 'center', color: '#bbb', fontSize: 12, padding: 12 }}>没有更多了</div>
      )}
      {records.length === 0 && !isLoading && (
        <Card style={{ margin: 12, textAlign: 'center', color: '#999' }}>
          {type === 'received' ? '暂无评教记录' : '暂无已评记录'}
        </Card>
      )}

      <Popup
        visible={!!detail}
        onMaskClick={() => setDetail(null)}
        bodyStyle={{ borderTopLeftRadius: 8, borderTopRightRadius: 8, maxHeight: '85vh', overflowY: 'auto' }}
      >
        {detail && (
          <div style={{ padding: 16 }}>
            <NavBar
              onBack={() => setDetail(null)}
              right={
                <Button size="small" fill="none" onClick={() => doExport(detail)}>
                  导出/打印
                </Button>
              }
            >
              评教详情
            </NavBar>

            {/* 基本信息 */}
            <List style={{ marginTop: 12 }}>
              <List.Item extra={detail.course_name}>课程</List.Item>
              <List.Item extra={detail.teacher_name}>被评教师</List.Item>
              <List.Item extra={detail.evaluator_name}>评教人</List.Item>
              <List.Item extra={detail.evaluator_role_name || detail.evaluator_role || '-'}>
                评教角色
              </List.Item>
              <List.Item extra={formatDate(detail.submit_time)}>评教时间</List.Item>
              <List.Item extra={detail.is_anonymous ? '是' : '否'}>是否匿名</List.Item>
            </List>

            {/* 课表信息（与提交页一致） */}
            {detail.schedule && (
              <>
                <List
                  style={{ marginTop: 12 }}
                  header={<span style={{ fontSize: 14, fontWeight: 600 }}>课表信息</span>}
                >
                  <List.Item extra={detail.schedule.class_time_text || '-'}>
                    上课时间
                  </List.Item>
                  <List.Item extra={detail.schedule.classroom || '-'}>教室</List.Item>
                  <List.Item extra={detail.schedule.class_info || '-'}>班级</List.Item>
                  <List.Item
                    extra={
                      detail.schedule.student_count != null
                        ? `${detail.schedule.student_count}人`
                        : '-'
                    }
                  >
                    应到人数
                  </List.Item>
                  <List.Item extra={detail.schedule.week_pattern || '-'}>周次</List.Item>
                </List>
              </>
            )}

            <EvalDetailBody detail={detail} />

            {type === 'sent' &&
              (hasPermission('evaluation:delete') || hasPermission('evaluation:delete_own')) && (
              <Button
                block
                color="danger"
                fill="outline"
                style={{ marginTop: 16 }}
                onClick={() => {
                  Dialog.confirm({
                    title: '确认删除',
                    content:
                      '删除后该记录将从列表、统计与汇总中移除，被评教师的评分统计会随之变化。确定要删除这条评教记录吗？',
                    confirmText: '删除',
                    cancelText: '取消',
                    onConfirm: () => delMut(detail.id),
                  })
                }}
              >
                删除此记录
              </Button>
            )}
          </div>
        )}
      </Popup>
    </PullToRefresh>
  )
}

/** 分数分组统计 + 按分组渲染维度值 */
function EvalDetailBody({ detail }: { detail: EvaluationRecord }) {
  const groups = groupEvaluationDimensions(detail)
  const totalMax = groups.reduce((s, g) => s + g.max_score, 0)
  const total = detail.total_score ?? groups.reduce((s, g) => s + g.score, 0)

  return (
    <>
      {totalMax > 0 && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '12px 16px',
            background: '#fff7f7',
            borderRadius: 8,
            margin: '12px 0',
          }}
        >
          <span style={{ fontSize: 14 }}>总分</span>
          <span style={{ fontSize: 20, fontWeight: 600, color: '#ff3141' }}>
            {total}
            <span style={{ fontSize: 13, color: '#999' }}> / {totalMax}</span>
          </span>
        </div>
      )}

      {groups.map((g) => (
        <div key={g.key} style={{ marginBottom: 16 }}>
          <div
            style={{
              fontWeight: 600,
              fontSize: 14,
              padding: '8px 0',
              borderBottom: '2px solid #104186',
              marginBottom: 8,
            }}
          >
            {g.name}
            {g.max_score > 0 && (
              <span style={{ float: 'right', color: '#c00' }}>
                {g.score}/{g.max_score}
              </span>
            )}
          </div>
          {g.rows.map((row) => (
            <ValueRow key={row.code} row={row} />
          ))}
        </div>
      ))}
    </>
  )
}

function ValueRow({ row }: { row: EvalDetailRow }) {
  const [viewerIdx, setViewerIdx] = useState<number | null>(null)
  const images = isImageArray(row.value) ? (row.value as string[]) : null
  const files = !images && isFileArray(row.value) ? (row.value as string[]) : null

  return (
    <div style={{ marginBottom: 8 }}>
      <div style={{ fontSize: 13, color: '#999' }}>{row.name}</div>
      <div style={{ fontSize: 14 }}>
        {images ? (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {images.map((u, i) => (
              <img
                key={u}
                src={u}
                style={{ width: 80, height: 80, objectFit: 'cover', borderRadius: 4 }}
                onClick={() => setViewerIdx(i)}
              />
            ))}
            {viewerIdx != null && (
              <ImageViewer
                image={images[viewerIdx]}
                visible
                onClose={() => setViewerIdx(null)}
              />
            )}
          </div>
        ) : files ? (
          files.map((u) => (
            <div key={u}>
              <a href={u} target="_blank" rel="noreferrer" download={getFileNameFromUrl(u)} style={{ color: '#104186' }}>
                {getFileNameFromUrl(u)}
              </a>
            </div>
          ))
        ) : (
          <span>{row.display_value || formatValueText(row.value)}</span>
        )}
      </div>
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
