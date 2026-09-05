import { useEffect, useRef, useState, type ChangeEvent, type KeyboardEvent } from 'react'
import { formatMention } from '../lib/mentions'

export interface MentionCandidate {
  kind: 'agent' | 'member'
  id: string
  name: string
}

const TOKEN_RE = /^[\p{L}\p{N}_-]*$/u

const NAV_KEYS = new Set(['ArrowDown', 'ArrowUp', 'Enter', 'Tab', 'Escape'])

// activeMentionQuery returns the @token immediately before the caret, or null
// when the caret is not inside one. A token only counts when the @ starts the
// text or follows whitespace — so email addresses like a@b never trigger.
export function activeMentionQuery(text: string, caret: number): string | null {
  const upto = text.slice(0, caret)
  const at = upto.lastIndexOf('@')
  if (at < 0) return null
  if (at > 0 && !/\s/u.test(upto[at - 1])) return null
  const token = upto.slice(at + 1)
  if (!TOKEN_RE.test(token)) return null
  return token
}

export function filterCandidates(
  candidates: MentionCandidate[],
  query: string,
): MentionCandidate[] {
  const q = query.toLowerCase()
  return candidates.filter(
    (c) => c.name && c.name.toLowerCase().includes(q),
  )
}

export function MentionInput({
  value,
  onChange,
  candidates,
  testId,
  placeholder,
}: {
  value: string
  onChange: (value: string) => void
  candidates: MentionCandidate[]
  testId: string
  placeholder?: string
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const pendingCaret = useRef<number | null>(null)
  const [query, setQuery] = useState<string | null>(null)
  const [highlight, setHighlight] = useState(0)
  const [dismissed, setDismissed] = useState(false)

  const matches = query === null ? [] : filterCandidates(candidates, query)
  const open = query !== null && !dismissed && matches.length > 0

  useEffect(() => {
    if (pendingCaret.current !== null && inputRef.current) {
      inputRef.current.setSelectionRange(pendingCaret.current, pendingCaret.current)
      pendingCaret.current = null
    }
  }, [value])

  const syncQuery = () => {
    const caret = inputRef.current?.selectionStart ?? value.length
    setQuery(activeMentionQuery(value, caret))
    setHighlight(0)
    setDismissed(false)
  }

  const select = (candidate: MentionCandidate) => {
    const input = inputRef.current
    const caret = input?.selectionStart ?? value.length
    const upto = value.slice(0, caret)
    const at = upto.lastIndexOf('@')
    const next =
      value.slice(0, at) + formatMention({ ...candidate }) + ' ' + value.slice(caret)
    pendingCaret.current = at + formatMention({ ...candidate }).length + 1
    onChange(next)
    setQuery(null)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (!open) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlight((h) => (h + 1) % matches.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlight((h) => (h - 1 + matches.length) % matches.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      select(matches[highlight])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setDismissed(true)
    }
  }

  return (
    <div className="mention-input">
      <input
        ref={inputRef}
        data-testid={testId}
        placeholder={placeholder}
        value={value}
        onChange={(e: ChangeEvent<HTMLInputElement>) => {
          onChange(e.target.value)
          const caret = e.target.selectionStart ?? e.target.value.length
          setQuery(activeMentionQuery(e.target.value, caret))
          setHighlight(0)
          setDismissed(false)
        }}
        onClick={syncQuery}
        onKeyUp={(e) => {
          // Navigation/selection keys already update the dropdown; syncing on
          // their keyup would clobber the highlight or reopen a dismissed list.
          if (!NAV_KEYS.has(e.key)) syncQuery()
        }}
        onKeyDown={onKeyDown}
      />
      {open && (
        <ul className="mention-suggestions" data-testid="mention-suggestions">
          {matches.map((c, i) => (
            <li key={`${c.kind}-${c.id}`}>
              <button
                type="button"
                data-testid={`mention-option-${i}`}
                className={i === highlight ? 'mention-option active' : 'mention-option'}
                onMouseDown={(e) => {
                  e.preventDefault()
                  select(c)
                }}
              >
                <span className="mention-kind">{c.kind === 'agent' ? 'Agent' : '成员'}</span>
                {c.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
