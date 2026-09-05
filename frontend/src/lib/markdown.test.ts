import { describe, expect, it } from 'vitest'
import { renderMarkdown } from './markdown'

describe('renderMarkdown', () => {
  it('renders markdown to sanitized html', () => {
    expect(renderMarkdown('# 标题\n\n**加粗** 文本')).toContain('<h1>标题</h1>')
    expect(renderMarkdown('# 标题\n\n**加粗** 文本')).toContain('<strong>加粗</strong>')
  })

  it('renders lists and links', () => {
    const html = renderMarkdown('- 项目一\n- 项目二\n\n[链接](https://example.com)')
    expect(html).toContain('<li>项目一</li>')
    expect(html).toContain('<li>项目二</li>')
    expect(html).toContain('<a href="https://example.com"')
  })

  it('strips script tags (XSS sanitization)', () => {
    const html = renderMarkdown('<p>safe</p><script>alert(1)</script>')
    expect(html).not.toContain('<script')
    expect(html).not.toContain('alert(1)')
    expect(html).toContain('safe')
  })

  it('strips event handler attributes (XSS sanitization)', () => {
    const html = renderMarkdown('<img src=x onerror="alert(1)">')
    expect(html).not.toContain('onerror')
  })

  it('strips javascript: link schemes (XSS sanitization)', () => {
    const html = renderMarkdown('[点击](javascript:alert(1))')
    expect(html).not.toContain('javascript:')
  })

  it('keeps plain text intact', () => {
    expect(renderMarkdown('普通评论，没有格式。')).toContain('普通评论，没有格式。')
  })
})
