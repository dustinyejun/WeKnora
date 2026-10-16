import assert from 'node:assert/strict'
import test from 'node:test'
import { renderMindTreeSvg, wrapText } from './mindmapSvg'

test('every node becomes one box and one connector per edge', () => {
  const svg = renderMindTreeSvg({
    title: '根',
    children: [
      { title: '甲', children: [{ title: '甲1' }, { title: '甲2' }] },
      { title: '乙' },
    ],
  })
  assert.equal(svg.match(/<rect x=/g)?.length, 5)
  assert.equal(svg.match(/<path /g)?.length, 4)
  assert.ok(svg.startsWith('<svg '))
})

test('node text is XML-escaped', () => {
  const svg = renderMindTreeSvg({ title: '根', children: [{ title: 'a < b & "c"' }] })
  assert.ok(svg.includes('a &lt; b &amp; &quot;c&quot;'))
  assert.ok(!svg.includes('a < b'))
})

test('long text wraps into several lines instead of one overflowing line', () => {
  const lines = wrapText('这是一个很长的节点文字'.repeat(6), 120, 13)
  assert.ok(lines.length > 1)
  assert.equal(lines.join(''), '这是一个很长的节点文字'.repeat(6))
})
import type { MindNode } from './answerMindmap'

type Box = { x: number; y: number; w: number; h: number }

// Node boxes only: the white background rect has no x/y attributes.
function boxes(svg: string): Box[] {
  return [...svg.matchAll(/<rect x="([\d.\-]+)" y="([\d.\-]+)" width="([\d.\-]+)" height="([\d.\-]+)"/g)].map(
    (m) => ({ x: Number(m[1]), y: Number(m[2]), w: Number(m[3]), h: Number(m[4]) }),
  )
}

function canvas(svg: string): { width: number; height: number } {
  const m = svg.match(/<svg[^>]* width="([\d.]+)" height="([\d.]+)"/)
  assert.ok(m, 'svg declares width and height')
  return { width: Number(m[1]), height: Number(m[2]) }
}

function assertLaidOut(svg: string) {
  const all = boxes(svg)
  const { width, height } = canvas(svg)
  for (const b of all) {
    assert.ok(b.y >= 0, `box above the top edge: y=${b.y}`)
    assert.ok(b.y + b.h <= height + 1e-6, `box below the canvas: bottom=${b.y + b.h} height=${height}`)
    assert.ok(b.x + b.w <= width + 1e-6, `box past the right edge: right=${b.x + b.w} width=${width}`)
  }
  const byColumn = new Map<number, Box[]>()
  for (const b of all) byColumn.set(b.x, [...(byColumn.get(b.x) ?? []), b])
  for (const col of byColumn.values()) {
    col.sort((a, b) => a.y - b.y)
    for (let i = 1; i < col.length; i++) {
      const prev = col[i - 1]
      assert.ok(
        col[i].y >= prev.y + prev.h - 1e-6,
        `boxes overlap in one column: ${prev.y}+${prev.h} vs ${col[i].y}`,
      )
    }
  }
}

const long = '这是一个足够长的分支标题用来触发多行换行'.repeat(5).slice(0, 69)

test('parents taller than their children neither overlap nor leave the canvas', () => {
  const root: MindNode = {
    title: '根节点',
    children: [
      { title: long, children: [{ title: '短', children: [] }] },
      { title: long, children: [{ title: '短', children: [] }] },
    ],
  }
  assertLaidOut(renderMindTreeSvg(root))
})

test('a tall parent next to a leaf-heavy sibling keeps every box in its band', () => {
  const root: MindNode = {
    title: '根',
    children: [
      { title: long, children: [{ title: 'a', children: [] }] },
      { title: '下一个', children: [{ title: 'b', children: [] }, { title: 'c', children: [] }] },
    ],
  }
  assertLaidOut(renderMindTreeSvg(root))
})

test('leaf-heavy trees still stack leaves one row gap apart', () => {
  const root: MindNode = {
    title: '根',
    children: [{ title: '分支', children: [{ title: '一', children: [] }, { title: '二', children: [] }, { title: '三', children: [] }] }],
  }
  const svg = renderMindTreeSvg(root)
  assertLaidOut(svg)
  const all = boxes(svg)
  const leafX = Math.max(...all.map((b) => b.x))
  const leaves = all.filter((b) => b.x === leafX).sort((a, b) => a.y - b.y)
  assert.equal(leaves.length, 3)
  for (let i = 1; i < leaves.length; i++) {
    // ROW_GAP in mindmapSvg.ts is 14.
    assert.ok(Math.abs(leaves[i].y - (leaves[i - 1].y + leaves[i - 1].h) - 14) < 1e-6, 'leaves sit one row gap apart')
  }
})
