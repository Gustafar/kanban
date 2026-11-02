// A run's diff, grouped by file and collapsible, with +/- counts per file.

interface FileDiff { name: string; adds: number; dels: number; lines: string[] }

function split(diff: string): FileDiff[] {
  const files: FileDiff[] = []
  let cur: FileDiff | null = null
  for (const l of diff.split('\n')) {
    if (l.startsWith('diff --git ')) {
      const m = / b\/(.+)$/.exec(l)
      cur = { name: m ? m[1] : l.slice(11), adds: 0, dels: 0, lines: [] }
      files.push(cur)
    } else if (cur) {
      if (l.startsWith('index ') || l.startsWith('--- ') || l.startsWith('+++ ')) continue // noise the file header already conveys
      if (l.startsWith('+')) cur.adds++
      else if (l.startsWith('-')) cur.dels++
      cur.lines.push(l)
    }
  }
  return files
}

const kind = (l: string) => (l.startsWith('@@') ? 'hunk' : l.startsWith('+') ? 'add' : l.startsWith('-') ? 'del' : l.startsWith('new file') || l.startsWith('deleted file') || l.startsWith('similarity') || l.startsWith('rename') ? 'meta' : '')

export function DiffView({ diff, direct }: { diff: string; direct: boolean }) {
  if (!diff.trim()) {
    return <p className="empty-note">{direct ? 'Nenhuma mudança nos arquivos desta execução.' : 'Nenhuma mudança na branch desta execução.'}</p>
  }
  const files = split(diff)
  const adds = files.reduce((n, f) => n + f.adds, 0)
  const dels = files.reduce((n, f) => n + f.dels, 0)
  return (
    <div className="diff-view">
      <p className="diff-total">{files.length} {files.length === 1 ? 'arquivo' : 'arquivos'} <span className="plus">+{adds}</span> <span className="minus">−{dels}</span></p>
      {files.map(f => (
        <details key={f.name} className="diff-file" open={files.length <= 3}>
          <summary>
            <span className="diff-name" title={f.name}>{f.name}</span>
            <span className="diff-counts"><span className="plus">+{f.adds}</span> <span className="minus">−{f.dels}</span></span>
          </summary>
          <pre className="diff">
            {f.lines.map((l, i) => <span key={i} className={kind(l)}>{l + '\n'}</span>)}
          </pre>
        </details>
      ))}
    </div>
  )
}
