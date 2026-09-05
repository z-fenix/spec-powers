export type MentionKind = 'agent' | 'member' | 'plain'

export interface Mention {
  kind: MentionKind
  id: string
  name: string
}

export interface MentionSegment {
  type: 'text' | 'mention'
  text?: string
  mention?: Mention
}

const MENTION_LINK_RE = /\[@([^\]]+)\]\(mention:\/\/(agent|member)\/([^)\s]+)\)/g
const PLAIN_MENTION_RE = /@([\p{L}\p{N}_-]+)/gu

// formatMention emits the comment-body mention link format consumed by the
// renderer; the closing bracket also keeps the backend's plain "@<name>"
// matcher working on the same content.
export function formatMention(m: { kind: 'agent' | 'member'; id: string; name: string }): string {
  return `[@${m.name}](mention://${m.kind}/${m.id})`
}

export function parseMentionSegments(content: string): MentionSegment[] {
  const segments: MentionSegment[] = []
  let last = 0

  const pushText = (text: string) => {
    let cursor = 0
    for (const m of text.matchAll(PLAIN_MENTION_RE)) {
      const before = text.slice(cursor, m.index)
      if (before) segments.push({ type: 'text', text: before })
      segments.push({
        type: 'mention',
        mention: { kind: 'plain', id: '', name: m[1] },
      })
      cursor = m.index + m[0].length
    }
    if (cursor < text.length) {
      segments.push({ type: 'text', text: text.slice(cursor) })
    }
  }

  for (const match of content.matchAll(MENTION_LINK_RE)) {
    pushText(content.slice(last, match.index))
    segments.push({
      type: 'mention',
      mention: {
        kind: match[2] as 'agent' | 'member',
        id: match[3],
        name: match[1],
      },
    })
    last = match.index + match[0].length
  }
  pushText(content.slice(last))
  return segments
}
