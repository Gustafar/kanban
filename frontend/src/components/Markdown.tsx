import { Fragment } from 'react'
import type { ReactNode } from 'react'

// Small, safe Markdown subset for AI summaries and comments: paragraphs, lists, headings, quotes,
// fenced code, `code`, **bold** and http(s) links. It builds React nodes, never raw HTML.

type Block =
  | { t: 'p'; lines: string[] }
  | { t: 'ul' | 'ol'; items: string[] }
  | { t: 'quote'; lines: string[] }
  | { t: 'code'; text: string }
  | { t: 'h'; level: number; text: string }

const FENCE = /^\s*```/
const HEADING = /^(#{1,4})\s+(.*)$/
const BULLET = /^\s*[-*•]\s+/
const NUMBERED = /^\s*\d+[.)]\s+/
const QUOTE = /^\s*>\s?/
const INLINE = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\[[^\]\n]+\]\(https?:\/\/[^)\s]+\))/g

const startsBlock = (l: string) => FENCE.test(l) || HEADING.test(l) || BULLET.test(l) || NUMBERED.test(l) || QUOTE.test(l)

function parse(src: string): Block[] {
  const lines = src.replace(/\r\n/g, '\n').split('\n')
  const out: Block[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (FENCE.test(line)) {
      const buf: string[] = []
      i++
      while (i < lines.length && !FENCE.test(lines[i])) buf.push(lines[i++])
      i++ // closing fence
      out.push({ t: 'code', text: buf.join('\n') })
    } else if (!line.trim()) {
      i++
    } else if (HEADING.test(line)) {
      const m = HEADING.exec(line)!
      out.push({ t: 'h', level: m[1].length, text: m[2] })
      i++
    } else if (BULLET.test(line) || NUMBERED.test(line)) {
      const ordered = NUMBERED.test(line)
      const re = ordered ? NUMBERED : BULLET
      const items: string[] = []
      while (i < lines.length && re.test(lines[i])) items.push(lines[i++].replace(re, ''))
      out.push({ t: ordered ? 'ol' : 'ul', items })
    } else if (QUOTE.test(line)) {
      const buf: string[] = []
      while (i < lines.length && QUOTE.test(lines[i])) buf.push(lines[i++].replace(QUOTE, ''))
      out.push({ t: 'quote', lines: buf })
    } else {
      const buf: string[] = []
      while (i < lines.length && lines[i].trim() && !startsBlock(lines[i])) buf.push(lines[i++])
      out.push({ t: 'p', lines: buf })
    }
  }
  return out
}

function inline(text: string, k: string): ReactNode[] {
  return text.split(INLINE).map((part, i) => {
    const key = `${k}-${i}`
    if (part.length > 2 && part.startsWith('`') && part.endsWith('`')) return <code key={key}>{part.slice(1, -1)}</code>
    if (part.length > 4 && part.startsWith('**') && part.endsWith('**')) return <strong key={key}>{part.slice(2, -2)}</strong>
    const link = /^\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)$/.exec(part)
    if (link) return <a key={key} href={link[2]} target="_blank" rel="noreferrer">{link[1]}</a>
    return <Fragment key={key}>{part}</Fragment>
  })
}

function lined(lines: string[], k: string): ReactNode[] {
  return lines.flatMap((l, i) => (i === 0 ? inline(l, `${k}-${i}`) : [<br key={`${k}-br${i}`} />, ...inline(l, `${k}-${i}`)]))
}

export function Markdown({ text }: { text: string }) {
  return (
    <div className="md">
      {parse(text).map((b, i) => {
        const k = `b${i}`
        switch (b.t) {
          case 'h': return <p key={k} className={`md-h md-h${Math.min(b.level, 3)}`}>{inline(b.text, k)}</p>
          case 'ul': return <ul key={k}>{b.items.map((it, j) => <li key={j}>{inline(it, `${k}-${j}`)}</li>)}</ul>
          case 'ol': return <ol key={k}>{b.items.map((it, j) => <li key={j}>{inline(it, `${k}-${j}`)}</li>)}</ol>
          case 'quote': return <blockquote key={k}>{lined(b.lines, k)}</blockquote>
          case 'code': return <pre key={k}><code>{b.text}</code></pre>
          default: return <p key={k}>{lined(b.lines, k)}</p>
        }
      })}
    </div>
  )
}
