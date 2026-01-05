<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import LanguageSelector from './LanguageSelector.svelte'
  import {
    startLiveTranslation,
    stopLiveTranslation,
    getLiveStatus,
    getSTTProviders,
    setSTTProvider,
    setupSTTProvider,
  } from '../services/wails'
  import type { LiveTranscript, LiveStatus, STTProviderInfo } from '../types'

  let { onToast = (msg: string, type: 'info' | 'error' | 'success' = 'info') => {} } = $props()

  // State
  let isActive = $state(false)
  let sourceLang = $state('auto')
  let targetLang = $state('zh')
  let transcripts = $state<LiveTranscript[]>([])
  let duration = $state(0)
  let sttProviders = $state<STTProviderInfo[]>([])
  let currentSTT = $state('')
  let isLoading = $state(false)
  let setupProgress = $state(-1)

  // Timer for duration update
  let durationInterval: number | null = null

  async function loadProviders() {
    try {
      sttProviders = await getSTTProviders()
      if (sttProviders.length > 0) {
        currentSTT = sttProviders[0].name
      }
    } catch (error) {
      console.error('Failed to load STT providers:', error)
    }
  }

  async function handleStart() {
    if (isActive) return

    // Check if provider is ready
    const provider = sttProviders.find((p) => p.name === currentSTT)
    if (provider && !provider.isReady) {
      onToast('STT 模型未就绪，请先下载模型', 'error')
      return
    }

    isLoading = true
    try {
      await startLiveTranslation(sourceLang, targetLang)
      isActive = true
      transcripts = []
      duration = 0
      durationInterval = setInterval(() => {
        duration++
      }, 1000) as unknown as number
      onToast('实时翻译已启动', 'success')
    } catch (error) {
      onToast(String(error), 'error')
    } finally {
      isLoading = false
    }
  }

  async function handleStop() {
    if (!isActive) return

    isLoading = true
    try {
      await stopLiveTranslation()
      isActive = false
      if (durationInterval) {
        clearInterval(durationInterval)
        durationInterval = null
      }
      onToast('实时翻译已停止', 'info')
    } catch (error) {
      onToast(String(error), 'error')
    } finally {
      isLoading = false
    }
  }

  async function handleSTTChange(event: Event) {
    const select = event.target as HTMLSelectElement
    currentSTT = select.value
    try {
      await setSTTProvider(currentSTT)
    } catch (error) {
      onToast(String(error), 'error')
    }
  }

  async function handleSetupSTT() {
    const provider = sttProviders.find((p) => p.name === currentSTT)
    if (!provider || provider.isReady) return

    try {
      setupProgress = 0
      await setupSTTProvider(currentSTT)
    } catch (error) {
      onToast(String(error), 'error')
      setupProgress = -1
    }
  }

  function formatDuration(seconds: number): string {
    const h = Math.floor(seconds / 3600)
    const m = Math.floor((seconds % 3600) / 60)
    const s = seconds % 60
    if (h > 0) {
      return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
    }
    return `${m}:${s.toString().padStart(2, '0')}`
  }

  onMount(() => {
    loadProviders()

    // Listen for live transcript events
    Events.On('live-transcript', (event: { data: LiveTranscript }) => {
      const transcript = event.data
      // Update or add transcript
      const existingIndex = transcripts.findIndex((t) => t.id === transcript.id)
      if (existingIndex >= 0) {
        transcripts[existingIndex] = transcript
      } else {
        transcripts = [...transcripts, transcript]
      }
      // Keep only last 50 transcripts
      if (transcripts.length > 50) {
        transcripts = transcripts.slice(-50)
      }
    })

    // Listen for STT setup events
    Events.On('stt-setup-progress', (event: { data: { provider: string; progress: number } }) => {
      if (event.data.provider === currentSTT) {
        setupProgress = event.data.progress
      }
    })

    Events.On('stt-setup-complete', (event: { data: string }) => {
      if (event.data === currentSTT) {
        setupProgress = 100
        onToast('模型下载完成', 'success')
        loadProviders() // Refresh provider status
      }
    })

    Events.On('stt-setup-error', (event: { data: { provider: string; error: string } }) => {
      if (event.data.provider === currentSTT) {
        setupProgress = -1
        onToast(`模型下载失败: ${event.data.error}`, 'error')
      }
    })
  })

  onDestroy(() => {
    if (durationInterval) {
      clearInterval(durationInterval)
    }
  })
</script>

<div class="live-translation">
  <div class="header">
    <h2>🎙️ 实时翻译</h2>
    {#if isActive}
      <span class="status-badge active">
        <span class="pulse"></span>
        录音中 · {formatDuration(duration)}
      </span>
    {/if}
  </div>

  <div class="controls">
    <div class="language-row">
      <div class="lang-selector">
        <label>源语言</label>
        <LanguageSelector value={sourceLang} onChange={(v) => (sourceLang = v)} />
      </div>
      <span class="arrow">→</span>
      <div class="lang-selector">
        <label>目标语言</label>
        <LanguageSelector value={targetLang} onChange={(v) => (targetLang = v)} />
      </div>
    </div>

    <div class="stt-row">
      <div class="stt-selector">
        <label>语音识别</label>
        <select value={currentSTT} onchange={handleSTTChange} disabled={isActive}>
          {#each sttProviders as provider}
            <option value={provider.name}>
              {provider.displayName}
              {#if !provider.isReady}(未就绪){/if}
            </option>
          {/each}
        </select>
      </div>

      {#if sttProviders.find((p) => p.name === currentSTT)?.requiresSetup && !sttProviders.find((p) => p.name === currentSTT)?.isReady}
        <button class="setup-btn" onclick={handleSetupSTT} disabled={setupProgress >= 0}>
          {#if setupProgress >= 0 && setupProgress < 100}
            下载中 {setupProgress}%
          {:else}
            下载模型
          {/if}
        </button>
      {/if}
    </div>

    <div class="action-row">
      {#if isActive}
        <button class="stop-btn" onclick={handleStop} disabled={isLoading}>
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="currentColor"
          >
            <rect x="6" y="6" width="12" height="12" rx="2" />
          </svg>
          停止翻译
        </button>
      {:else}
        <button class="start-btn" onclick={handleStart} disabled={isLoading}>
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3z" />
            <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
            <line x1="12" y1="19" x2="12" y2="22" />
          </svg>
          开始翻译
        </button>
      {/if}
    </div>
  </div>

  <div class="transcripts">
    {#if transcripts.length === 0}
      <div class="empty-state">
        {#if isActive}
          <p>正在监听音频...</p>
        {:else}
          <p>点击"开始翻译"捕获系统音频并实时翻译</p>
        {/if}
      </div>
    {:else}
      {#each transcripts.toReversed() as transcript (transcript.id)}
        <div class="transcript-item" class:pending={!transcript.isFinal}>
          <div class="original">{transcript.text}</div>
          <div class="translated">{transcript.translated}</div>
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .live-translation {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 16px;
    gap: 16px;
  }

  .header {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .header h2 {
    margin: 0;
    font-size: 18px;
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .status-badge {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 20px;
    font-size: 12px;
    font-weight: 500;
  }

  .status-badge.active {
    background: rgba(239, 68, 68, 0.1);
    color: #ef4444;
  }

  .pulse {
    width: 8px;
    height: 8px;
    background: #ef4444;
    border-radius: 50%;
    animation: pulse 1.5s infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.5;
      transform: scale(1.2);
    }
  }

  /* Toolbar Style Controls */
  .controls {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 16px;
    background: var(--color-surface); /* Subtle background */
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border);
  }

  @supports (backdrop-filter: blur(20px)) {
    .controls {
      background: var(--color-surface-translucent);
      backdrop-filter: blur(20px);
      -webkit-backdrop-filter: blur(20px);
    }
  }

  .language-row,
  .stt-row {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .arrow {
    color: var(--color-text-tertiary);
    font-size: 14px;
    margin-top: 18px; /* Visual alignment with inputs */
  }

  .lang-selector,
  .stt-selector {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--color-text-secondary);
    letter-spacing: 0.03em;
  }

  .stt-selector select {
    appearance: none;
    -webkit-appearance: none;
    padding: 10px 12px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-background);
    color: var(--color-text);
    font-size: 14px;
    width: 100%;
    transition: all var(--transition-fast);
    background-image: url("data:image/svg+xml;charset=UTF-8,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='currentColor' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3e%3cpolyline points='6 9 12 15 18 9'%3e%3c/polyline%3e%3c/svg%3e");
    background-repeat: no-repeat;
    background-position: right 10px center;
    background-size: 14px;
    padding-right: 32px;
  }

  .stt-selector select:hover {
    border-color: var(--color-text-tertiary);
  }

  .stt-selector select:focus {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px rgba(0, 122, 255, 0.15);
    outline: none;
  }

  .setup-btn {
    padding: 8px 16px;
    background: var(--color-primary);
    color: white;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--transition-fast);
    margin-top: 18px; /* Align with inputs */
    white-space: nowrap;
  }

  .setup-btn:disabled {
    opacity: 0.7;
    cursor: not-allowed;
  }

  .action-row {
    margin-top: 4px;
  }

  .start-btn,
  .stop-btn {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 12px;
    border: none;
    border-radius: var(--radius-lg);
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;
    transition: all var(--transition-normal);
  }

  .start-btn {
    background: var(--color-active, #007aff);
    color: white;
    box-shadow: 0 2px 4px rgba(0, 122, 255, 0.2);
  }

  .start-btn:hover:not(:disabled) {
    background: var(--color-primary-hover);
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(0, 122, 255, 0.3);
  }

  .stop-btn {
    background: var(--color-surface);
    color: var(--color-danger);
    border: 1px solid rgba(255, 59, 48, 0.2);
  }

  .stop-btn:hover:not(:disabled) {
    background: rgba(255, 59, 48, 0.05);
    border-color: var(--color-danger);
  }

  .start-btn:disabled,
  .stop-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    transform: none !important;
  }

  /* Transcripts Feed */
  .transcripts {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 12px 4px;
    scroll-behavior: smooth;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--color-text-tertiary);
    gap: 12px;
    opacity: 0.6;
  }

  .transcript-item {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px 16px;
    background: var(--color-background);
    border-radius: var(--radius-lg) var(--radius-lg) var(--radius-lg) 2px;
    box-shadow: var(--shadow-sm);
    border: 1px solid var(--color-border);
    margin-right: 12px; /* Chat bubble look */
    transition: all 0.3s ease;
  }

  .transcript-item:hover {
    box-shadow: var(--shadow-md);
    transform: translateY(-1px);
  }

  .transcript-item.pending {
    opacity: 0.8;
    border: 1px dashed var(--color-border);
    box-shadow: none;
    background: transparent;
  }

  .original {
    font-size: 13px;
    color: var(--color-text-secondary);
    line-height: 1.4;
  }

  .translated {
    font-size: 16px;
    color: var(--color-text);
    font-weight: 500;
    line-height: 1.5;
  }
</style>
