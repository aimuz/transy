<script lang="ts">
  import { onMount } from 'svelte'
  import { Events, Browser } from '@wailsio/runtime'
  import TranslationPanel from './components/TranslationPanel.svelte'
  import SettingsModal from './components/SettingsModal.svelte'
  import Toast from './components/Toast.svelte'
  import {
    getProviders,
    getDefaultLanguages,
    getAccessibilityPermission,
    getVersion,
  } from './services/wails'
  import type { Provider, Usage } from './types'

  // Global state using Svelte 5 runes
  let providers = $state<Provider[]>([])
  let defaultLanguages = $state<Record<string, string>>({})
  let showSettings = $state(false)
  let toastMessage = $state('')
  let toastType = $state<'info' | 'error' | 'success'>('info')
  let toastVisible = $state(false)
  let accessibilityGranted = $state(true) // 默认假设已授权，避免闪烁
  let lastUsage = $state<Usage | null>(null)
  let version = $state('v1.0')

  // Toast helper
  function showToast(message: string, type: 'info' | 'error' | 'success' = 'info') {
    toastMessage = message
    toastType = type
    toastVisible = true
    setTimeout(() => {
      toastVisible = false
    }, 3000)
  }

  // Load initial data
  async function loadData() {
    try {
      providers = await getProviders()
      defaultLanguages = await getDefaultLanguages()
      version = await getVersion()

      // Check accessibility permission on load
      accessibilityGranted = await getAccessibilityPermission()
    } catch (error) {
      console.error('Failed to load data:', error)
      showToast(String(error), 'error')
    }
  }

  // Reload providers
  async function reloadProviders() {
    providers = await getProviders()
  }

  // Reload default languages
  async function reloadDefaultLanguages() {
    defaultLanguages = await getDefaultLanguages()
  }

  // Open system accessibility settings
  function openAccessibilitySettings() {
    // 使用 Wails v3 Browser API 打开系统设置
    Browser.OpenURL('x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility')
  }

  onMount(() => {
    loadData()

    // Listen for clipboard events from backend (Wails v3 Events API)
    Events.On('set-clipboard-text', (event: { data: unknown }) => {
      // Dispatch custom event that TranslationPanel can listen to
      window.dispatchEvent(new CustomEvent('clipboard-text', { detail: event.data as string }))
    })

    // Listen for accessibility permission status
    Events.On('accessibility-permission', (event: { data: unknown }) => {
      accessibilityGranted = event.data as boolean
      if (event.data) {
        showToast('辅助功能权限已授予，快捷键已启用', 'success')
      }
    })
  })
</script>

<div class="app">
  <div class="drag-region" data-wails-drag></div>

  {#if !accessibilityGranted}
    <div class="permission-banner">
      <span class="permission-icon">⚠️</span>
      <span>需要辅助功能权限才能使用双击 Cmd+C 快捷键</span>
      <button class="permission-btn" onclick={openAccessibilitySettings}>打开系统设置</button>
    </div>
  {/if}

  <main class="container">
    <TranslationPanel
      {defaultLanguages}
      onToast={showToast}
      onUsageChange={(u) => (lastUsage = u)}
    />
  </main>

  <footer class="footer">
    <div class="footer-left">
      <span class="version">Transy {version}</span>
      {#if lastUsage}
        <span class="usage-info">
          {#if lastUsage.cacheHit}
            <span class="cache-badge">缓存</span>
          {/if}
          <span class="token-count">{lastUsage.totalTokens} tokens</span>
        </span>
      {/if}
    </div>
    <button class="settings-btn" onclick={() => (showSettings = true)}>
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="12" cy="12" r="3"></circle>
        <path
          d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82-.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
        ></path>
      </svg>
    </button>
  </footer>

  {#if showSettings}
    <SettingsModal
      {providers}
      {defaultLanguages}
      onClose={() => (showSettings = false)}
      onProvidersChange={reloadProviders}
      onLanguagesChange={reloadDefaultLanguages}
      onToast={showToast}
    />
  {/if}

  <Toast message={toastMessage} type={toastType} visible={toastVisible} />
</div>

<style>
  .app {
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .container {
    flex: 1;
    display: flex;
    flex-direction: column;
    padding: 0 16px 60px;
    margin: 0 auto;
    width: 100%;
    height: 100%;
  }

  .permission-banner {
    background: var(--color-warning-bg);
    border-bottom: 1px solid var(--color-warning-text);
    padding: 10px 16px;
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 13px;
    color: var(--color-warning-text);
  }

  .permission-icon {
    font-size: 16px;
  }

  .permission-btn {
    margin-left: auto;
    padding: 4px 12px;
    background: var(--color-warning-text);
    color: var(--color-warning-bg);
    border: none;
    border-radius: var(--radius-md);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .permission-btn:hover {
    background: var(--color-warning-text);
    opacity: 0.9;
  }

  .footer {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    padding: 12px 20px;
    background: var(--color-surface);
    border-top: 1px solid var(--color-border);
    display: flex;
    justify-content: space-between;
    align-items: center;
    z-index: 100;
  }

  .footer-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .version {
    color: var(--color-text-secondary);
    font-size: 12px;
  }

  .usage-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    color: var(--color-text-tertiary);
  }

  .cache-badge {
    padding: 1px 6px;
    background: var(--color-success-bg);
    color: white;
    border-radius: 8px;
    font-size: 10px;
  }

  .token-count {
    opacity: 0.8;
  }

  .settings-btn {
    color: var(--color-text-secondary);
    background: none;
    border: none;
    cursor: pointer;
    padding: 8px;
    border-radius: var(--radius-md);
    transition: all var(--transition-fast);
    display: flex;
    align-items: center;
  }

  .settings-btn:hover {
    background: var(--color-surface);
  }
</style>
