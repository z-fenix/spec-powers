import { useEffect, useState } from 'react'
import { getChangeByIssue, getGuard, type Change, type GuardReport } from '../api/workflow'
import { ApiError } from '../api/client'

const PHASES = [
  { key: 'proposal', label: '提案' },
  { key: 'specs', label: '规格' },
  { key: 'design', label: '设计' },
  { key: 'tasks', label: '任务' },
] as const

const GATES = [
  { key: 'phase_legal', label: '阶段产物齐备' },
  { key: 'handoff_fresh', label: '阶段交接有效' },
  { key: 'verify_passed', label: '验证报告通过' },
  { key: 'can_advance', label: '可进入下一阶段' },
  { key: 'can_archive', label: '可归档' },
] as const

export function phaseState(phase: string, key: string): 'done' | 'current' | 'upcoming' {
  const order: string[] = PHASES.map((p) => p.key)
  const current = order.indexOf(phase)
  const idx = order.indexOf(key)
  if (idx < current) return 'done'
  if (idx === current) return 'current'
  return 'upcoming'
}

export function WorkflowProgress({ issueId }: { issueId: string }) {
  const [change, setChange] = useState<Change | null>(null)
  const [guard, setGuard] = useState<GuardReport | null>(null)
  const [missing, setMissing] = useState(false)

  useEffect(() => {
    let cancelled = false
    setMissing(false)
    getChangeByIssue(issueId)
      .then((c) => {
        if (cancelled) return
        setChange(c)
        return getGuard(c.id).then((g) => {
          if (!cancelled) setGuard(g)
        })
      })
      .catch((err) => {
        if (!cancelled && err instanceof ApiError && err.status === 404) {
          setMissing(true)
        }
      })
    return () => {
      cancelled = true
    }
  }, [issueId])

  if (missing) return <div data-testid="workflow-empty" />
  if (!change || !guard) return null

  return (
    <section data-testid="workflow-progress" className="workflow-progress">
      <h3>工作流进度</h3>
      <ol className="phase-stepper">
        {PHASES.map((p) => (
          <li key={p.key} data-testid={`phase-${p.key}`} data-state={phaseState(change.phase, p.key)}>
            {p.label}
          </li>
        ))}
      </ol>
      <h4>门禁结果</h4>
      <ul data-testid="guard-gates" className="guard-gates">
        {GATES.map((g) => (
          <li key={g.key} data-testid={`gate-${g.key}`} data-passed={String(guard[g.key as keyof GuardReport] as boolean)}>
            {g.label}：{guard[g.key as keyof GuardReport] ? '通过' : '未通过'}
          </li>
        ))}
      </ul>
      {guard.reasons.length > 0 && (
        <div data-testid="guard-reasons">
          <h4>阻塞原因</h4>
          <ul>
            {guard.reasons.map((r, i) => (
              <li key={i}>{r}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
