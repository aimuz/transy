<script lang="ts">
  import Modal from './Modal.svelte'
  import { addProvider, updateProvider } from '../services/wails'
  import type { Provider } from '../types'

  type Props = {
    provider?: Provider
    onClose: () => void
    onSave: () => void
    onToast: (message: string, type?: 'info' | 'error' | 'success') => void
  }

  let { provider, onClose, onSave, onToast }: Props = $props()

  // Determine if editing
  let isEditing = $derived(!!provider)
  let title = $derived(isEditing ? '编辑翻译提供商' : '添加翻译提供商')

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

  // Form state - initialized from provider prop (captures initial value intentionally)
  let type = $state<'openai' | 'openai-compatible' | 'gemini' | 'claude'>('openai')
  let name = $state('')
  let baseUrl = $state('')
  let apiKey = $state('')
  let model = $state('')
  let systemPrompt = $state(DEFAULT_SETTINGS.systemPrompt)
  let maxTokens = $state(DEFAULT_SETTINGS.maxTokens)
  let temperature = $state(DEFAULT_SETTINGS.temperature)
  let disableThinking = $state(false)
  let showAdvanced = $state(false)

  // Initialize form from provider when component mounts
  $effect(() => {
    if (provider) {
      type = provider.type || 'openai'
      name = provider.name || ''
      baseUrl = provider.base_url || ''
      apiKey = provider.api_key || ''
      model = provider.model || ''
      systemPrompt = provider.system_prompt || DEFAULT_SETTINGS.systemPrompt
      maxTokens = provider.max_tokens || DEFAULT_SETTINGS.maxTokens
      temperature = provider.temperature || DEFAULT_SETTINGS.temperature
      disableThinking = provider.disable_thinking || false
    }
  })

  // Show base URL field when type is openai-compatible, gemini or claude
  let showBaseUrl = $derived(type !== 'openai')

  // Auto-fill defaults when type changes
  function handleTypeChange() {
    if (type === 'gemini') {
      if (!model) model = 'gemini-1.5-flash'
      if (!name) name = 'Gemini'
    } else if (type === 'claude') {
      if (!model) model = 'claude-3-5-sonnet-latest'
      if (!name) name = 'Claude'
    } else if (type === 'openai') {
      if (!model) model = 'gpt-4o'
      if (!name) name = 'OpenAI'
    }
  }

  // Dynamic placeholder for Base URL
  let baseUrlPlaceholder = $derived.by(() => {
    if (type === 'gemini') return '例如：https://generativelanguage.googleapis.com/v1beta/models'
    if (type === 'claude') return '例如：https://api.anthropic.com/v1/messages'
    return '例如：https://api.example.com/v1/chat/completions'
  })

  // Save handler
  async function handleSave() {
    try {
      const providerData: Provider = {
        name,
        type,
        base_url: baseUrl,
        api_key: apiKey,
        model,
        system_prompt: systemPrompt,
        max_tokens: maxTokens,
        temperature,
        active: true,
        disable_thinking: disableThinking,
      }

      if (isEditing && provider) {
        await updateProvider(provider.name, providerData)
        onToast(`更新 ${name} 成功`, 'success')
      } else {
        await addProvider(providerData)
        onToast(`添加 ${name} 成功`, 'success')
      }

      onSave()
    } catch (error) {
      onToast(String(error), 'error')
    }
  }
</script>

<Modal {title} {onClose}>
  {#snippet children()}
    <div class="form-group">
      <label for="provider-type">类型</label>
      <select id="provider-type" bind:value={type} onchange={handleTypeChange}>
        <option value="openai">OpenAI</option>
        <option value="openai-compatible">OpenAI 兼容服务</option>
        <option value="gemini">Google Gemini</option>
        <option value="claude">Anthropic Claude</option>
      </select>
    </div>

    <div class="form-group">
      <label for="provider-name">名称</label>
      <input id="provider-name" type="text" bind:value={name} placeholder="例如：OpenAI" />
    </div>

    {#if showBaseUrl}
      <div class="form-group">
        <label for="provider-base-url">API Base URL</label>
        <input
          id="provider-base-url"
          type="text"
          bind:value={baseUrl}
          placeholder={baseUrlPlaceholder}
        />
      </div>
    {/if}

    <div class="form-group">
      <label for="provider-api-key">API Key</label>
      <input id="provider-api-key" type="password" bind:value={apiKey} />
    </div>

    <div class="form-group">
      <label for="provider-model">Model</label>
      <input id="provider-model" type="text" bind:value={model} placeholder="例如：gpt-3.5-turbo" />
    </div>

    <div class="advanced-options">
      <button type="button" class="toggle-advanced" onclick={() => (showAdvanced = !showAdvanced)}>
        <span class="icon">{showAdvanced ? '▼' : '▶'}</span>
        高级选项
      </button>

      {#if showAdvanced}
        <div class="advanced-fields">
          <div class="form-group">
            <label for="provider-prompt">System Prompt</label>
            <textarea id="provider-prompt" bind:value={systemPrompt} placeholder="自定义系统提示词"
            ></textarea>
          </div>
          <div class="form-group">
            <label for="provider-max-tokens">Max Tokens</label>
            <input id="provider-max-tokens" type="number" bind:value={maxTokens} />
          </div>
          <div class="form-group">
            <label for="provider-temperature">Temperature</label>
            <input
              id="provider-temperature"
              type="number"
              bind:value={temperature}
              step="0.1"
              min="0"
              max="2"
            />
          </div>
          {#if type === 'gemini'}
            <div class="form-group checkbox-group">
              <label>
                <input type="checkbox" bind:checked={disableThinking} />
                关闭思考模式
              </label>
              <p class="hint">适用于 Gemini 2.5 Flash 等支持思考的模型，关闭后可减少延迟和成本</p>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <button class="save-btn" onclick={handleSave}>
      {isEditing ? '保存更改' : '添加'}
    </button>
  {/snippet}
</Modal>

<style>
  .advanced-options {
    margin-top: 16px;
    border-top: 1px solid var(--color-border);
    padding-top: 16px;
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
    text-align: left;
  }

  .toggle-advanced:hover {
    color: var(--color-text);
  }

  .icon {
    font-size: 12px;
    transition: transform var(--transition-fast);
  }

  .advanced-fields {
    margin-top: 16px;
  }

  .save-btn {
    width: 100%;
    padding: 12px;
    background: var(--color-primary);
    color: #fff;
    border: none;
    border-radius: var(--radius-lg);
    font-size: 14px;
    cursor: pointer;
    transition: all var(--transition-fast);
    margin-top: 16px;
  }

  .save-btn:hover {
    background: var(--color-primary-hover);
  }

  .checkbox-group {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    font-size: 14px;
  }

  .checkbox-group input[type='checkbox'] {
    width: 16px;
    height: 16px;
    cursor: pointer;
  }

  .hint {
    font-size: 12px;
    color: var(--color-text-secondary);
    margin: 0;
    line-height: 1.4;
  }
</style>
