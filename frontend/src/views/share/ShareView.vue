<template>
  <div class="share-page">
    <div v-if="loading" class="share-page__state">加载中…</div>
    <div v-else-if="error" class="share-page__state">{{ error }}</div>
    <template v-else>
      <header class="share-page__header">
        <h1>{{ share.title || '对话分享' }}</h1>
        <p class="share-page__notice">内容由 AI 生成，请核实重要信息</p>
      </header>
      <main class="share-page__body">
        <section v-for="item in share.items" :key="item.message_id" :class="['share-item', `is-${item.role}`]">
          <div v-if="item.role === 'user'" class="share-item__bubble">{{ item.content }}</div>
          <template v-else>
            <div class="share-item__agent">{{ share.agent_name || 'AI 助手' }}</div>
            <div class="share-item__answer chat-markdown" v-html="render(item.content)"></div>
            <div v-if="item.files?.length" class="share-item__files">
              <a v-for="f in item.files" :key="f.position" class="share-file"
                :href="fileUrl(item, f)" :download="inWeChat ? undefined : f.file_name">
                <t-icon name="file" class="share-file__icon" />
                <span class="share-file__meta">
                  <span class="share-file__name">{{ f.file_name }}</span>
                  <span class="share-file__size">{{ formatSize(f.file_size) }}</span>
                </span>
                <t-icon name="download" />
              </a>
            </div>
            <p v-if="inWeChat && item.files?.length" class="share-item__hint">点击文件可预览；如需保存或转发，打开后点右上角「···」</p>
          </template>
        </section>
      </main>
      <footer class="share-page__footer">分享于 {{ formatTime(share.created_at) }} · KnowPulse</footer>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';
import { createChatMarkdownRenderer, renderChatMarkdown } from '@/utils/chatMarkdownRenderer';
import { sanitizeMarkdownHTML, safeMarkdownToHTML, createSafeImage, isValidImageURL } from '@/utils/security';

const route = useRoute();
const token = String(route.params.token || '');
const loading = ref(true);
const error = ref('');
const share = ref<any>({});

// Generated files are listed as cards, so resource:// references inside the
// answer render nothing instead of an image that needs a login to fetch.
const renderer = createChatMarkdownRenderer({
  imageRenderer: ({ href, title, text }) =>
    href.startsWith('resource://') ? '' : createSafeImage(href, text || '', title || ''),
  isValidImageUrl: (href) => href.startsWith('resource://') || isValidImageURL(href),
});

const render = (content: string) =>
  renderChatMarkdown((content || '').replace(/<think>[\s\S]*?<\/think>/g, ''), {
    renderer,
    escapeMarkdown: safeMarkdownToHTML,
    sanitizeHtml: sanitizeMarkdownHTML,
    streaming: false,
  } as any);

// WeChat's in-app browser cannot save downloads (it shows a 0 B stub), so
// files open inline there and WeChat previews them.
const inWeChat = /MicroMessenger/i.test(navigator.userAgent);
const fileUrl = (item: any, f: any) =>
  `/api/v1/public/shares/${encodeURIComponent(token)}/files/${encodeURIComponent(item.message_id)}/${f.position}` +
  (inWeChat ? '?inline=1' : '');

const formatSize = (n: number) =>
  n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`;
const formatTime = (v: string) => (v ? new Date(v).toLocaleString('zh-CN', { hour12: false }) : '');

onMounted(async () => {
  try {
    const res = await fetch(`/api/v1/public/shares/${encodeURIComponent(token)}`);
    const body = await res.json();
    if (!res.ok || !body.success) throw new Error('分享不存在或已失效');
    share.value = body.data;
    document.title = share.value.title || '对话分享';
  } catch (e: any) {
    error.value = e?.message || '加载失败';
  } finally {
    loading.value = false;
  }
});
</script>

<style lang="less">
@import '@/components/css/chat-markdown.less';
</style>

<style lang="less" scoped>
.share-page {
  min-height: 100vh;
  background: var(--td-bg-color-page);
  color: var(--td-text-color-primary);
  padding: 24px 16px 48px;
  box-sizing: border-box;
}
.share-page__state { text-align: center; padding-top: 30vh; color: var(--td-text-color-secondary); }
.share-page__header, .share-page__body, .share-page__footer { max-width: 820px; margin: 0 auto; }
.share-page__header h1 { font-size: 22px; margin: 8px 0; }
.share-page__notice { color: var(--td-text-color-placeholder); font-size: 13px; margin: 0 0 24px; }
.share-item { margin-bottom: 24px; }
.share-item.is-user { display: flex; justify-content: flex-end; }
.share-item__bubble {
  max-width: 80%; padding: 10px 14px; border-radius: 12px;
  background: var(--td-bg-color-secondarycontainer); white-space: pre-wrap; word-break: break-word;
}
.share-item__agent { font-weight: 600; margin-bottom: 8px; }
.share-item__answer { overflow-x: auto; }
.share-item__files { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px; margin-top: 12px; }
.share-file {
  display: flex; align-items: center; gap: 10px; padding: 12px 14px; border-radius: 10px;
  background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke);
  color: inherit; text-decoration: none;
}
.share-file:hover { border-color: var(--td-brand-color); }
.share-file__icon { font-size: 22px; color: var(--td-brand-color); }
.share-file__meta { display: flex; flex-direction: column; min-width: 0; flex: 1; }
.share-file__name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.share-file__size { font-size: 12px; color: var(--td-text-color-placeholder); }
.share-item__hint { font-size: 12px; color: var(--td-text-color-placeholder); margin: 8px 0 0; }
.share-page__footer { text-align: center; color: var(--td-text-color-placeholder); font-size: 12px; margin-top: 32px; }
</style>
