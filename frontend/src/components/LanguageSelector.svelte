<script lang="ts">
  import { LANGUAGES } from '../types'

  type Props = {
    value: string
    displayValue?: string
    onChange: (value: string) => void
    excludeCodes?: string[]
  }

  let { value, displayValue, onChange, excludeCodes = [] }: Props = $props()

  // Filter out excluded languages
  const filteredLanguages = $derived(
    excludeCodes.length > 0 ? LANGUAGES.filter((l) => !excludeCodes.includes(l.code)) : LANGUAGES
  )

  function handleChange(e: Event) {
    const select = e.target as HTMLSelectElement
    onChange(select.value)
  }
</script>

<div class="language-group">
  <select {value} onchange={handleChange}>
    {#each filteredLanguages as lang}
      <option value={lang.code}>
        {lang.code === value && displayValue ? displayValue : lang.name}
      </option>
    {/each}
  </select>
</div>

<style>
  .language-group {
    flex: 1;
    position: relative;
  }

  select {
    width: 100%;
    padding: 8px 12px;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    font-size: 14px;
    background-color: var(--color-input-bg);
    color: var(--color-text);
    appearance: none;
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  select:hover {
    background-color: var(--color-surface);
  }

  select:focus {
    outline: none;
    border-color: var(--color-primary);
    box-shadow: 0 0 0 2px rgba(0, 122, 255, 0.1);
  }
</style>
