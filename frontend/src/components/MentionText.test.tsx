import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { MentionText } from './MentionText'

function renderContent(content: string) {
  return render(
    <MemoryRouter>
      <MentionText content={content} />
    </MemoryRouter>,
  )
}

describe('MentionText', () => {
  it('renders plain text unchanged', () => {
    renderContent('普通评论内容')
    expect(screen.getByText('普通评论内容')).toBeInTheDocument()
  })

  it('renders an agent mention as a styled link to the agents page', () => {
    const { container } = renderContent('[@KunCoding](mention://agent/a1) 请查看')
    const link = screen.getByText('@KunCoding')
    expect(link).toHaveClass('mention')
    expect(link).toHaveAttribute('href', '/agents')
    expect(container.textContent).toBe('@KunCoding 请查看')
  })

  it('renders a member mention as a link to the member profile page', () => {
    renderContent('hi [@张三](mention://member/u1)')
    const link = screen.getByText('@张三')
    expect(link).toHaveAttribute('href', '/members/u1')
  })

  it('renders a plain @token as a mention chip without a link', () => {
    renderContent('先 @KunCoding 看看')
    const chip = screen.getByText('@KunCoding')
    expect(chip).toHaveClass('mention')
    expect(chip.tagName).toBe('SPAN')
  })

  it('renders multiple mentions and text in order', () => {
    const { container } = render(
      <MemoryRouter>
        <MentionText content="[@A](mention://agent/a1) 和 [@B](mention://member/u2) 与 @C" />
      </MemoryRouter>,
    )
    const chips = container.querySelectorAll('.mention')
    expect(chips).toHaveLength(3)
    expect(chips[0].textContent).toBe('@A')
    expect(chips[1].textContent).toBe('@B')
    expect(chips[2].textContent).toBe('@C')
  })
})
