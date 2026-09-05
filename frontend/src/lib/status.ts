export const STATUSES = [
  'backlog',
  'todo',
  'in_progress',
  'in_review',
  'blocked',
  'done',
  'cancelled',
] as const

export type IssueStatus = (typeof STATUSES)[number]

export const STATUS_LABELS: Record<string, string> = {
  backlog: '待排期',
  todo: '待办',
  in_progress: '进行中',
  in_review: '评审中',
  blocked: '阻塞',
  done: '完成',
  cancelled: '已取消',
}

export const STATUS_CATEGORIES = [
  'backlog',
  'todo',
  'in_progress',
  'in_review',
  'blocked',
  'done',
  'cancelled',
] as const

export const CATEGORY_LABELS: Record<string, string> = {
  backlog: '待排期',
  todo: '待办',
  in_progress: '进行中',
  in_review: '评审中',
  blocked: '阻塞',
  done: '完成',
  cancelled: '已取消',
}

export interface StatusEntry {
  name: string
  category: string
  position: number
}

// The built-in directory: every workspace uses it until it customizes.
export const DEFAULT_DIRECTORY: StatusEntry[] = STATUSES.map((s, i) => ({
  name: s,
  category: s,
  position: i,
}))

export function statusLabel(name: string): string {
  return STATUS_LABELS[name] ?? name
}
