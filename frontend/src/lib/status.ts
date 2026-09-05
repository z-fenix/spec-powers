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

export const PRIORITIES = ['none', 'low', 'medium', 'high', 'urgent'] as const

export const PRIORITY_LABELS: Record<string, string> = {
  none: '无',
  low: '低',
  medium: '中',
  high: '高',
  urgent: '紧急',
}

export function parseLabels(raw: string): string[] {
  return raw
    .split(/[,，;；\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

// Overdue: due date (YYYY-MM-DD, possibly with a time suffix) is before
// today and the issue has not reached a terminal state.
export function isOverdue(dueDate: string, status: string): boolean {
  if (!dueDate) return false
  if (status === 'done' || status === 'cancelled') return false
  const day = dueDate.slice(0, 10)
  const today = new Date()
  const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
  return day < todayStr
}
