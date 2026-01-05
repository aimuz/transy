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
    bottom: 24px;
    left: 50%;
    transform: translateX(-50%);
    padding: 10px 16px;
    background: var(--color-text); /* Solid dark/light inverse */
    color: var(--color-background); /* Inverse text */
    border-radius: var(--radius-lg);
    font-size: 13px;
    font-weight: 500;
    z-index: 10000;
    animation: slideUp 0.3s cubic-bezier(0.16, 1, 0.3, 1);
    box-shadow: 0 8px 16px rgba(0, 0, 0, 0.12);
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: fit-content;
    white-space: nowrap;
  }

  /* Status Indicator Dot */
  .toast::before {
    content: '';
    display: block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .toast.info::before {
    background: var(--color-text-secondary); /* Grey dot */
  }

  .toast.success::before {
    background: #30d158; /* Bright Green dot */
    box-shadow: 0 0 8px rgba(48, 209, 88, 0.4);
  }

  .toast.error::before {
    background: #ff453a; /* Bright Red dot */
    box-shadow: 0 0 8px rgba(255, 69, 58, 0.4);
  }

  @media (prefers-color-scheme: dark) {
    .toast {
      background: #333; /* Slightly lighter than pure black for contrast */
      color: white;
      border: 1px solid rgba(255, 255, 255, 0.1);
    }
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
