<script lang="ts">
  import { setProviderActive, removeProvider } from '../services/wails'
  import type { Provider } from '../types'

  type Props = {
    provider: Provider
    onEdit: () => void
    onChange: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { provider, onEdit, onChange, onToast }: Props = $props()

  async function handleSetActive() {
    try {
      await setProviderActive(provider.name)
      onChange()
      onToast(`${provider.name} 已激活`, 'success')
    } catch (error) {
      onToast(String(error), 'error')
    }
  }

  async function handleRemove() {
    try {
      await removeProvider(provider.name)
      onChange()
      onToast(`${provider.name} 已删除`, 'success')
    } catch (error) {
      onToast(String(error), 'error')
    }
  }
</script>

<div class="provider-card" class:active={provider.active}>
  <div class="provider-header">
    <div class="provider-title">
      <span class="provider-name">{provider.name}</span>
      <span class="provider-type">
        {provider.type === 'openai'
          ? 'OpenAI'
          : provider.type === 'gemini'
            ? 'Google Gemini'
            : provider.type === 'claude'
              ? 'Anthropic Claude'
              : 'OpenAI 兼容服务'}
      </span>
    </div>
    <div class="provider-actions">
      <button class="action-btn" class:active={provider.active} onclick={handleSetActive}>
        {provider.active ? '已激活' : '激活'}
      </button>
      <button class="action-btn" onclick={onEdit}>编辑</button>
      <button class="delete-btn" onclick={handleRemove}>&times;</button>
    </div>
  </div>
  <div class="provider-info">
    <div>模型：{provider.model}</div>
    {#if provider.system_prompt}
      <div>系统提示词：{provider.system_prompt}</div>
    {/if}
    <div>最大 Token：{provider.max_tokens || 1000}</div>
    <div>温度：{provider.temperature || 0.3}</div>
  </div>
</div>

<style>
  .provider-card {
    background-color: var(--color-background);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-xl);
    padding: 20px;
    margin-bottom: 16px;
    transition: all var(--transition-fast);
  }

  .provider-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  }

  .provider-card.active {
    border-color: var(--color-primary);
    background-color: var(--color-surface);
  }

  .provider-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;
  }

  .provider-title {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .provider-name {
    font-size: 16px;
    font-weight: 600;
    color: var(--color-text);
  }

  .provider-type {
    font-size: 12px;
    color: var(--color-text-secondary);
    background: var(--color-surface);
    padding: 2px 8px;
    border-radius: var(--radius-sm);
  }

  .provider-actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .action-btn {
    padding: 4px 12px;
    border: 1px solid var(--color-primary);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-primary);
    font-size: 12px;
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .action-btn:hover {
    background: var(--color-surface);
  }

  .action-btn.active {
    background: var(--color-primary);
    color: white;
  }

  .delete-btn {
    width: 24px;
    height: 24px;
    border: none;
    background: transparent;
    color: var(--color-text-secondary);
    font-size: 18px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    transition: all var(--transition-fast);
  }

  .delete-btn:hover {
    background: var(--color-surface);
    color: var(--color-danger);
  }

  .provider-info {
    font-size: 13px;
    color: var(--color-text-secondary);
  }

  .provider-info div {
    margin-bottom: 4px;
  }

  .provider-info div:last-child {
    margin-bottom: 0;
  }
</style>
