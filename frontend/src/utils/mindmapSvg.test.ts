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
