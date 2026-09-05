import { Link } from 'react-router-dom'
import { parseMentionSegments, type Mention } from '../lib/mentions'

function MentionChip({ mention }: { mention: Mention }) {
  if (mention.kind === 'agent') {
    return (
      <Link className="mention" to="/agents" data-testid={`mention-agent-${mention.id}`}>
        @{mention.name}
      </Link>
    )
  }
  if (mention.kind === 'member') {
    return (
      <Link
        className="mention"
        to={`/members/${mention.id}`}
        data-testid={`mention-member-${mention.id}`}
      >
        @{mention.name}
      </Link>
    )
  }
  return <span className="mention">@{mention.name}</span>
}

export function MentionText({ content }: { content: string }) {
  const segments = parseMentionSegments(content)
  return (
    <>
      {segments.map((seg, i) =>
        seg.type === 'mention' ? (
          <MentionChip key={i} mention={seg.mention!} />
        ) : (
          <span key={i}>{seg.text}</span>
        ),
      )}
    </>
  )
}
