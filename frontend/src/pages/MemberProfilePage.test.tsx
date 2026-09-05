import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { MemberProfilePage } from './MemberProfilePage'

describe('MemberProfilePage', () => {
  it('shows the member id as a placeholder profile', () => {
    render(
      <MemoryRouter initialEntries={['/members/u1']}>
        <Routes>
          <Route path="/members/:userId" element={<MemberProfilePage />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByTestId('member-profile')).toBeInTheDocument()
    expect(screen.getByTestId('member-id')).toHaveTextContent('u1')
  })
})
