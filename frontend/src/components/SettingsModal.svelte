<script lang="ts">
  import Modal from './Modal.svelte'
  import CredentialCard from './CredentialCard.svelte'
  import CredentialModal from './CredentialModal.svelte'
  import TranslationProfileCard from './TranslationProfileCard.svelte'
  import TranslationProfileModal from './TranslationProfileModal.svelte'
  import {
    setDefaultLanguage,
    getCredentials,
    getTranslationProfiles,
    getSpeechConfig,
    setSpeechConfig,
  } from '../services/wails'
  import type { APICredential, TranslationProfile, SpeechConfig } from '../types'
  import { TRANSCRIPTION_MODELS } from '../types'

  type Props = {
    // Legacy props kept for compatibility (but not used)
    providers: any[]
    defaultLanguages: Record<string, string>
    onClose: () => void
    onProvidersChange: () => void
    onLanguagesChange: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { defaultLanguages, onClose, onProvidersChange, onLanguagesChange, onToast }: Props = $props()

  // State
  let showAddProfile = $state(false)
  let editingProfile = $state<TranslationProfile | null>(null)
  let defaultZhTarget = $state('en')
  let defaultEnTarget = $state('zh')

  // New architecture state
  let credentials = $state<APICredential[]>([])
  let profiles = $state<TranslationProfile[]>([])
  let speechConfig = $state<SpeechConfig | null>(null)
  let showAddCredential = $state(false)
  let editingCredential = $state<APICredential | null>(null)

  // Load new architecture data
  async function loadNewData() {
    try {
      credentials = await getCredentials()
      profiles = await getTranslationProfiles()
      speechConfig = await getSpeechConfig()
      if (speechConfig && !speechConfig.mode) {
        speechConfig.mode = 'transcription'
      }
    } catch (error) {
      console.error('Failed to load new config data:', error)
    }
  }

  // Initial load
  $effect(() => {
    loadNewData()
  })

  // Sync defaults when props change
  $effect(() => {
    defaultZhTarget = defaultLanguages['zh'] || 'en'
    defaultEnTarget = defaultLanguages['en'] || 'zh'
  })

  // Reset model when mode changes to ensure valid selection
  $effect(() => {
    if (!speechConfig) return
    // Default to gpt-4o-transcribe if no model selected
    if (!speechConfig.model) {
      speechConfig.model = 'gpt-4o-transcribe'
    }
  })

  // Check if credential is in use
  function isCredentialInUse(credId: string): boolean {
    // Check translation profiles
    if (profiles.some((p) => p.credential_id === credId)) return true
    // Check speech config
    if (speechConfig?.credential_id === credId) return true
    return false
  }

  // Only OpenAI credentials for Realtime API
  let speechCredentials = $derived.by(() => {
    return credentials.filter((c) => c.type === 'openai')
  })

  // Handle speech config change
  async function handleSpeechConfigChange() {
    if (!speechConfig) return
    try {
      await setSpeechConfig(speechConfig)
      onToast('语音服务设置已保存', 'success')
    } catch (error) {
      onToast(String(error), 'error')
    }
  }

  // Save default languages
  async function saveDefaultLanguages() {
    try {
      await setDefaultLanguage('zh', defaultZhTarget)
      await setDefaultLanguage('en', defaultEnTarget)
      onLanguagesChange()
      onToast('默认翻译语言设置已保存', 'success')
    } catch (error) {
      onToast(String(error), 'error')
    }
  }

  // Handle credential modal close
  function handleCredentialModalClose() {
    showAddCredential = false
    editingCredential = null
  }

  // Handle credential saved
  function handleCredentialSaved() {
    loadNewData()
    handleCredentialModalClose()
  }

  // Handle profile modal close
  function handleProfileModalClose() {
    showAddProfile = false
    editingProfile = null
  }

  // Handle profile saved
  function handleProfileSaved() {
    onProvidersChange()
    loadNewData()
    handleProfileModalClose()
  }
</script>

<Modal title="设置" {onClose}>
  {#snippet children()}
    <!-- API Credentials Section -->
    <div class="settings-section">
      <h3>🔑 API 凭证</h3>
      <p class="settings-description">管理你的 API 密钥，可在翻译和语音服务中复用</p>
      <div class="credentials-container">
        {#if credentials.length === 0}
          <div class="empty-state">还没有添加任何 API 凭证</div>
        {:else}
          {#each credentials as credential (credential.id)}
            <CredentialCard
              {credential}
              inUse={isCredentialInUse(credential.id)}
              onEdit={() => (editingCredential = credential)}
              onChange={loadNewData}
              {onToast}
            />
          {/each}
        {/if}
      </div>
      <button class="add-btn" onclick={() => (showAddCredential = true)}>+ 添加 API 凭证</button>
    </div>

    <!-- Translation Profiles Section -->
    <div class="settings-section">
      <h3>🌐 翻译配置</h3>
      <p class="settings-description">配置翻译服务使用的模型和参数</p>
      <div class="providers-container">
        {#if profiles.length === 0}
          <div class="empty-state">还没有添加任何翻译配置</div>
        {:else}
          {#each profiles as profile (profile.id)}
            <TranslationProfileCard
              {profile}
              onEdit={() => (editingProfile = profile)}
              onChange={() => {
                onProvidersChange()
                loadNewData()
              }}
              {onToast}
            />
          {/each}
        {/if}
      </div>
      <button class="add-btn" onclick={() => (showAddProfile = true)}>+ 添加翻译配置</button>
    </div>

    <!-- Speech Config Section -->
    <div class="settings-section">
      <h3>🎤 语音转录</h3>
      <p class="settings-description">配置 OpenAI Realtime API 语音转录服务</p>
      <div class="speech-config">
        <label class="checkbox-label">
          <input
            type="checkbox"
            checked={speechConfig?.enabled || false}
            onchange={(e) => {
              if (!speechConfig) {
                speechConfig = { enabled: e.currentTarget.checked, mode: 'realtime' }
              } else {
                speechConfig.enabled = e.currentTarget.checked
              }
            }}
          />
          <span>启用语音转录</span>
        </label>

        {#if speechConfig?.enabled}
          <div class="speech-options">
            <div class="form-group">
              <label for="speech-credential">OpenAI API 凭证</label>
              <select id="speech-credential" bind:value={speechConfig.credential_id}>
                <option value="">选择凭证...</option>
                {#each speechCredentials as cred}
                  <option value={cred.id}>{cred.name}</option>
                {/each}
              </select>
              {#if speechCredentials.length === 0}
                <span class="help-text warning">需要添加 OpenAI API 凭证</span>
              {/if}
            </div>

            <div class="form-group">
              <label for="speech-model">转录模型</label>
              <select id="speech-model" bind:value={speechConfig.model}>
                {#each TRANSCRIPTION_MODELS as model}
                  <option value={model.id}>{model.name}</option>
                {/each}
              </select>
              <span class="help-text">使用 OpenAI Realtime API 进行实时语音转录</span>
            </div>

            <button class="btn btn-primary" onclick={handleSpeechConfigChange}>保存语音设置</button>
          </div>
        {/if}
      </div>
    </div>

    <!-- Default Languages Section -->
    <div class="settings-section">
      <h3>🌍 默认翻译语言</h3>
      <p class="settings-description">当检测到以下语言时，自动设置目标语言</p>
      <div class="default-language-settings">
        <div class="form-group">
          <label for="default-zh-target">检测到中文时，翻译为：</label>
          <select id="default-zh-target" bind:value={defaultZhTarget}>
            <option value="en">英语</option>
            <option value="ja">日语</option>
            <option value="ko">韩语</option>
            <option value="fr">法语</option>
            <option value="de">德语</option>
            <option value="es">西班牙语</option>
            <option value="ru">俄语</option>
            <option value="auto">自动</option>
          </select>
        </div>
        <div class="form-group">
          <label for="default-en-target">检测到英语时，翻译为：</label>
          <select id="default-en-target" bind:value={defaultEnTarget}>
            <option value="zh">中文</option>
            <option value="ja">日语</option>
            <option value="ko">韩语</option>
            <option value="fr">法语</option>
            <option value="de">德语</option>
            <option value="es">西班牙语</option>
            <option value="ru">俄语</option>
            <option value="auto">自动</option>
          </select>
        </div>
        <button class="btn btn-primary" onclick={saveDefaultLanguages}>保存默认语言设置</button>
      </div>
    </div>
  {/snippet}
</Modal>

{#if showAddCredential}
  <CredentialModal onClose={handleCredentialModalClose} onSave={handleCredentialSaved} {onToast} />
{/if}

{#if editingCredential}
  <CredentialModal
    credential={editingCredential}
    onClose={handleCredentialModalClose}
    onSave={handleCredentialSaved}
    {onToast}
  />
{/if}

{#if showAddProfile}
  <TranslationProfileModal
    onClose={handleProfileModalClose}
    onSave={handleProfileSaved}
    {onToast}
  />
{/if}

{#if editingProfile}
  <TranslationProfileModal
    profile={editingProfile}
    onClose={handleProfileModalClose}
    onSave={handleProfileSaved}
    {onToast}
  />
{/if}

<style>
  .settings-section {
    margin-bottom: 24px;
    padding-bottom: 24px;
    border-bottom: 1px solid var(--color-border);
  }

  .settings-section:last-child {
    border-bottom: none;
    margin-bottom: 0;
    padding-bottom: 0;
  }

  .settings-section h3 {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 8px;
    color: var(--color-text);
  }

  .settings-description {
    font-size: 13px;
    color: var(--color-text-secondary);
    margin-bottom: 16px;
  }

  .credentials-container,
  .providers-container {
    margin-bottom: 12px;
  }

  .empty-state {
    text-align: center;
    padding: 24px;
    color: var(--color-text-secondary);
    font-size: 14px;
    background: var(--color-surface);
    border-radius: var(--radius-lg);
    border: 1px dashed var(--color-border);
  }

  .add-btn {
    width: 100%;
    padding: 12px;
    background: var(--color-primary);
    color: #fff;
    border: none;
    border-radius: var(--radius-lg);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .add-btn:hover {
    background: var(--color-primary-hover);
  }

  .speech-config {
    background: var(--color-surface);
    padding: 16px;
    border-radius: var(--radius-lg);
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 14px;
    color: var(--color-text);
  }

  .checkbox-label input[type='checkbox'] {
    width: 18px;
    height: 18px;
    accent-color: var(--color-primary);
  }

  .speech-options {
    margin-top: 16px;
    padding-top: 16px;
    border-top: 1px solid var(--color-border);
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .default-language-settings {
    background: var(--color-surface);
    padding: 16px;
    border-radius: var(--radius-lg);
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-group label {
    font-size: 13px;
    font-weight: 500;
    color: var(--color-text);
  }

  .form-group select {
    padding: 8px 12px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-size: 14px;
    background: var(--color-background);
    color: var(--color-text);
  }

  .form-group select:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  .help-text {
    font-size: 12px;
    color: var(--color-text-secondary);
  }

  .help-text.warning {
    color: var(--color-warning, #f59e0b);
  }

  .btn {
    padding: 10px 20px;
    border: none;
    border-radius: var(--radius-md);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .btn-primary {
    background: var(--color-primary);
    color: #fff;
  }

  .btn-primary:hover {
    background: var(--color-primary-hover);
  }

  .mode-selector {
    display: flex;
    gap: 16px;
    margin-top: 4px;
    padding: 8px;
    background: var(--color-background);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .radio-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--color-text);
    cursor: pointer;
  }

  .radio-label input[type='radio'] {
    width: 16px;
    height: 16px;
    accent-color: var(--color-primary);
  }
</style>
