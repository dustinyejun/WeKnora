import assert from 'node:assert/strict'
import test from 'node:test'
import { markdownToMindTree, mindmapFileName, sanitizeMindLabel } from './answerMindmap'

test('headings and nested list items become a nested tree', () => {
  const tree = markdownToMindTree(
    ['# 概述', '- 目标一', '  - 子项', '## 方案', '1. 步骤'].join('\n'),
    '什么是内控',
  )
  assert.deepEqual(tree, {
    title: '什么是内控',
    children: [
      {
        title: '概述',
        children: [
          { title: '目标一', children: [{ title: '子项' }] },
          { title: '方案', children: [{ title: '步骤' }] },
        ],
      },
    ],
  })
})

test('indentation never skips a level', () => {
  const tree = markdownToMindTree('### 深层\n- 直接子项', '题')
  assert.deepEqual(tree, { title: '题', children: [{ title: '深层', children: [{ title: '直接子项' }] }] })
})

test('code fences and tables are skipped; flat prose becomes sentence nodes', () => {
  const tree = markdownToMindTree(['```', '# 不是标题', '```', '| a | b |', '第一句。第二句！'].join('\n'), '题')
  assert.deepEqual(tree, { title: '题', children: [{ title: '第一句。' }, { title: '第二句！' }] })
})

test('labels keep their wording but lose markdown syntax', () => {
  assert.equal(sanitizeMindLabel('**[链接](http://x)** (括号) a:b'), '链接 (括号) a:b')
  assert.equal(sanitizeMindLabel('x'.repeat(200)).length, 121)
})

test('empty input yields no map', () => {
  assert.equal(markdownToMindTree('   \n\n'), null)
})

test('download name comes from the title and drops invalid file characters', () => {
  assert.equal(mindmapFileName('内控/规范: 结构?'), '内控规范 结构脑图')
  assert.equal(mindmapFileName('  '), '脑图')
})
