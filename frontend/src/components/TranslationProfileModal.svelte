<script lang="ts">
  import Modal from './Modal.svelte'
  import {
    addTranslationProfile,
    updateTranslationProfile,
    getCredentials,
  } from '../services/wails'
  import type { TranslationProfile, APICredential } from '../types'

  type Props = {
    profile?: TranslationProfile | null
    onClose: () => void
    onSave: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { profile = null, onClose, onSave, onToast }: Props = $props()

  // Determine if editing
  let isEditing = $derived(!!profile)
  let title = $derived(isEditing ? '编辑翻译配置' : '添加翻译配置')

  // Default settings
  const DEFAULT_SETTINGS = {
    systemPrompt: `You are a professional translator. Translate the provided text into [Target Language] accurately.

Key Requirements:
- Preserve the logic, structure, and distinct style of the original text.
- Use professional and context-appropriate terminology.
- Output strictly the translation alone.
`,
    maxTokens: 5000,
    temperature: 0.3,
  }

  // State
  let name = $state('')
  let credentialId = $state('')
  let model = $state('')
  let systemPrompt = $state(DEFAULT_SETTINGS.systemPrompt)
  let maxTokens = $state(DEFAULT_SETTINGS.maxTokens)
  let temperature = $state(DEFAULT_SETTINGS.temperature)
  let disableThinking = $state(false)
  let showAdvanced = $state(false)
  let saving = $state(false)

  // Data
  let credentials = $state<APICredential[]>([])

  // Load credentials
  $effect(() => {
    getCredentials().then((creds) => {
      credentials = creds
      // If adding new and no credential selected, select first
      if (!isEditing && !credentialId && creds.length > 0) {
        credentialId = creds[0].id
        handleCredentialChange(creds[0])
      }
    })
  })

  // Initialize from profile
  $effect(() => {
    if (profile) {
      name = profile.name
      credentialId = profile.credential_id
      model = profile.model
      systemPrompt = profile.system_prompt || DEFAULT_SETTINGS.systemPrompt
      maxTokens = profile.max_tokens || DEFAULT_SETTINGS.maxTokens
      temperature = profile.temperature || DEFAULT_SETTINGS.temperature
      disableThinking = profile.disable_thinking || false
    }
  })

  // Handle credential selection to auto-fill defaults
  function handleCredentialChange(cred?: APICredential) {
    if (!cred) {
      cred = credentials.find((c) => c.id === credentialId)
    }
    if (!cred) return

    // Only set defaults if model is empty (new profile)
    if (!model) {
      if (cred.type === 'openai') model = 'gpt-4o'
      else if (cred.type === 'claude') model = 'claude-3-5-sonnet-latest'
      else if (cred.type === 'gemini') {
        model = 'gemini-1.5-flash'
        name = name || 'Gemini 翻译'
      }

      if (!name) {
        name = cred.name + ' 翻译'
      }
    }
  }

  // Current credential type for UI logic
  let currentCredentialType = $derived.by(() => {
    const cred = credentials.find((c) => c.id === credentialId)
    return cred?.type || ''
  })

  async function handleSave() {
    if (!name.trim()) {
      onToast('请输入配置名称', 'error')
      return
    }
    if (!credentialId) {
      onToast('请选择 API 凭证', 'error')
      return
    }
    if (!model.trim()) {
      onToast('请输入模型名称', 'error')
      return
    }

    saving = true
    try {
      const data: TranslationProfile = {
        id: profile?.id || '',
        name: name.trim(),
        credential_id: credentialId,
        model: model.trim(),
        system_prompt: systemPrompt,
        max_tokens: maxTokens,
        temperature,
        active: profile?.active || false,
        disable_thinking: disableThinking,
      }

      if (isEditing && profile) {
        await updateTranslationProfile(profile.id, data)
        onToast('配置已更新', 'success')
      } else {
        await addTranslationProfile(data)
        onToast('配置已添加', 'success')
      }
      onSave()
    } catch (error) {
      onToast(String(error), 'error')
    } finally {
      saving = false
    }
  }
</script>

<Modal {title} {onClose}>
  {#snippet children()}
    <div class="form">
      <div class="form-group">
        <label for="profile-name">配置名称</label>
        <input id="profile-name" type="text" bind:value={name} placeholder="例如：GPT-4o 翻译" />
      </div>

      <div class="form-group">
        <label for="profile-credential">API 凭证</label>
        <select
          id="profile-credential"
          bind:value={credentialId}
          onchange={() => handleCredentialChange()}
        >
          <option value="">请选择凭证...</option>
          {#each credentials as cred}
            <option value={cred.id}>{cred.name} ({cred.type})</option>
          {/each}
        </select>
        {#if credentials.length === 0}
          <span class="help-text warning">请先添加 API 凭证</span>
        {/if}
      </div>

      <div class="form-group">
        <label for="profile-model">模型 (Model)</label>
        <input id="profile-model" type="text" bind:value={model} placeholder="例如：gpt-4o" />
      </div>

      <div class="advanced-options">
        <button
          type="button"
          class="toggle-advanced"
          onclick={() => (showAdvanced = !showAdvanced)}
        >
          <span class="icon">{showAdvanced ? '▼' : '▶'}</span>
          高级选项
        </button>

        {#if showAdvanced}
          <div class="advanced-fields">
            <div class="form-group">
              <label for="profile-prompt">System Prompt</label>
              <textarea
                id="profile-prompt"
                bind:value={systemPrompt}
                rows="4"
                placeholder="自定义系统提示词"
              ></textarea>
            </div>

            <div class="row">
              <div class="form-group half">
                <label for="profile-max-tokens">Max Tokens</label>
                <input id="profile-max-tokens" type="number" bind:value={maxTokens} />
              </div>
              <div class="form-group half">
                <label for="profile-temp">Temperature</label>
                <input
                  id="profile-temp"
                  type="number"
                  bind:value={temperature}
                  step="0.1"
                  min="0"
                  max="2"
                />
              </div>
            </div>

            {#if currentCredentialType === 'gemini'}
              <div class="form-group checkbox-group">
                <label>
                  <input type="checkbox" bind:checked={disableThinking} />
                  关闭思考模式 (Gemini 2.5 Flash 等)
                </label>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <div class="form-actions">
        <button class="btn btn-secondary" onclick={onClose} disabled={saving}>取消</button>
        <button
          class="btn btn-primary"
          onclick={handleSave}
          disabled={saving || credentials.length === 0}
        >
          {saving ? '保存中...' : '保存'}
        </button>
      </div>
    </div>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .form-group label {
    font-size: 14px;
    font-weight: 500;
    color: var(--color-text);
  }

  .form-group input,
  .form-group select,
  .form-group textarea {
    padding: 10px 12px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-size: 14px;
    background: var(--color-surface);
    color: var(--color-text);
    transition: border-color var(--transition-fast);
  }

  .form-group input:focus,
  .form-group select:focus,
  .form-group textarea:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  .row {
    display: flex;
    gap: 16px;
  }

  .half {
    flex: 1;
  }

  .help-text.warning {
    font-size: 12px;
    color: var(--color-warning, #f59e0b);
  }

  .advanced-options {
    margin-top: 8px;
    border-top: 1px solid var(--color-border);
    padding-top: 8px;
  }

  .toggle-advanced {
    background: none;
    border: none;
    color: var(--color-text-secondary);
    cursor: pointer;
    padding: 8px 0;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    width: 100%;
  }

  .toggle-advanced:hover {
    color: var(--color-text);
  }

  .advanced-fields {
    margin-top: 12px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
  }

  .checkbox-group input[type='checkbox'] {
    width: 16px;
    height: 16px;
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 16px;
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

  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-primary {
    background: var(--color-primary);
    color: #fff;
  }

  .btn-primary:hover:not(:disabled) {
    background: var(--color-primary-hover);
  }

  .btn-secondary {
    background: var(--color-surface);
    color: var(--color-text);
    border: 1px solid var(--color-border);
  }

  .btn-secondary:hover:not(:disabled) {
    background: var(--color-hover);
  }
</style>
