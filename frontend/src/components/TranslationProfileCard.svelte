<script lang="ts">
  import {
    setTranslationProfileActive,
    removeTranslationProfile,
    getCredentials,
  } from '../services/wails'
  import type { TranslationProfile, APICredential } from '../types'

  type Props = {
    profile: TranslationProfile
    onEdit: () => void
    onChange: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { profile, onEdit, onChange, onToast }: Props = $props()

  let credentialName = $state('Unknown Credential')
  let credentialType = $state('')

  // Fetch credential info
  $effect(() => {
    getCredentials().then((creds: APICredential[]) => {
      const cred = creds.find((c) => c.id === profile.credential_id)
      if (cred) {
        credentialName = cred.name
        credentialType = cred.type
      }
    })
  })

  async function handleSetActive() {
    try {
      await setTranslationProfileActive(profile.id)
      onChange()
      onToast(`${profile.name} 已激活`, 'success')
    } catch (error) {
      onToast(String(error), 'error')
    }
  }

  async function handleRemove() {
    if (!confirm(`确定要删除配置 "${profile.name}" 吗？`)) return

    try {
      await removeTranslationProfile(profile.id)
      onChange()
      onToast(`${profile.name} 已删除`, 'success')
    } catch (error) {
      onToast(String(error), 'error')
    }
  }

  function getTypeLabel(type: string): string {
    const labels: Record<string, string> = {
      openai: 'OpenAI',
      'openai-compatible': '自定义',
      gemini: 'Gemini',
      claude: 'Claude',
    }
    return labels[type] || type
  }
</script>

<div class="profile-card" class:active={profile.active}>
  <div class="profile-header">
    <div class="profile-title">
      <span class="profile-name">{profile.name}</span>
      {#if credentialType}
        <span class="profile-type">{getTypeLabel(credentialType)}</span>
      {/if}
    </div>
    <div class="profile-actions">
      <button class="action-btn" class:active={profile.active} onclick={handleSetActive}>
        {profile.active ? '已激活' : '激活'}
      </button>
      <button class="action-btn" onclick={onEdit}>编辑</button>
      <button class="delete-btn" onclick={handleRemove}>&times;</button>
    </div>
  </div>
  <div class="profile-info">
    <div><b>凭证：</b> {credentialName}</div>
    <div><b>模型：</b> {profile.model}</div>
    {#if profile.system_prompt}
      <div class="truncate"><b>Prompt：</b> {profile.system_prompt}</div>
    {/if}
  </div>
</div>

<style>
  .profile-card {
    background-color: var(--color-background);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-xl);
    padding: 16px;
    margin-bottom: 12px;
    transition: all var(--transition-fast);
  }

  .profile-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
  }

  .profile-card.active {
    border-color: var(--color-primary);
    background-color: var(--color-surface);
  }

  .profile-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  .profile-title {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .profile-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--color-text);
  }

  .profile-type {
    font-size: 11px;
    color: var(--color-text-secondary);
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    padding: 1px 6px;
    border-radius: var(--radius-sm);
  }

  .profile-actions {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .action-btn {
    padding: 4px 10px;
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
    background: rgba(239, 68, 68, 0.1);
    color: #ef4444;
  }

  .profile-info {
    font-size: 13px;
    color: var(--color-text-secondary);
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .truncate {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100%;
  }

  b {
    font-weight: 500;
    color: var(--color-text);
  }
</style>
