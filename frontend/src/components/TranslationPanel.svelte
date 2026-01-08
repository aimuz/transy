<script lang="ts">
  import { onMount } from 'svelte'
  import LanguageSelector from './LanguageSelector.svelte'
  import { translateWithLLM, detectLanguage, takeScreenshotAndOCR } from '../services/wails'
  import { LANGUAGE_NAME_MAP, LANGUAGE_CODE_MAP, type Usage } from '../types'

  type Props = {
    defaultLanguages: Record<string, string>
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
    onUsageChange?: (usage: Usage | null) => void
  }

  let { defaultLanguages, onToast, onUsageChange }: Props = $props()

  // State
  let sourceText = $state('')
  let targetText = $state('')
  let sourceLang = $state('auto')
  let targetLang = $state('auto')
  let detectedLangName = $state('')
  let detectedTargetName = $state('')
  let isTranslating = $state(false)
  let isOCR = $state(false)
  let debounceTimer: ReturnType<typeof setTimeout> | null = null

  // Derived source language display
  let sourceLangDisplay = $derived(
    sourceLang === 'auto' && detectedLangName ? `自动（${detectedLangName}）` : undefined
  )

  // Derived target language display
  let targetLangDisplay = $derived(
    targetLang === 'auto' && detectedTargetName ? `自动（${detectedTargetName}）` : undefined
  )

  // Handle source text change with debounce
  function handleSourceInput() {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
    }

    if (!sourceText.trim()) {
      targetText = ''
      return
    }

    debounceTimer = setTimeout(async () => {
      await detectAndTranslate()
    }, 500)
  }

  // Detect language and translate
  async function detectAndTranslate() {
    if (!sourceText.trim()) return

    try {
      // Detect language
      const detection = await detectLanguage(sourceText)

      if (detection.name) {
        detectedLangName = detection.name

        // Update detected target language name
        if (detection.defaultTarget && LANGUAGE_CODE_MAP[detection.defaultTarget]) {
          detectedTargetName = LANGUAGE_CODE_MAP[detection.defaultTarget]
        }

        // Smart switch: if detected source matches current target, switch target
        if (sourceLang === 'auto' && detection.code === targetLang) {
          const newTarget = detection.defaultTarget || 'en'
          if (newTarget !== targetLang) {
            targetLang = newTarget
            // clear detected target name since we switched to specific lang
            if (targetLang !== 'auto') {
              detectedTargetName = ''
            }
          }
        }
      }

      // Translate
      await translate()
    } catch (error) {
      console.error('Detection/translation error:', error)
      onToast(String(error), 'error')
    }
  }

  // Translate text
  async function translate() {
    if (!sourceText.trim()) {
      targetText = ''
      return
    }

    isTranslating = true

    try {
      // Resolve actual source language
      let actualSourceLang = sourceLang
      if (sourceLang === 'auto' && detectedLangName) {
        actualSourceLang = LANGUAGE_NAME_MAP[detectedLangName] || 'en'
      }

      // Resolve actual target language
      let actualTargetLang = targetLang
      if (targetLang === 'auto') {
        actualTargetLang = defaultLanguages[actualSourceLang] || 'en'
      }

      const result = await translateWithLLM({
        text: sourceText,
        sourceLang: actualSourceLang,
        targetLang: actualTargetLang,
      })

      targetText = result.text
      onUsageChange?.(result.usage)
    } catch (error) {
      console.error('Translation error:', error)
      onToast(String(error), 'error')
    } finally {
      isTranslating = false
    }
  }

  // Handle language change
  function handleSourceLangChange(lang: string) {
    sourceLang = lang
    if (lang !== 'auto') {
      detectedLangName = ''

      // Smart switch target language
      if (targetLang === lang || targetLang === 'auto') {
        const newTarget = defaultLanguages[lang] || 'en'
        if (newTarget !== lang) {
          targetLang = newTarget
          detectedTargetName = ''
        }
      }
    }
    if (sourceText.trim()) {
      translate()
    }
  }

  function handleTargetLangChange(lang: string) {
    targetLang = lang
    if (lang !== 'auto') {
      detectedTargetName = ''
    }
    if (sourceText.trim()) {
      translate()
    }
  }

  // Swap languages
  function handleSwap() {
    if (sourceLang === 'auto' || targetLang === 'auto') return

    const temp = sourceLang
    sourceLang = targetLang
    targetLang = temp
    detectedLangName = ''
    detectedTargetName = ''

    if (sourceText.trim()) {
      translate()
    }
  }

  // Handle OCR screenshot
  async function handleOCR() {
    if (isOCR) return
    isOCR = true
    try {
      await takeScreenshotAndOCR()
      // Result handled by set-clipboard-text event
    } catch (error) {
      console.error('OCR error:', error)
    } finally {
      isOCR = false
    }
  }

  // Clear source text
  function clearSource() {
    sourceText = ''
    targetText = ''
  }

  // Copy target text
  async function copyTarget() {
    if (!targetText) {
      onToast('没有可复制的译文', 'info')
      return
    }

    try {
      await navigator.clipboard.writeText(targetText)
      onToast('已复制到剪贴板', 'success')
    } catch (error) {
      console.error('Copy failed:', error)
      onToast('复制失败', 'error')
    }
  }

  // Listen for clipboard events from backend
  onMount(() => {
    const handleClipboardText = (e: CustomEvent<string>) => {
      sourceText = e.detail
      detectAndTranslate()
    }

    window.addEventListener('clipboard-text', handleClipboardText as EventListener)

    return () => {
      window.removeEventListener('clipboard-text', handleClipboardText as EventListener)
    }
  })
</script>

<div class="translation-panel">
  <header class="header">
    <LanguageSelector
      value={sourceLang}
      displayValue={sourceLangDisplay}
      onChange={handleSourceLangChange}
    />
    <button class="swap-btn" onclick={handleSwap} title="交换语言">
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M8 3L4 7L8 11"></path>
        <path d="M4 7H20"></path>
        <path d="M16 21L20 17L16 13"></path>
        <path d="M20 17H4"></path>
      </svg>
    </button>
    <LanguageSelector
      value={targetLang}
      displayValue={targetLangDisplay}
      onChange={handleTargetLangChange}
    />
  </header>

  <div class="translation-area">
    <div class="text-area">
      <div class="text-container">
        <textarea
          class="source-text-area"
          placeholder="请输入要翻译的文本"
          bind:value={sourceText}
          oninput={handleSourceInput}
        ></textarea>

        <div class="toolbar">
          <button
            class="icon-btn tool-btn"
            onclick={handleOCR}
            title="截图 OCR (Cmd+Shift+O)"
            disabled={isOCR}
          >
            {#if isOCR}
              <div class="spinner-sm"></div>
            {:else}
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path
                  d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"
                ></path>
                <circle cx="12" cy="13" r="4"></circle>
              </svg>
            {/if}
          </button>
          {#if sourceText}
            <button class="icon-btn tool-btn" onclick={clearSource} title="清空源文本">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <line x1="18" y1="6" x2="6" y2="18"></line>
                <line x1="6" y1="6" x2="18" y2="18"></line>
              </svg>
            </button>
          {/if}
        </div>
      </div>
    </div>
    <div class="text-area">
      <div class="text-container">
        <textarea class="target-text-area" placeholder="翻译结果" readonly value={targetText}
        ></textarea>

        <div class="toolbar">
          <button class="icon-btn tool-btn" onclick={copyTarget} title="复制译文">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
            </svg>
          </button>
        </div>

        {#if isTranslating}
          <div class="loading-indicator">
            <div class="loading-spinner"></div>
            <span>翻译中...</span>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .translation-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .header {
    margin-bottom: 16px;
    display: flex;
    align-items: center;
    gap: 12px;
    background: var(--color-surface);
    padding: 8px;
    border-radius: var(--radius-lg);
  }

  .swap-btn {
    background: var(--color-surface);
    border: none;
    padding: 8px;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    border-radius: var(--radius-md);
    color: var(--color-text-secondary);
    transition: all var(--transition-fast);
  }

  .swap-btn:hover {
    background: var(--color-surface);
    filter: brightness(0.95);
    color: var(--color-text);
  }

  .translation-area {
    flex: 1;
    display: flex;
    gap: 16px;
    margin-bottom: 16px;
    min-height: 0;
  }

  .text-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    background: var(--color-surface);
    border-radius: var(--radius-lg);
    padding: 12px;
    position: relative;
  }

  .text-container {
    position: relative;
    display: flex;
    flex: 1;
  }

  .source-text-area,
  .target-text-area {
    flex: 1;
    padding: 4px;
    padding-right: 32px;
  }

  .toolbar {
    position: absolute;
    top: 0;
    right: 0;
    display: flex;
    gap: 4px;
    background-color: var(--color-toolbar-bg);
    backdrop-filter: blur(2px);
    border-bottom-left-radius: 6px;
  }

  .tool-btn {
    padding: 4px;
    color: var(--color-text-tertiary);
    transition: all var(--transition-fast);
  }

  .tool-btn:hover:not(:disabled) {
    color: var(--color-primary);
    background-color: var(--color-surface);
    border-radius: 4px;
  }

  .tool-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .spinner-sm {
    width: 14px;
    height: 14px;
    border: 2px solid var(--color-border);
    border-left-color: var(--color-primary);
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .loading-indicator {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background-color: var(--color-toolbar-bg);
    border-radius: var(--radius-lg);
    padding: 8px 16px;
    display: flex;
    align-items: center;
    gap: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
    z-index: 10;
  }
</style>
