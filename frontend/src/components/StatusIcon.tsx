import type { IssueStatus } from '../lib/status'

/* Status colors follow Multica's STATUS_CONFIG: muted for backlog/todo/
   cancelled, warning for in_progress, success for in_review, info for done,
   destructive for blocked. */
const STATUS_COLOR_CLASS: Record<string, string> = {
  backlog: 'status-muted',
  todo: 'status-muted',
  in_progress: 'status-warning',
  in_review: 'status-success',
  done: 'status-info',
  blocked: 'status-destructive',
  cancelled: 'status-muted',
}

/* Custom workspace statuses render through their category; built-in
   statuses pass their name, which doubles as the category. */
export function StatusIcon({ status, category }: { status: string; category?: string }) {
  const kind = category ?? status
  const cls = STATUS_COLOR_CLASS[kind] ?? 'status-muted'
  const common = {
    className: `status-icon ${cls}`,
    width: 14,
    height: 14,
    viewBox: '0 0 14 14',
    'aria-hidden': true as const,
  }

  switch (kind) {
    case 'in_progress':
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M7 1.5 A5.5 5.5 0 0 1 12.5 7 L7 7 Z" fill="currentColor" />
        </svg>
      )
    case 'in_review':
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M7 1.5 A5.5 5.5 0 0 1 12.5 7 L7 7 Z M7 7 L7 1.5 A5.5 5.5 0 0 0 1.5 7 Z" fill="currentColor" />
          <path d="M1.5 7 A5.5 5.5 0 0 0 7 12.5 L7 7 Z" fill="currentColor" />
        </svg>
      )
    case 'done':
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="6.5" fill="currentColor" />
          <path
            d="M4 7.2 L6.2 9.4 L10 5.2"
            fill="none"
            stroke="var(--surface)"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )
    case 'blocked':
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <line x1="3.2" y1="10.8" x2="10.8" y2="3.2" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      )
    case 'cancelled':
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M4.8 4.8 L9.2 9.2 M9.2 4.8 L4.8 9.2" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
      )
    case 'backlog':
      return (
        <svg {...common}>
          {Array.from({ length: 16 }, (_, i) => {
            const angle = (i / 16) * Math.PI * 2
            return (
              <circle
                key={i}
                cx={7 + 5.2 * Math.cos(angle)}
                cy={7 + 5.2 * Math.sin(angle)}
                r="0.7"
                fill="currentColor"
              />
            )
          })}
        </svg>
      )
    case 'todo':
    default:
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      )
  }
}

export type { IssueStatus }
