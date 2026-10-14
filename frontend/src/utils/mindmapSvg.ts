// Left-to-right mind map as standalone SVG, styled after the structure maps
// the chat exports: dark root, one colour per top-level branch, wrapped node
// text, curved connectors. Positions are computed here, not by a graph library.
import type { MindNode } from './answerMindmap'

const PALETTE = ['#1f6feb', '#0f766e', '#b45309', '#7c3aed', '#be123c', '#15803d', '#475569']
const ROOT_FILL = '#0b3d6b'
const TEXT_FILL = '#1e293b'
const FONT =
  "'Noto Sans CJK SC','Source Han Sans SC','PingFang SC','Microsoft YaHei','Hiragino Sans GB','WenQuanYi Zen Hei',SimHei,sans-serif"

const MARGIN = 40
const COL_GAP = 80
const ROW_GAP = 14
const PAD_X = 12
const PAD_Y = 8
const NODE_TEXT_W = 260
const ROOT_TEXT_W = 340

const fontSize = (depth: number) => (depth === 0 ? 18 : depth === 1 ? 15 : 13)

// CJK and full-width characters take a full em; Latin text is narrower.
const charWidth = (ch: string, px: number) => (ch.charCodeAt(0) >= 0x2e80 ? px : px * 0.56)

const textWidth = (text: string, px: number) => {
  let w = 0
  for (const ch of text) w += charWidth(ch, px)
  return w
}

export function wrapText(text: string, maxW: number, px: number): string[] {
  const lines: string[] = []
  let cur = ''
  let w = 0
  for (const ch of text) {
    const cw = charWidth(ch, px)
    if (w + cw > maxW && cur) {
      lines.push(cur)
      cur = ''
      w = 0
    }
    cur += ch
    w += cw
  }
  if (cur || !lines.length) lines.push(cur)
  return lines
}

const escapeXml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

interface Laid {
  depth: number
  color: string
  lines: string[]
  px: number
  lineH: number
  w: number
  h: number
  x: number
  cy: number
  children: Laid[]
}

function build(node: MindNode, depth: number, color: string, colorOf?: (i: number) => string): Laid {
  const px = fontSize(depth)
  const lines = wrapText(node.title, depth === 0 ? ROOT_TEXT_W : NODE_TEXT_W, px)
  const lineH = px * 1.45
  const children = (node.children ?? []).map((c, i) =>
    build(c, depth + 1, colorOf ? colorOf(i) : color),
  )
  return {
    depth,
    color,
    lines,
    px,
    lineH,
    w: Math.max(...lines.map((l) => textWidth(l, px))) + PAD_X * 2,
    h: lines.length * lineH + PAD_Y * 2,
    x: 0,
    cy: 0,
    children,
  }
}

export function renderMindTreeSvg(root: MindNode): string {
  const laid = build(root, 0, ROOT_FILL, (i) => PALETTE[i % PALETTE.length])

  // Column x per depth: each column is as wide as its widest box.
  const colW: number[] = []
  const walkWidths = (n: Laid) => {
    colW[n.depth] = Math.max(colW[n.depth] ?? 0, n.w)
    n.children.forEach(walkWidths)
  }
  walkWidths(laid)
  const colX: number[] = [MARGIN]
  for (let d = 1; d < colW.length; d++) colX[d] = colX[d - 1] + colW[d - 1] + COL_GAP

  // Leaves stack top to bottom; a parent centres on its first and last child.
  let cursor = MARGIN
  const placeY = (n: Laid) => {
    if (!n.children.length) {
      n.cy = cursor + n.h / 2
      cursor += n.h + ROW_GAP
    } else {
      n.children.forEach(placeY)
      n.cy = (n.children[0].cy + n.children[n.children.length - 1].cy) / 2
    }
    n.x = colX[n.depth]
  }
  placeY(laid)
  const height = cursor - ROW_GAP + MARGIN
  const width = colX[colW.length - 1] + colW[colW.length - 1] + MARGIN

  const paths: string[] = []
  const boxes: string[] = []
  const texts: string[] = []
  const walk = (n: Laid) => {
    const top = n.cy - n.h / 2
    if (n.depth === 0) {
      boxes.push(`<rect x="${n.x}" y="${top}" width="${n.w}" height="${n.h}" rx="10" fill="${ROOT_FILL}"/>`)
    } else {
      boxes.push(
        `<rect x="${n.x}" y="${top}" width="${n.w}" height="${n.h}" rx="8" fill="${n.color}" fill-opacity="0.1" stroke="${n.color}" stroke-width="1.4"/>`,
      )
    }
    const fill = n.depth === 0 ? '#ffffff' : TEXT_FILL
    const weight = n.depth <= 1 ? 700 : 400
    const tspans = n.lines
      .map((line, i) => {
        const y = top + PAD_Y + n.px + i * n.lineH
        return `<tspan x="${n.x + PAD_X}" y="${y}">${escapeXml(line)}</tspan>`
      })
      .join('')
    texts.push(`<text font-size="${n.px}" font-weight="${weight}" fill="${fill}">${tspans}</text>`)

    for (const c of n.children) {
      const x1 = n.x + n.w
      const x2 = c.x
      const mid = (x1 + x2) / 2
      paths.push(
        `<path d="M ${x1} ${n.cy} C ${mid} ${n.cy} ${mid} ${c.cy} ${x2} ${c.cy}" fill="none" stroke="${c.color}" stroke-width="2.2" stroke-opacity="0.6"/>`,
      )
      walk(c)
    }
  }
  walk(laid)

  return [
    `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" font-family="${escapeXml(FONT)}">`,
    `<rect width="100%" height="100%" fill="#ffffff"/>`,
    ...paths,
    ...boxes,
    ...texts,
    `</svg>`,
  ].join('\n')
}
