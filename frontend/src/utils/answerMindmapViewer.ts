import { MessagePlugin } from 'tdesign-vue-next'
import { generateSessionMindmap } from '@/api/chat'
import { markdownToMindTree, mindmapFileName, type MindNode } from './answerMindmap'
import { stripCitationTagsForMarkdown } from './citationMarkdown'
import { renderMindTreeSvg } from './mindmapSvg'
import { openMermaidFullscreen } from './mermaidViewer.ts'

// Builds a mind map of an answer and opens it in the fullscreen viewer.
// The session model summarises the answer; if that fails or returns nothing
// usable, the map is built from the answer's own headings and list items.
// Shows a progress message while the model works, and says so when the local
// fallback was used, so a slow or failed model is never silent.
export async function openAnswerMindmap(opts: {
  sessionId?: string
  content: string
  title?: string
  t: (key: string) => string
}): Promise<void> {
  const content = stripCitationTagsForMarkdown(opts.content).text
  const progress = await MessagePlugin.loading(opts.t('chat.mindmap.generating'), 0)
  let tree: MindNode | null = null
  let source: 'model' | 'local' = 'model'
  try {
    if (opts.sessionId) {
      try {
        const res = await generateSessionMindmap(opts.sessionId, { content, title: opts.title })
        if (res?.data && typeof res.data.title === 'string') tree = res.data
      } catch {
        tree = null
      }
    }
    if (!tree) {
      source = 'local'
      tree = markdownToMindTree(content, opts.title || undefined)
    }
  } finally {
    progress.close()
  }

  if (!tree) {
    MessagePlugin.error(opts.t('chat.mindmap.renderFailed'))
    return
  }
  if (source === 'local') MessagePlugin.info(opts.t('chat.mindmap.localFallback'))
  openMermaidFullscreen(renderMindTreeSvg(tree), `${mindmapFileName(tree.title)}.png`)
}
