export type StatusFilter = '' | '1' | '2' | 'supervisor_yes' | 'supervisor_no'
export type CreatorFilter = '' | 'my_created' | 'other_created'

export interface HomeFilters {
  semester: string | undefined
  statusFilter: StatusFilter
  creatorFilter: CreatorFilter
  keyword: string
}

/** 「我创建的」空列表时的引导文案（列表里还有『全部任务』可切换） */
export const MY_CREATED_EMPTY_HINT = '当前仅显示我创建的任务，可切换到『全部任务』查看本学期全部评教任务'

/**
 * 「我创建的」空列表时的引导文案（无「查看他人评教任务」权限）。
 * 此时列表里没有其他视图可切，改为引导找管理员开权限。
 */
export const MY_CREATED_SCOPED_EMPTY_HINT =
  '当前仅显示我创建的任务。如需查看本学院其他评教任务，请联系管理员分配「查看他人评教任务」权限'

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
export function showMyCreatedHint(creatorFilter: CreatorFilter): boolean {
  return creatorFilter === 'my_created'
}

/**
 * 「我创建的」空列表的引导文案：有无「查看他人评教任务」（task:view_all）权限，
 * 决定是引导去切「全部任务」，还是引导找管理员开权限。
 */
export function myCreatedEmptyHint(canViewOthersTasks: boolean): string {
  return canViewOthersTasks ? MY_CREATED_EMPTY_HINT : MY_CREATED_SCOPED_EMPTY_HINT
}
