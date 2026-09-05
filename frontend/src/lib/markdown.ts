import { marked } from 'marked'
import DOMPurify from 'dompurify'

export function renderMarkdown(content: string): string {
  return DOMPurify.sanitize(marked.parse(content, { async: false }) as string)
}
