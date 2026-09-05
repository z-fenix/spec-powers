import { useParams } from 'react-router-dom'

export function MemberProfilePage() {
  const { userId = '' } = useParams()
  return (
    <section data-testid="member-profile">
      <h2>成员</h2>
      <p>
        成员 ID: <code data-testid="member-id">{userId}</code>
      </p>
      <p>个人主页建设中。</p>
    </section>
  )
}
