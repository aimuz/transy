<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    title: string
    onClose: () => void
    children: Snippet
  }

  let { title, onClose, children }: Props = $props()

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      onClose()
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose()
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions a11y_interactive_supports_focus -->
<div class="modal" onclick={handleBackdropClick} role="dialog" aria-modal="true" tabindex="-1">
  <div class="modal-content">
    <div class="modal-header">
      <h2>{title}</h2>
      <button class="close-btn" onclick={onClose}>&times;</button>
    </div>
    <div class="modal-body">
      {@render children()}
    </div>
  </div>
</div>

<style>
  .modal {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.5);
    z-index: 1000;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 40px;
  }

  .modal-content {
    background: var(--color-background);
    border-radius: var(--radius-xl);
    width: 90%;
    max-width: 600px;
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.2);
  }

  .modal-header {
    padding: 20px;
    border-bottom: 1px solid var(--color-border);
    display: flex;
    justify-content: space-between;
    align-items: center;
    position: sticky;
    top: 0;
    background: var(--color-background);
    z-index: 1;
  }

  .modal-header h2 {
    margin: 0;
    font-size: 20px;
    font-weight: 600;
    color: var(--color-text);
  }

  .modal-body {
    padding: 20px;
  }

  .close-btn {
    background: none;
    border: none;
    font-size: 24px;
    color: var(--color-text-secondary);
    cursor: pointer;
    padding: 0;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    transition: all var(--transition-fast);
  }

  .close-btn:hover {
    background: var(--color-surface);
    color: var(--color-text);
  }
</style>
