// Mind map tree from a Markdown answer, used when the model call fails.
// Structure only: headings and list items become nodes, nothing is summarised.
// ponytail: flat answers give a flat map; the model path is the main one.

export interface MindNode {
  title: string
  children?: MindNode[]
}

const MAX_NODES = 120
const MAX_LABEL = 120

export function sanitizeMindLabel(raw: string): string {
  const text = raw
    .replace(/<\/?(?:kb|web)\b[^>]*>/gi, '')
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[*_`~#>|]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (!text) return '…'
  return text.length > MAX_LABEL ? `${text.slice(0, MAX_LABEL)}…` : text
}

interface RawLine {
  type: 'heading' | 'item' | 'text'
  level: number
  text: string
}

function parseMarkdown(markdown: string): RawLine[] {
  const lines: RawLine[] = []
  let inFence = false
  for (const line of markdown.split(/\r?\n/)) {
    if (/^\s*(```|~~~)/.test(line)) {
      inFence = !inFence
      continue
    }
    if (inFence || !line.trim()) continue
    if (/^\s*\|/.test(line) || /^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) continue

    const heading = /^(#{1,6})\s+(.*)$/.exec(line)
    if (heading) {
      lines.push({ type: 'heading', level: heading[1].length, text: heading[2] })
      continue
    }
    const item = /^(\s*)(?:[-*+]|\d+[.)])\s+(.*)$/.exec(line)
    if (item) {
      const indent = item[1].replace(/\t/g, '    ').length
      lines.push({ type: 'item', level: Math.floor(indent / 2), text: item[2] })
      continue
    }
    lines.push({ type: 'text', level: 0, text: line.trim() })
  }
  return lines
}

// Plain prose with no structure: one node per sentence.
function sentencesOf(lines: RawLine[]): RawLine[] {
  return lines.flatMap((line) =>
    line.text
      .split(/(?<=[。！？!?])/)
      .map((s) => s.trim())
      .filter(Boolean)
      .map((text) => ({ type: 'text' as const, level: 0, text })),
  )
}

export function markdownToMindTree(markdown: string, rootTitle = '回答'): MindNode | null {
  const parsed = parseMarkdown(markdown)
  if (!parsed.length) return null

  const headings = parsed.filter((l) => l.type === 'heading')
  const base = headings.length ? Math.min(...headings.map((h) => h.level)) : 1
  const hasStructure = parsed.some((l) => l.type !== 'text')
  const lines = hasStructure ? parsed : sentencesOf(parsed)

  // Logical depth: heading depth, plus list nesting under it. Clamped so the
  // tree never skips a level.
  const flat: { depth: number; text: string }[] = []
  let headingDepth = 0
  let prev = 0
  for (const line of lines) {
    let depth: number
    if (line.type === 'heading') {
      headingDepth = line.level - base + 1
      depth = headingDepth
    } else if (line.type === 'item') {
      depth = headingDepth + 1 + line.level
    } else {
      depth = headingDepth + 1
    }
    depth = Math.max(1, Math.min(depth, prev + 1))
    prev = depth
    flat.push({ depth, text: sanitizeMindLabel(line.text) })
    if (flat.length >= MAX_NODES) break
  }

  const root: MindNode = { title: sanitizeMindLabel(rootTitle), children: [] }
  const stack: { depth: number; node: MindNode }[] = [{ depth: 0, node: root }]
  for (const { depth, text } of flat) {
    while (stack.length > 1 && stack[stack.length - 1].depth >= depth) stack.pop()
    const node: MindNode = { title: text }
    const parent = stack[stack.length - 1].node
    ;(parent.children ??= []).push(node)
    stack.push({ depth, node })
  }
  return root
}

// File name from the map's root title, e.g. "行政事业单位内部控制规范（试行）结构".
// Characters that are invalid in file names are dropped.
export function mindmapFileName(title: string): string {
  const name = title
    .replace(/[\\/:*?"<>|\u0000-\u001f]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 60)
    .trim()
  return name ? `${name}脑图` : '脑图'
}
