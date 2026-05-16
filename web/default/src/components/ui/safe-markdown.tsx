/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'

type SafeMarkdownProps = {
  children: string
  className?: string
}

const SAFE_HREF_PROTOCOLS = new Set(['http:', 'https:', 'mailto:', 'tel:'])
const HREF_SCHEME_PATTERN = /^[a-zA-Z][a-zA-Z\d+.-]*:/
const HREF_CONTROL_CHAR_PATTERN = /[\u0000-\u001F\u007F]/u

export function isSafeHref(href: string | undefined): boolean {
  if (!href) return false

  const trimmed = href.trim()
  if (!trimmed || HREF_CONTROL_CHAR_PATTERN.test(trimmed)) return false
  if (trimmed.startsWith('#')) return true
  if (trimmed.startsWith('./') || trimmed.startsWith('../')) return true
  if (trimmed.startsWith('/')) return !trimmed.startsWith('//')
  if (!HREF_SCHEME_PATTERN.test(trimmed)) return true

  try {
    return SAFE_HREF_PROTOCOLS.has(new URL(trimmed).protocol)
  } catch {
    return false
  }
}

export function safeMarkdownUrlTransform(url: string): string {
  const trimmed = url.trim()
  return isSafeHref(trimmed) ? trimmed : ''
}

export function SafeMarkdown(props: SafeMarkdownProps) {
  return (
    <div
      className={cn(
        'prose prose-sm dark:prose-invert max-w-none',
        'prose-headings:font-semibold prose-headings:tracking-tight',
        'prose-h1:text-2xl prose-h2:text-xl prose-h3:text-lg',
        'prose-p:leading-relaxed prose-p:my-2',
        'prose-a:text-primary prose-a:no-underline hover:prose-a:underline',
        'prose-code:bg-muted prose-code:px-1 prose-code:py-0.5 prose-code:rounded prose-code:before:content-none prose-code:after:content-none',
        'prose-pre:bg-muted prose-pre:border',
        'prose-blockquote:border-l-primary prose-blockquote:bg-muted/50 prose-blockquote:py-1',
        'prose-ul:my-2 prose-ol:my-2 prose-li:my-1',
        'prose-table:border prose-thead:bg-muted',
        'prose-td:border prose-th:border prose-td:px-3 prose-th:px-3',
        'prose-img:rounded-lg prose-img:shadow-sm',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        '[overflow-wrap:anywhere] break-words',
        props.className
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        urlTransform={safeMarkdownUrlTransform}
        components={{
          a: (anchorProps) => {
            if (!isSafeHref(anchorProps.href)) {
              return <span>{anchorProps.children}</span>
            }

            return (
              <a
                {...anchorProps}
                href={anchorProps.href?.trim()}
                target='_blank'
                rel='noopener noreferrer'
              />
            )
          },
        }}
      >
        {props.children}
      </ReactMarkdown>
    </div>
  )
}
