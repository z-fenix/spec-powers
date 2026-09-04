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
