<template>
  <!-- T2 引用溯源：气泡下方引用行（FAQ 卡 / 文档聚合卡）。
       keyword_fallback 渲染为灰色折叠"可能相关"，不与精确引用并列（设计 §6.3）。 -->
  <div
    v-if="visible"
    class="citation-list"
  >
    <div class="citation-title">
      {{ $t('citation.title') }}
    </div>
    <div class="citation-cards">
      <template
        v-for="(c, i) in citations"
        :key="i"
      >
        <button
          v-if="c.type === 'faq' && c.faq"
          class="citation-card"
          :class="{ 'is-fallback': c.faq.match_type === 'keyword_fallback' }"
          type="button"
          @click="$emit('open', c)"
        >
          <span class="citation-idx">{{ i + 1 }}</span>
          <span class="citation-q">{{ c.faq.question }}</span>
          <span class="citation-badge">{{ matchLabel(c.faq.match_type) }}</span>
        </button>
        <button
          v-else-if="c.type === 'chunk'"
          class="citation-card"
          type="button"
          @click="$emit('open', c)"
        >
          <span class="citation-idx">{{ i + 1 }}</span>
          <span class="citation-q">{{ chunkTitle(c) }}</span>
          <span class="citation-badge">{{ $t('citation.doc') }}</span>
        </button>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import i18n from '@/i18n'

const props = defineProps({
  citations: { type: Array, default: () => [] }
})
defineEmits(['open'])

// 空 / 全空数组不渲染（后端 omitempty 已保证，双保险）
const visible = computed(() => (props.citations || []).length > 0)

const matchLabel = (mt) => ({
  exact: i18n.global.t('citation.exact'),
  token: i18n.global.t('citation.semantic'),
  semantic: i18n.global.t('citation.semantic'),
  keyword_fallback: i18n.global.t('citation.fallback')
}[mt] || mt)

const chunkTitle = (c) => {
  const agg = (c.doc_aggs || [])[0]
  if (agg?.doc_title) return agg.doc_title
  const ch = (c.chunks || [])[0]
  return ch?.doc_title || i18n.global.t('citation.kbSnippet')
}
</script>

<style scoped lang="scss">
.citation-list {
  margin-top: 6px;
  padding: 0 4px;
}

.citation-title {
  font-size: 11px;
  color: var(--el-text-color-secondary, #909399);
  margin-bottom: 4px;
}

.citation-cards {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.citation-card {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 3px 8px;
  border: 1px solid var(--el-border-color-light, #e4e7ed);
  border-radius: 8px;
  background: var(--el-fill-color-lighter, #f5f7fa);
  cursor: pointer;
  font-size: 12px;
  color: var(--el-text-color-primary, #303133);

  &:hover {
    border-color: var(--el-color-primary-light-5, #a0cfff);
  }

  &.is-fallback {
    opacity: 0.65;
    border-style: dashed;
  }
}

.citation-idx {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--el-color-primary, #409eff);
  color: #fff;
  font-size: 10px;
}

.citation-q {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 180px;
}

.citation-badge {
  font-size: 10px;
  color: var(--el-text-color-secondary, #909399);
  border: 1px solid currentColor;
  border-radius: 4px;
  padding: 0 4px;
}
</style>
