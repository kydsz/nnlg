import { useEffect, useRef, useState } from 'react'
import { Select, Space, type GetProp } from 'antd'
import { LoadingOutlined } from '@ant-design/icons'
import { taskApi } from '@/api/modules/tasks'
import { userApi } from '@/api/modules/users'
import type { User } from '@/api/types'
import { useDebounce } from '@/hooks/useDebounce'

/**
 * 教师选择器：远程搜索（姓名/工号）+ 下拉滚动分页动态加载。
 * 数据来源 /teacher-options：可见范围含「人事学院或授课学院」，外聘教师可被授课学院选择。
 * 编辑时可通过 initialOption 回显未出现在当前列表中的教师。
 */
export default function TeacherSelect({
  value,
  onChange,
  onSelect,
  initialOption,
  style,
}: {
  value?: number
  onChange?: (v: number | undefined) => void
  onSelect?: (u?: User) => void
  initialOption?: Partial<User> & { id: number; username: string }
  style?: React.CSSProperties
}) {
  const PAGE_SIZE = 50
  const [keyword, setKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const [options, setOptions] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<User | null>(null)
  const reqId = useRef(0)

  const fetchPage = async (p: number, kw: string, append: boolean) => {
    const id = ++reqId.current
    setLoading(true)
    try {
      const res = await taskApi.teacherOptions({
        page: p,
        page_size: PAGE_SIZE,
        keyword: kw || undefined,
      })
      if (id !== reqId.current) return // 已有更新的请求，丢弃过期结果
      setOptions((prev) => (append ? [...prev, ...(res.list || [])] : res.list || []))
      setTotal(res.total ?? 0)
      setPage(p)
    } finally {
      if (id === reqId.current) setLoading(false)
    }
  }

  useEffect(() => {
    fetchPage(1, debouncedKeyword, false)
  }, [debouncedKeyword])

  // 编辑回显：优先使用调用方给定的教师快照；否则按 ID 单独拉取
  useEffect(() => {
    if (value == null) {
      setSelected(null)
      return
    }
    if (selected?.id === value || options.some((o) => o.id === value)) return
    if (initialOption && initialOption.id === value) {
      setSelected(initialOption as User)
      return
    }
    userApi
      .get(value)
      .then((u) => setSelected(u || null))
      .catch(() => {})
  }, [value, initialOption])

  const handlePopupScroll: GetProp<typeof Select, 'onPopupScroll'> = (e) => {
    const el = e.currentTarget
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 40 && !loading && options.length < total) {
      fetchPage(page + 1, debouncedKeyword, true)
    }
  }

  // 保持列表原始顺序；仅当已选中教师不在当前已加载列表时追加到末尾用于回显
  const mergedOptions =
    selected && !options.some((o) => o.id === selected.id) ? [...options, selected] : options

  return (
    <Select
      showSearch
      allowClear
      filterOption={false}
      placeholder="选择教师（支持姓名/工号搜索）"
      value={value}
      style={style}
      loading={loading}
      onSearch={setKeyword}
      onPopupScroll={handlePopupScroll}
      onChange={(v) => {
        setKeyword('')
        if (v == null) {
          setSelected(null)
          onSelect?.(undefined)
          onChange?.(undefined)
          return
        }
        const u = mergedOptions.find((o) => o.id === v)
        setSelected(u || null)
        if (u) onSelect?.(u) // 选项尚未加载完成时不回传 undefined，避免误清空已选授课学院
        onChange?.(v as number)
      }}
      notFoundContent={loading ? <LoadingOutlined spin /> : '暂无教师'}
      options={mergedOptions.map((t) => ({
        value: t.id,
        label: `${t.username}（${t.user_no}）`,
        desc: [t.college_name, ...(t.teaching_college_names || [])].filter(Boolean).join(' / '),
      }))}
      optionRender={(option) => {
        const { label, desc } = option as unknown as { label?: React.ReactNode; desc?: string }
        return (
          <Space>
            <span>{label}</span>
            {desc && <span style={{ color: '#999', fontSize: 12 }}>{desc}</span>}
          </Space>
        )
      }}
    />
  )
}
