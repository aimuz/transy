<script lang="ts">
  type Props = {
    message: string
    type: 'info' | 'error' | 'success'
    visible: boolean
  }

  let { message, type, visible }: Props = $props()
</script>

{#if visible}
  <div class="toast {type}">
    {message}
  </div>
{/if}

<style>
  .toast {
    position: fixed;
    bottom: 20px;
    left: 50%;
    transform: translateX(-50%);
    padding: 12px 24px;
    background: var(--color-surface);
    color: var(--color-text);
    border-radius: var(--radius-lg);
    font-size: 14px;
    z-index: 10000;
    animation: slideUp 0.3s ease;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  }

  .toast.error {
    background: var(--color-danger);
    color: white;
  }

  .toast.success {
    background: var(--color-success-bg);
    color: white;
  }

  @keyframes slideUp {
    from {
      opacity: 0;
      transform: translateX(-50%) translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateX(-50%) translateY(0);
    }
  }
</style>
