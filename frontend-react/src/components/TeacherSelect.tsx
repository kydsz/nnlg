import { useEffect, useRef, useState } from 'react'
import { Select, Space, type GetProp } from 'antd'
import { LoadingOutlined } from '@ant-design/icons'
import { userApi } from '@/api/modules/users'
import type { User } from '@/api/types'
import { useDebounce } from '@/hooks/useDebounce'

/**
 * 教师选择器：远程搜索（姓名/工号）+ 下拉滚动分页动态加载，
 * 编辑时自动回显已选教师（不在当前已加载列表时单独拉取）。
 * 任务创建/编辑与评教记录编辑（改课堂课程信息）共用。
 */
export default function TeacherSelect({
  value,
  onChange,
  style,
}: {
  value?: number
  onChange?: (v: number | undefined) => void
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
      const res = await userApi.list({
        page: p,
        page_size: PAGE_SIZE,
        role: 'teacher',
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

  // 编辑回显：选中教师不在当前已加载列表时，单独拉取该教师信息
  useEffect(() => {
    if (value == null) {
      setSelected(null)
      return
    }
    if (selected?.id === value || options.some((o) => o.id === value)) return
    userApi
      .get(value)
      .then((u) => setSelected(u || null))
      .catch(() => {})
  }, [value])

  const handlePopupScroll: GetProp<typeof Select, 'onPopupScroll'> = (e) => {
    const el = e.currentTarget
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 40 && !loading && options.length < total) {
      fetchPage(page + 1, debouncedKeyword, true)
    }
  }

  // 保持列表原始顺序；仅当已选中教师不在当前已加载列表时（如翻页后）追加到末尾用于回显
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
        // 选中/清空后重置搜索词，下次打开恢复完整列表的原始顺序
        setKeyword('')
        if (v == null) {
          setSelected(null)
          onChange?.(undefined)
          return
        }
        const u = mergedOptions.find((o) => o.id === v)
        setSelected(u || null)
        onChange?.(v as number)
      }}
      notFoundContent={loading ? <LoadingOutlined spin /> : '暂无教师'}
      options={mergedOptions.map((t) => ({
        value: t.id,
        label: `${t.username}（${t.user_no}）`,
        desc: t.college_name || undefined,
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
