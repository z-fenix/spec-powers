import { useParams } from 'react-router-dom'

export function MemberProfilePage() {
  const { userId = '' } = useParams()
  return (
    <div className="page" data-testid="member-profile">
      <div className="page-header">
        <h1 className="page-title">成员</h1>
      </div>
      <div className="page-body">
      <p>
        成员 ID: <code data-testid="member-id">{userId}</code>
      </p>
      <p>个人主页建设中。</p>
    </div>
    </div>
  )
}
