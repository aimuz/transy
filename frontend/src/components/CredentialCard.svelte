<script lang="ts">
  import { removeCredential } from '../services/wails'
  import type { APICredential } from '../types'

  type Props = {
    credential: APICredential
    inUse: boolean
    onEdit: () => void
    onChange: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { credential, inUse, onEdit, onChange, onToast }: Props = $props()

  // Mask API key for display
  function maskApiKey(key: string): string {
    if (key.length <= 8) return '••••••••'
    return key.slice(0, 4) + '••••' + key.slice(-4)
  }

  // Get type display name
  function getTypeLabel(type: string): string {
    const labels: Record<string, string> = {
      openai: 'OpenAI',
      'openai-compatible': '自定义',
      gemini: 'Gemini',
      claude: 'Claude',
    }
    return labels[type] || type
  }

  // Delete credential
  async function handleDelete() {
    if (inUse) {
      onToast('此凭证正在被使用，无法删除', 'error')
      return
    }

    if (!confirm(`确定要删除 "${credential.name}" 吗？`)) return

    try {
      await removeCredential(credential.id)
      onToast('凭证已删除', 'success')
      onChange()
    } catch (error) {
      onToast(String(error), 'error')
    }
  }
</script>

<div class="credential-card">
  <div class="credential-info">
    <div class="credential-header">
      <span class="credential-name">{credential.name}</span>
      <span class="credential-type">{getTypeLabel(credential.type)}</span>
    </div>
    <div class="credential-key">{maskApiKey(credential.api_key)}</div>
    {#if credential.base_url}
      <div class="credential-url">{credential.base_url}</div>
    {/if}
  </div>
  <div class="credential-actions">
    <button class="btn-icon" onclick={onEdit} title="编辑">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
        <path d="m18.5 2.5 3 3L12 15l-4 1 1-4 9.5-9.5z" />
      </svg>
    </button>
    <button
      class="btn-icon btn-delete"
      onclick={handleDelete}
      disabled={inUse}
      title={inUse ? '此凭证正在被使用' : '删除'}
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="3 6 5 6 21 6" />
        <path d="m19 6-.867 12.142A2 2 0 0 1 16.138 20H7.862a2 2 0 0 1-1.995-1.858L5 6m5 4v6m4-6v6" />
        <path d="M14 6V4a2 2 0 0 0-2-2H12a2 2 0 0 0-2 2v2" />
      </svg>
    </button>
  </div>
</div>

<style>
  .credential-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    background: var(--color-surface);
    border-radius: var(--radius-lg);
    margin-bottom: 8px;
    border: 1px solid var(--color-border);
  }

  .credential-info {
    flex: 1;
    min-width: 0;
  }

  .credential-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
  }

  .credential-name {
    font-weight: 500;
    color: var(--color-text);
  }

  .credential-type {
    font-size: 11px;
    padding: 2px 6px;
    background: var(--color-primary);
    color: #fff;
    border-radius: var(--radius-sm);
  }

  .credential-key {
    font-size: 12px;
    color: var(--color-text-secondary);
    font-family: monospace;
  }

  .credential-url {
    font-size: 11px;
    color: var(--color-text-tertiary);
    margin-top: 2px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .credential-actions {
    display: flex;
    gap: 4px;
  }

  .btn-icon {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-radius: var(--radius-md);
    color: var(--color-text-secondary);
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .btn-icon:hover {
    background: var(--color-hover);
    color: var(--color-text);
  }

  .btn-icon:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .btn-delete:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.1);
    color: #ef4444;
  }
</style>
