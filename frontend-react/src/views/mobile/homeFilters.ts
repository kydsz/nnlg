export type StatusFilter = '' | '1' | '2' | 'supervisor_yes' | 'supervisor_no'
export type CreatorFilter = '' | 'my_created' | 'other_created'

export interface HomeFilters {
  semester: string | undefined
  statusFilter: StatusFilter
  creatorFilter: CreatorFilter
  keyword: string
}

/** 「我创建的」空列表时的引导文案 */
export const MY_CREATED_EMPTY_HINT = '当前仅显示我创建的任务，可切换到『全部任务』查看本学期全部评教任务'

/**
 * 首页筛选的默认视图：我创建的任务 + 全部状态 + 无关键词。
 * 学期为主轴——每个学期的默认视图都从这里来；不传学期即页面初始态
 * （semester 为 undefined，由当前学期异步定位补上）。
 */
export function defaultFiltersFor(semester?: string): HomeFilters {
  return {
    semester,
    statusFilter: '',
    creatorFilter: 'my_created',
    keyword: '',
  }
}

/** 「我创建的」空列表需要引导提示；「全部任务」「其他创建的」维持普通空态 */
export function showMyCreatedHint(f: HomeFilters): boolean {
  return f.creatorFilter === 'my_created'
}
