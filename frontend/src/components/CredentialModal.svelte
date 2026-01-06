<script lang="ts">
  import Modal from './Modal.svelte'
  import { addCredential, updateCredential } from '../services/wails'
  import type { APICredential } from '../types'

  type Props = {
    credential?: APICredential | null
    onClose: () => void
    onSave: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { credential = null, onClose, onSave, onToast }: Props = $props()

  const isEdit = !!credential

  // Form state
  let name = $state(credential?.name || '')
  let type = $state<'openai' | 'openai-compatible' | 'gemini' | 'claude'>(
    credential?.type || 'openai'
  )
  let apiKey = $state(credential?.api_key || '')
  let baseUrl = $state(credential?.base_url || '')
  let saving = $state(false)

  // Type options
  const typeOptions = [
    { value: 'openai', label: 'OpenAI', placeholder: 'sk-...' },
    { value: 'gemini', label: 'Google Gemini', placeholder: 'AIza...' },
    { value: 'claude', label: 'Anthropic Claude', placeholder: 'sk-ant-...' },
    { value: 'openai-compatible', label: '自定义 API (OpenAI 兼容)', placeholder: 'your-api-key' },
  ] as const

  // Get placeholder for current type
  function getPlaceholder(): string {
    return typeOptions.find((t) => t.value === type)?.placeholder || ''
  }

  // Handle save
  async function handleSave() {
    if (!name.trim()) {
      onToast('请输入凭证名称', 'error')
      return
    }
    if (!apiKey.trim()) {
      onToast('请输入 API Key', 'error')
      return
    }
    if (type === 'openai-compatible' && !baseUrl.trim()) {
      onToast('自定义 API 需要输入 Base URL', 'error')
      return
    }

    saving = true
    try {
      const cred: APICredential = {
        id: credential?.id || '',
        name: name.trim(),
        type,
        api_key: apiKey.trim(),
        base_url: type === 'openai-compatible' ? baseUrl.trim() : undefined,
      }

      if (isEdit && credential) {
        await updateCredential(credential.id, cred)
        onToast('凭证已更新', 'success')
      } else {
        await addCredential(cred)
        onToast('凭证已添加', 'success')
      }
      onSave()
    } catch (error) {
      onToast(String(error), 'error')
    } finally {
      saving = false
    }
  }
</script>

<Modal title={isEdit ? '编辑 API 凭证' : '添加 API 凭证'} {onClose}>
  {#snippet children()}
    <div class="form">
      <div class="form-group">
        <label for="cred-name">凭证名称</label>
        <input id="cred-name" type="text" bind:value={name} placeholder="例如：我的 OpenAI" />
      </div>

      <div class="form-group">
        <label for="cred-type">API 类型</label>
        <select id="cred-type" bind:value={type}>
          {#each typeOptions as option}
            <option value={option.value}>{option.label}</option>
          {/each}
        </select>
      </div>

      <div class="form-group">
        <label for="cred-key">API Key</label>
        <input id="cred-key" type="password" bind:value={apiKey} placeholder={getPlaceholder()} />
      </div>

      {#if type === 'openai-compatible'}
        <div class="form-group">
          <label for="cred-url">Base URL</label>
          <input
            id="cred-url"
            type="url"
            bind:value={baseUrl}
            placeholder="https://api.example.com/v1"
          />
          <span class="help-text">OpenAI 兼容 API 的基础地址</span>
        </div>
      {/if}

      <div class="form-actions">
        <button class="btn btn-secondary" onclick={onClose} disabled={saving}>取消</button>
        <button class="btn btn-primary" onclick={handleSave} disabled={saving}>
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
  .form-group select {
    padding: 10px 12px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-size: 14px;
    background: var(--color-surface);
    color: var(--color-text);
    transition: border-color var(--transition-fast);
  }

  .form-group input:focus,
  .form-group select:focus {
    outline: none;
    border-color: var(--color-primary);
  }

  .help-text {
    font-size: 12px;
    color: var(--color-text-secondary);
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 8px;
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
