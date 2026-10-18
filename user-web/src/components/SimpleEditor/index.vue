<template>
  <div class="simple-editor">
    <div class="editor-toolbar">
      <button
        :class="{ active: isCommandActive('bold') }"
        :title="$t('加粗')"
        @click="execCommand('bold')"
      >
        <strong>B</strong>
      </button>
      <button
        :class="{ active: isCommandActive('italic') }"
        :title="$t('斜体')"
        @click="execCommand('italic')"
      >
        <em>I</em>
      </button>
      <button
        :class="{ active: isCommandActive('underline') }"
        :title="$t('下划线')"
        @click="execCommand('underline')"
      >
        <u>U</u>
      </button>
      <span class="separator" />
      <button
        :class="{ active: isCommandActive('justifyLeft') }"
        :title="$t('左对齐')"
        @click="execCommand('justifyLeft')"
      >
        ≡
      </button>
      <button
        :class="{ active: isCommandActive('justifyCenter') }"
        :title="$t('居中')"
        @click="execCommand('justifyCenter')"
      >
        ≡
      </button>
      <button
        :class="{ active: isCommandActive('justifyRight') }"
        :title="$t('右对齐')"
        @click="execCommand('justifyRight')"
      >
        ≡
      </button>
      <span class="separator" />
      <button
        :title="$t('插入链接')"
        @click="insertLink"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.7"
          stroke-linecap="round"
          stroke-linejoin="round"
        ><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" /><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" /></svg>
      </button>
      <button
        :title="$t('插入图片')"
        @click="insertImage"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.7"
          stroke-linecap="round"
          stroke-linejoin="round"
        ><rect
          x="3"
          y="3"
          width="18"
          height="18"
          rx="2"
          ry="2"
        /><circle
          cx="8.5"
          cy="8.5"
          r="1.5"
        /><polyline points="21 15 16 10 5 21" /></svg>
      </button>
      <span class="separator" />
      <button
        :title="$t('撤销')"
        @click="execCommand('undo')"
      >
        ↶
      </button>
      <button
        :title="$t('重做')"
        @click="execCommand('redo')"
      >
        ↷
      </button>
    </div>
    <div
      ref="editorRef"
      class="editor-content"
      contenteditable="true"
      role="textbox"
      tabindex="0"
      aria-label="富文本编辑区"
      @input="handleInput"
      @keydown="handleKeydown"
      @paste="handlePaste"
    />
  </div>
</template>

<script setup>
import i18n from '@/i18n'
import DOMPurify from 'dompurify'

import { ref, onMounted, watch } from 'vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  disabled: {
    type: Boolean,
    default: false
  }
})

const emits = defineEmits(['update:modelValue', 'input'])

const editorRef = ref(null)

onMounted(() => {
  if (props.disabled) {
    editorRef.value.contentEditable = false
  }
  if (props.modelValue && editorRef.value) {
    editorRef.value.innerHTML = DOMPurify.sanitize(props.modelValue, { USE_PROFILES: { html: true } })
  }
});

watch(() => props.modelValue, (newVal) => {
  if (editorRef.value && editorRef.value.innerHTML !== newVal) {
    editorRef.value.innerHTML = DOMPurify.sanitize(newVal || '', { USE_PROFILES: { html: true } });
  }
});

const handleInput = () => {
  const html = editorRef.value.innerHTML
  emits('update:modelValue', html)
  emits('input', html)
};

const handleKeydown = (e) => {
  if (e.key === 'Tab') {
    e.preventDefault()
    document.execCommand('insertHTML', false, '&nbsp;&nbsp;&nbsp;&nbsp;')
  }
};

const handlePaste = (e) => {
  e.preventDefault()
  const text = e.clipboardData.getData('text/plain')
  document.execCommand('insertText', false, text)
};

const execCommand = (command, value = null) => {
  document.execCommand(command, false, value)
  handleInput()
};

const isCommandActive = (command) => {
  return document.queryCommandState(command)
};

const insertLink = () => {
  const url = prompt(i18n.global.t('请输入链接地址:'))
  if (url) {
    execCommand('createLink', url)
  }
};

const insertImage = () => {
  const url = prompt(i18n.global.t('请输入图片地址:'))
  if (url) {
    execCommand('insertImage', url)
  }
};
</script>

<style scoped>
.simple-editor {
  border: 1px solid #ccc;
  border-radius: 4px;
  overflow: hidden;
}

.editor-toolbar {
  display: flex;
  align-items: center;
  padding: 8px;
  background-color: #f5f5f5;
  border-bottom: 1px solid #ccc;
  flex-wrap: wrap;
}

.editor-toolbar button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  margin-right: 5px;
  background-color: #fff;
  color: #475569;
  border: 1px solid #ccc;
  border-radius: 3px;
  cursor: pointer;
  font-size: 14px;
}
.editor-toolbar button svg { width: 16px; height: 16px; }

.editor-toolbar button:hover {
  background-color: #e6e6e6;
}

.editor-toolbar button.active {
  background-color: #d1e7ff;
  border-color: #4F46E5;
}

.separator {
  display: inline-block;
  width: 1px;
  height: 20px;
  margin: 0 5px;
  background-color: #ccc;
}

.editor-content {
  min-height: 300px;
  padding: 12px;
  overflow-y: auto;
  background-color: #fff;
}

.editor-content:focus {
  outline: none;
}

.editor-content img {
  max-width: 100%;
  height: auto;
}
</style>