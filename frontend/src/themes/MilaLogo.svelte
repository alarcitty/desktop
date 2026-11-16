<script lang="ts">
  interface Props {
    class?: string;
    ariaLabel?: string;
  }

  let { class: className = '', ariaLabel = 'Mila' }: Props = $props();
</script>

<svg
  class="mila-logo-svg {className}"
  viewBox="0 0 67 66"
  fill="none"
  xmlns="http://www.w3.org/2000/svg"
  role="img"
  aria-label={ariaLabel}
>
  <path
    d="M10 40L13.082 26H25.4098L30.0328 40H25.4098L20.7869 28.1L22.3279 40H19.2459L15.3934 28.1L16.1639 40H10Z"
    fill="currentColor"
  />
  <path d="M31.5738 40L26.9508 26H31.5738L36.1967 40H31.5738Z" fill="currentColor" />
  <path d="M37.7377 40L33.1148 26H37.7377L42.3607 40H37.7377Z" fill="currentColor" />
  <path d="M52.377 40L42.3607 26H46.2131L57 40H52.377Z" fill="currentColor" />
  <path d="M43.9016 40L39.2787 26H43.9016L48.5246 40H43.9016Z" fill="currentColor" />
</svg>

<style>
  .mila-logo-svg {
    display: block;
    color: var(--mila-logo-color, #ffffff);
    user-select: none;
    pointer-events: none;
    transition: color 360ms cubic-bezier(0.22, 1, 0.36, 1);
  }
</style>
