<div class="watch-together-soon">
  <span class="soon-label">Soon</span>
</div>

<style>
  .watch-together-soon {
    width: 100%;
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--mila-app-canvas, #0c0c0c);
    transition: background-color 360ms cubic-bezier(0.22, 1, 0.36, 1);
    user-select: none;
  }

  .soon-label {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 4.2vw, 64px);
    font-weight: 700;
    line-height: 1.06;
    letter-spacing: -0.02em;
    color: var(--mila-popup-comboSectionText, rgba(255, 255, 255, 0.2));
  }
</style>
