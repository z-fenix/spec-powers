import { describe, expect, it } from 'vitest'
import {
  formatMention,
  parseMentionSegments,
  type MentionSegment,
} from './mentions'

describe('formatMention', () => {
  it('formats an agent mention link', () => {
    expect(formatMention({ kind: 'agent', id: 'a1', name: 'KunCoding' })).toBe(
      '[@KunCoding](mention://agent/a1)',
    )
  })

  it('formats a member mention link', () => {
    expect(formatMention({ kind: 'member', id: 'u1', name: '张三' })).toBe(
      '[@张三](mention://member/u1)',
    )
  })
})

describe('parseMentionSegments', () => {
  it('returns a single text segment without mentions', () => {
    expect(parseMentionSegments('hello world')).toEqual([
      { type: 'text', text: 'hello world' },
    ])
  })

  it('parses an agent mention link into a linked segment', () => {
    const segments = parseMentionSegments('[@KunCoding](mention://agent/a1) 请看')
    expect(segments).toEqual([
      {
        type: 'mention',
        mention: { kind: 'agent', id: 'a1', name: 'KunCoding' },
      },
      { type: 'text', text: ' 请看' },
    ])
  })

  it('parses a member mention link into a linked segment', () => {
    const segments = parseMentionSegments('hi [@张三](mention://member/u1)')
    expect(segments[1]).toEqual({
      type: 'mention',
      mention: { kind: 'member', id: 'u1', name: '张三' },
    })
  })

  it('parses a plain @token as an unlinked mention', () => {
    const segments = parseMentionSegments('先 @KunCoding 看看')
    expect(segments).toEqual([
      { type: 'text', text: '先 ' },
      {
        type: 'mention',
        mention: { kind: 'plain', id: '', name: 'KunCoding' },
      },
      { type: 'text', text: ' 看看' },
    ])
  })

  it('parses plain CJK mention tokens', () => {
    const segments = parseMentionSegments('@张三 你好')
    expect(segments[0]).toEqual({
      type: 'mention',
      mention: { kind: 'plain', id: '', name: '张三' },
    })
  })

  it('stops plain tokens at punctuation and word boundaries', () => {
    const segments = parseMentionSegments('@KunCoding, @KunCodingX')
    expect(segments).toHaveLength(3)
    expect(segments[0]).toEqual({
      type: 'mention',
      mention: { kind: 'plain', id: '', name: 'KunCoding' },
    })
    expect(segments[1]).toEqual({ type: 'text', text: ', ' })
    expect(segments[2]).toEqual({
      type: 'mention',
      mention: { kind: 'plain', id: '', name: 'KunCodingX' },
    })
  })

  it('does not treat a bare @ as a mention', () => {
    expect(parseMentionSegments('email me @ home')).toEqual([
      { type: 'text', text: 'email me @ home' },
    ])
  })

  it('keeps a non-mention markdown link as text', () => {
    const segments = parseMentionSegments('[click](https://example.com)')
    expect(segments).toEqual([{ type: 'text', text: '[click](https://example.com)' }])
  })

  it('round-trips formatted mentions', () => {
    const content =
      '[@KunCoding](mention://agent/a1) 和 [@张三](mention://member/u1) 一起'
    const segments: MentionSegment[] = parseMentionSegments(content)
    const mentions = segments.filter((s) => s.type === 'mention')
    expect(mentions).toHaveLength(2)
    expect(mentions[0].mention).toEqual({ kind: 'agent', id: 'a1', name: 'KunCoding' })
    expect(mentions[1].mention).toEqual({ kind: 'member', id: 'u1', name: '张三' })
  })
})
