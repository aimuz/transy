<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import LanguageSelector from './LanguageSelector.svelte'
  import { startLiveTranslation, stopLiveTranslation } from '../services/wails'
  import type { LiveTranscript, VADState } from '../types'

  let { onToast = (msg: string, type: 'info' | 'error' | 'success' = 'info') => {} } = $props()

  // State
  let isActive = $state(false)
  let sourceLang = $state('auto')
  let targetLang = $state('zh')
  let transcripts = $state<LiveTranscript[]>([])
  let duration = $state(0)
  let isLoading = $state(false)
  let vadState = $state<VADState>('listening')

  // Timer for duration update
  let durationInterval: number | null = null

  async function handleStart() {
    if (isActive) return

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

  function formatDuration(seconds: number): string {
    const m = Math.floor(seconds / 60)
    const s = seconds % 60
    return `${m}:${s.toString().padStart(2, '0')}`
  }

  function formatTime(timestamp: number): string {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  // Event cleanup
  let unsubTranscript: () => void
  let unsubVad: () => void

  onMount(() => {
    // Listen for live transcript events
    unsubTranscript = Events.On('live-transcript', (event: { data: LiveTranscript }) => {
      console.log(event)
      const transcript = event.data
      const existingIndex = transcripts.findIndex((t) => t.id === transcript.id)
      if (existingIndex >= 0) {
        transcripts[existingIndex] = transcript
      } else {
        transcripts = [...transcripts, transcript]
      }
      // Keep only last 100 transcripts
      if (transcripts.length > 100) {
        transcripts = transcripts.slice(-100)
      }
    })

    unsubVad = Events.On('live-vad-update', (event: { data: VADState }) => {
      vadState = event.data
    })
  })

  onDestroy(() => {
    if (durationInterval) {
      clearInterval(durationInterval)
    }
    if (unsubTranscript) unsubTranscript()
    if (unsubVad) unsubVad()
  })
</script>

<div class="live-translation">
  <!-- Compact controls -->
  <div class="controls">
    <div class="language-row">
      <LanguageSelector value={sourceLang} onChange={(v) => (sourceLang = v)} />
      <span class="arrow">→</span>
      <LanguageSelector value={targetLang} onChange={(v) => (targetLang = v)} />
    </div>

    <div class="actions">
      {#if isActive}
        <span class="duration">{formatDuration(duration)}</span>
        <button class="control-btn stop" onclick={handleStop} disabled={isLoading}>
          <div class="btn-animation">
            <span></span><span></span><span></span>
          </div>
          停止
        </button>
      {:else}
        <button class="control-btn start" onclick={handleStart} disabled={isLoading}>
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
          >
            <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3z" />
            <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
            <line x1="12" y1="19" x2="12" y2="22" />
          </svg>
          开始
        </button>
      {/if}
    </div>
  </div>

  <!-- Transcript feed -->
  <div class="transcript-feed">
    {#if transcripts.length === 0}
      <div class="empty-state">
        {#if isActive}
          <div class="listening-animation" class:speaking={vadState === 'speaking'}>
            <span></span><span></span><span></span>
          </div>
          <p>
            {#if vadState === 'speaking'}
              正在说话...
            {:else if vadState === 'processing'}
              正在处理...
            {:else}
              正在监听音频...
            {/if}
          </p>
        {:else}
          <p class="hint">点击开始捕获系统音频并实时翻译</p>
        {/if}
      </div>
    {:else}
      {#each transcripts.toReversed() as transcript (transcript.id)}
        <div class="transcript-card" class:pending={!transcript.isFinal}>
          <div class="transcript-header">
            <span class="timestamp">{formatTime(transcript.timestamp)}</span>
            {#if !transcript.isFinal}
              <span class="pending-badge">处理中</span>
            {/if}
          </div>
          <div class="source-text">
            {#if !transcript.sourceText && !transcript.text && !transcript.isFinal}
              <span class="typing">...</span>
            {:else}
              {transcript.sourceText || transcript.text}
            {/if}
          </div>
          {#if transcript.targetText || transcript.translated}
            <div class="target-text">{transcript.targetText || transcript.translated}</div>
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .live-translation {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    gap: 16px;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .duration {
    font-size: 13px;
    font-weight: 600;
    color: #ef4444;
    font-variant-numeric: tabular-nums;
  }

  /* Controls */
  .controls {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    background: var(--color-surface);
    border-radius: var(--radius-lg);
  }

  .language-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
  }

  .arrow {
    color: var(--color-text-tertiary);
    font-size: 14px;
  }

  .control-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 20px;
    border: none;
    border-radius: var(--radius-md);
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    white-space: nowrap;
  }

  .control-btn.start {
    background: var(--color-primary);
    color: white;
  }

  .control-btn.start:hover:not(:disabled) {
    background: var(--color-primary-hover);
    transform: translateY(-1px);
  }

  .control-btn.stop {
    background: rgba(239, 68, 68, 0.1);
    color: #ef4444;
    /* border: 1px solid rgba(239, 68, 68, 0.2); */
  }

  .control-btn.stop:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.15);
    border-color: #ef4444;
  }

  .control-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  /* Button inline animation */
  .btn-animation {
    display: flex;
    gap: 2px;
    align-items: center;
    height: 16px;
  }

  .btn-animation span {
    width: 3px;
    height: 10px;
    background: #ef4444;
    border-radius: 1px;
    animation: wave 1s ease-in-out infinite;
  }

  .btn-animation span:nth-child(2) {
    animation-delay: 0.1s;
  }
  .btn-animation span:nth-child(3) {
    animation-delay: 0.2s;
  }

  /* Transcript Feed */
  .transcript-feed {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 0 4px 8px 0;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 200px;
    color: var(--color-text-tertiary);
    gap: 16px;
  }

  .hint {
    font-size: 14px;
    opacity: 0.7;
  }

  .listening-animation {
    display: flex;
    gap: 4px;
    align-items: center;
    height: 24px;
  }

  .listening-animation span {
    width: 4px;
    height: 16px;
    background: var(--color-primary);
    border-radius: 2px;
    animation: wave 1s ease-in-out infinite;
  }

  .listening-animation span:nth-child(2) {
    animation-delay: 0.1s;
  }
  .listening-animation span:nth-child(3) {
    animation-delay: 0.2s;
  }

  @keyframes wave {
    0%,
    100% {
      height: 8px;
    }
    50% {
      height: 24px;
    }
  }

  .listening-animation.speaking span {
    background: #10b981; /* Green when speaking */
    animation-duration: 0.5s;
  }

  /* Transcript Cards */
  .transcript-card {
    padding: 12px 16px;
    background: var(--color-surface);
    border-radius: var(--radius-lg);
    transition: all 0.2s ease;
  }

  .transcript-card:hover {
    border-color: var(--color-text-tertiary);
  }

  .transcript-card.pending {
    background: linear-gradient(135deg, rgba(99, 102, 241, 0.05), rgba(139, 92, 246, 0.05));
    border: 1px solid transparent;
    background-clip: padding-box;
    position: relative;
    animation: pulse-glow 2s ease-in-out infinite;
  }

  .transcript-card.pending::before {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    padding: 1px;
    background: linear-gradient(
      135deg,
      rgba(99, 102, 241, 0.4),
      rgba(139, 92, 246, 0.4),
      rgba(6, 182, 212, 0.4)
    );
    -webkit-mask:
      linear-gradient(#fff 0 0) content-box,
      linear-gradient(#fff 0 0);
    mask:
      linear-gradient(#fff 0 0) content-box,
      linear-gradient(#fff 0 0);
    -webkit-mask-composite: xor;
    mask-composite: exclude;
    pointer-events: none;
  }

  @keyframes pulse-glow {
    0%,
    100% {
      box-shadow: 0 0 8px rgba(99, 102, 241, 0.15);
    }
    50% {
      box-shadow: 0 0 16px rgba(139, 92, 246, 0.25);
    }
  }

  .transcript-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
  }

  .timestamp {
    font-size: 11px;
    color: var(--color-text-tertiary);
    font-variant-numeric: tabular-nums;
  }

  .pending-badge {
    font-size: 10px;
    padding: 2px 6px;
    background: var(--color-primary);
    color: white;
    border-radius: 4px;
    opacity: 0.8;
  }

  .source-text {
    font-size: 13px;
    color: var(--color-text-secondary);
    line-height: 1.5;
    margin-bottom: 6px;
  }

  .target-text {
    font-size: 15px;
    color: var(--color-text);
    font-weight: 500;
    line-height: 1.5;
  }
  .typing {
    color: var(--color-text-tertiary);
    font-style: italic;
    animation: blink 1.5s infinite;
  }

  @keyframes blink {
    0%,
    100% {
      opacity: 0.3;
    }
    50% {
      opacity: 1;
    }
  }
</style>
