import { describe, expect, it, vi } from 'vitest'
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MentionInput, type MentionCandidate } from './MentionInput'

const candidates: MentionCandidate[] = [
  { kind: 'agent', id: 'a1', name: 'KunCoding' },
  { kind: 'agent', id: 'a2', name: 'SpecBot' },
  { kind: 'member', id: 'u1', name: '张三' },
]

function Harness({ onInput }: { onInput?: (v: string) => void }) {
  const [value, setValue] = useState('')
  return (
    <MentionInput
      testId="comment-input"
      value={value}
      onChange={(v) => {
        setValue(v)
        onInput?.(v)
      }}
      candidates={candidates}
    />
  )
}

function setup() {
  const onInput = vi.fn()
  const utils = render(<Harness onInput={onInput} />)
  const input = screen.getByTestId('comment-input') as HTMLInputElement
  return { onInput, input, ...utils }
}

describe('MentionInput', () => {
  it('renders an input without suggestions initially', () => {
    setup()
    expect(screen.getByTestId('comment-input')).toBeInTheDocument()
    expect(screen.queryByTestId('mention-suggestions')).not.toBeInTheDocument()
  })

  it('shows matching suggestions after typing @', async () => {
    const user = userEvent.setup()
    setup()
    await user.type(screen.getByTestId('comment-input'), 'hello @Ku')
    const list = screen.getByTestId('mention-suggestions')
    expect(list).toBeInTheDocument()
    expect(screen.getByTestId('mention-option-0')).toHaveTextContent('KunCoding')
    expect(screen.queryByText('SpecBot')).not.toBeInTheDocument()
  })

  it('matches members case-insensitively and by CJK name', async () => {
    const user = userEvent.setup()
    setup()
    await user.type(screen.getByTestId('comment-input'), '@张')
    expect(screen.getByTestId('mention-option-0')).toHaveTextContent('张三')
  })

  it('hides suggestions when no candidate matches', async () => {
    const user = userEvent.setup()
    setup()
    await user.type(screen.getByTestId('comment-input'), '@zzz')
    expect(screen.queryByTestId('mention-suggestions')).not.toBeInTheDocument()
  })

  it('inserts a mention link when a suggestion is clicked', async () => {
    const user = userEvent.setup()
    const { input } = setup()
    await user.type(input, 'hi @Ku')
    await user.click(screen.getByTestId('mention-option-0'))
    expect(input.value).toBe('hi [@KunCoding](mention://agent/a1) ')
    expect(screen.queryByTestId('mention-suggestions')).not.toBeInTheDocument()
  })

  it('selects with keyboard: ArrowDown then Enter', async () => {
    const user = userEvent.setup()
    const { input } = setup()
    await user.type(input, '@')
    // two agent candidates plus the member start with... '@' matches all three
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')
    expect(input.value).toBe('[@张三](mention://member/u1) ')
  })

  it('closes suggestions on Escape', async () => {
    const user = userEvent.setup()
    const { input } = setup()
    await user.type(input, '@Ku')
    expect(screen.getByTestId('mention-suggestions')).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByTestId('mention-suggestions')).not.toBeInTheDocument()
  })

  it('does not open suggestions without a preceding @', async () => {
    const user = userEvent.setup()
    setup()
    await user.type(screen.getByTestId('comment-input'), 'Ku')
    expect(screen.queryByTestId('mention-suggestions')).not.toBeInTheDocument()
  })

  it('does not open suggestions for emails like a@b', async () => {
    const user = userEvent.setup()
    setup()
    await user.type(screen.getByTestId('comment-input'), 'mail a@Ku')
    expect(screen.queryByTestId('mention-suggestions')).not.toBeInTheDocument()
  })
})
