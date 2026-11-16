<script lang="ts">
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import MainButton from '../../marlakit/button/MainButton.svelte';
  import RgbLine from '../../marlakit/slider/RgbLine.svelte';
  import logoSrc from '../../assets/mila_header.svg';

  type SetupStep = 'welcome' | 'question' | 'done' | 'outro';
  type LanguagePref = 'ru' | 'en' | 'zh';
  type ThemePref = 'dark' | 'light' | 'custom';
  type QualityPref = '1080p' | '720p' | 'auto';

  interface Props {
    ready?: boolean;
    onContinue: () => void;
  }

  let { ready = true, onContinue }: Props = $props();

  const LANG_KEY = 'mila.setup.language';
  const THEME_KEY = 'mila.setup.theme';
  const HUE_KEY = 'mila.setup.customHue';
  const QUALITY_KEY = 'mila.setup.quality';

  const langOptions: Array<{ value: LanguagePref; label: string }> = [
    { value: 'ru', label: 'Русский' },
    { value: 'en', label: 'English' },
    { value: 'zh', label: '中文' }
  ];

  const themeOptions: Array<{ value: ThemePref; label: string }> = [
    { value: 'dark', label: 'Dark' },
    { value: 'light', label: 'Light' },
    { value: 'custom', label: 'Custom' }
  ];

  const qualityOptions: Array<{ value: QualityPref; label: string }> = [
    { value: '1080p', label: '1080p' },
    { value: '720p', label: '720p' },
    { value: 'auto', label: 'Auto' }
  ];

  function readStorage<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
    try {
      const saved = window.localStorage.getItem(key) as T | null;
      if (saved && allowed.includes(saved)) return saved;
    } catch {
      // Ignore restricted storage contexts
    }
    return fallback;
  }

  function readSavedHue(): number {
    try {
      const n = Number(window.localStorage.getItem(HUE_KEY));
      if (Number.isFinite(n) && n >= 0 && n <= 360) return n;
    } catch {
      // Ignore restricted storage contexts
    }
    return 215;
  }

  let step = $state<SetupStep>('welcome');
  let qIndex = $state(0);
  let selectedLang = $state<LanguagePref>(readStorage(LANG_KEY, ['ru', 'en', 'zh'], 'ru'));
  let selectedTheme = $state<ThemePref>(readStorage(THEME_KEY, ['dark', 'light', 'custom'], 'dark'));
  let customHue = $state<number>(readSavedHue());
  let selectedQuality = $state<QualityPref>(
    readStorage(QUALITY_KEY, ['1080p', '720p', 'auto'], '1080p')
  );
  let slideDir = $state<1 | -1>(1);
  let pendingContinue = $state(false);
  let sequenceStarted = false;
  let doneTimer: number | undefined;
  let exitTimer: number | undefined;

  const TOTAL_QUESTIONS = 3;
  const isExpanded = $derived(step === 'done' || step === 'outro');

  const currentQuestionMeta = $derived.by(() => {
    if (qIndex === 0) {
      return {
        title: 'Choose your language',
        subtitle: 'Select the language used across the interface and catalog.',
        ariaLabel: 'Interface language',
        valueKey: selectedLang,
        valueLabel: langOptions.find((o) => o.value === selectedLang)?.label ?? 'Русский'
      };
    }
    if (qIndex === 1) {
      return {
        title: 'Choose your theme',
        subtitle: 'Pick the appearance that feels right for your space.',
        ariaLabel: 'Theme appearance',
        valueKey: selectedTheme,
        valueLabel: themeOptions.find((o) => o.value === selectedTheme)?.label ?? 'Dark'
      };
    }
    return {
      title: 'Default video quality',
      subtitle: 'Preferred resolution when starting an episode.',
      ariaLabel: 'Default video quality',
      valueKey: selectedQuality,
      valueLabel: qualityOptions.find((o) => o.value === selectedQuality)?.label ?? '1080p'
    };
  });

  function revealBlur(
    _node: HTMLElement,
    params: { duration?: number; delay?: number; y?: number; scale?: number; blur?: number } = {}
  ) {
    const duration = params.duration ?? 420;
    const delay = params.delay ?? 0;
    const y = params.y ?? 10;
    const scale = params.scale ?? 0.97;
    const blur = params.blur ?? 6;
    return {
      duration,
      delay,
      easing: cubicOut,
      css: (t: number) => {
        const inv = 1 - t;
        return `opacity: ${t}; transform: translate3d(0, ${(y * inv).toFixed(2)}px, 0) scale(${(
          scale +
          (1 - scale) * t
        ).toFixed(4)}); filter: blur(${(blur * inv).toFixed(2)}px);`;
      }
    };
  }

  function slideBlur(
    _node: HTMLElement,
    params: { dir: 1 | -1; mode: 'in' | 'out'; duration?: number }
  ) {
    const duration = params.duration ?? 340;
    const offset = (params.mode === 'in' ? 1 : -1) * params.dir * 34;
    const startScale = 0.82;
    const maxBlur = 8;
    return {
      duration,
      easing: cubicOut,
      css: (t: number) => {
        const inv = 1 - t;
        const x = offset * inv;
        const s = startScale + (1 - startScale) * t;
        const b = maxBlur * inv;
        return `opacity: ${t}; transform: translate3d(${x.toFixed(2)}px, 0, 0) scale(${s.toFixed(
          4
        )}); filter: blur(${b.toFixed(2)}px);`;
      }
    };
  }

  function saveStorage(key: string, val: string) {
    try {
      window.localStorage.setItem(key, val);
    } catch {
      // Ignore storage quota / restricted context errors
    }
  }

  function handleWelcome() {
    if (step !== 'welcome') return;
    qIndex = 0;
    step = 'question';
  }

  function cycleCurrent(delta: 1 | -1) {
    slideDir = delta;
    if (qIndex === 0) {
      const idx = langOptions.findIndex((o) => o.value === selectedLang);
      const next = langOptions[(idx + delta + langOptions.length) % langOptions.length].value;
      selectedLang = next;
      saveStorage(LANG_KEY, next);
      return;
    }
    if (qIndex === 1) {
      const idx = themeOptions.findIndex((o) => o.value === selectedTheme);
      const next = themeOptions[(idx + delta + themeOptions.length) % themeOptions.length].value;
      selectedTheme = next;
      saveStorage(THEME_KEY, next);
      return;
    }
    const idx = qualityOptions.findIndex((o) => o.value === selectedQuality);
    const next =
      qualityOptions[(idx + delta + qualityOptions.length) % qualityOptions.length].value;
    selectedQuality = next;
    saveStorage(QUALITY_KEY, next);
  }

  function handleHueChange(nextHue: number) {
    customHue = nextHue;
    saveStorage(HUE_KEY, String(nextHue));
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (step !== 'question') return;
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      cycleCurrent(-1);
    } else if (e.key === 'ArrowRight') {
      e.preventDefault();
      cycleCurrent(1);
    }
  }

  function startDoneSequence() {
    if (sequenceStarted) return;
    sequenceStarted = true;
    step = 'done';

    doneTimer = window.setTimeout(() => {
      step = 'outro';
      exitTimer = window.setTimeout(() => {
        onContinue();
      }, 430);
    }, 1550);
  }

  function handleContinue() {
    if (step === 'question' && qIndex < TOTAL_QUESTIONS - 1) {
      qIndex += 1;
      return;
    }
    if (ready) {
      startDoneSequence();
      return;
    }
    pendingContinue = true;
  }

  $effect(() => {
    if (pendingContinue && ready) {
      startDoneSequence();
    }
  });

  $effect(() => () => {
    window.clearTimeout(doneTimer);
    window.clearTimeout(exitTimer);
  });
</script>

<svelte:window onkeydown={handleKeyDown} />

<div
  class="setup-scene"
  class:is-question={step === 'question'}
  class:is-expanded={isExpanded}
  role="dialog"
  aria-modal="true"
  aria-label="Welcome to Mila"
  out:fade={{ duration: 460, easing: cubicOut }}
>
  <div
    class="setup-ambient"
    class:is-elevated={step === 'question'}
    class:is-done={isExpanded}
    aria-hidden="true"
  ></div>

  <div class="setup-logo-anchor" class:is-elevated={step !== 'welcome'}>
    {#if step === 'welcome' || step === 'question'}
      <div
        class="setup-logo-frame"
        out:revealBlur={{ duration: 340, y: -12, scale: 0.92, blur: 7 }}
      >
        <img src={logoSrc} alt="Mila" class="setup-logo" decoding="async" draggable="false" />
      </div>
    {/if}
  </div>

  {#if step === 'question'}
    {#key qIndex}
      <div
        class="setup-question-stage"
        in:revealBlur={{ duration: 460, delay: 180, y: 14, scale: 0.97, blur: 8 }}
        out:revealBlur={{ duration: 300, y: -10, scale: 0.96, blur: 8 }}
      >
        <div class="setup-question-header">
          <h1 class="setup-question-title">{currentQuestionMeta.title}</h1>
          <p class="setup-question-sub">{currentQuestionMeta.subtitle}</p>
        </div>

        <div class="setup-stepper-zone">
          <div class="setup-stepper" role="group" aria-label={currentQuestionMeta.ariaLabel}>
            <button
              type="button"
              class="stepper-chevron"
              aria-label="Previous option"
              onclick={() => cycleCurrent(-1)}
            >
              <ChevronLeft size={28} strokeWidth={2} />
            </button>

            <div class="stepper-value-slot" aria-live="polite">
              {#key currentQuestionMeta.valueKey}
                <span
                  class="stepper-value"
                  in:slideBlur={{ dir: slideDir, mode: 'in' }}
                  out:slideBlur={{ dir: slideDir, mode: 'out' }}
                >
                  {currentQuestionMeta.valueLabel}
                </span>
              {/key}
            </div>

            <button
              type="button"
              class="stepper-chevron"
              aria-label="Next option"
              onclick={() => cycleCurrent(1)}
            >
              <ChevronRight size={28} strokeWidth={2} />
            </button>
          </div>

          {#if qIndex === 1 && selectedTheme === 'custom'}
            <div
              class="setup-rgb-wrap"
              in:revealBlur={{ duration: 320, y: 8, scale: 0.95, blur: 6 }}
              out:revealBlur={{ duration: 220, y: 6, scale: 0.95, blur: 5 }}
            >
              <RgbLine bind:hue={customHue} onchange={handleHueChange} />
            </div>
          {/if}
        </div>
      </div>
    {/key}
  {/if}

  {#if step === 'done'}
    <div
      class="setup-done-stage"
      in:revealBlur={{ duration: 540, delay: 300, y: 12, scale: 0.96, blur: 8 }}
      out:revealBlur={{ duration: 420, y: -8, scale: 0.97, blur: 8 }}
    >
      <h1 class="setup-done-title">All done</h1>
    </div>
  {/if}

  <div class="setup-footer">
    {#if step === 'welcome'}
      <div
        class="setup-btn-slot"
        out:revealBlur={{ duration: 240, y: 8, scale: 0.95, blur: 6 }}
      >
        <MainButton class="setup-primary-btn" onclick={handleWelcome}>
          Welcome
        </MainButton>
      </div>
    {:else if step === 'question'}
      <div
        class="setup-btn-slot"
        in:revealBlur={{ duration: 420, delay: 260, y: 10, scale: 0.96, blur: 6 }}
        out:revealBlur={{ duration: 300, y: 10, scale: 0.95, blur: 6 }}
      >
        <MainButton
          class="setup-primary-btn"
          loading={pendingContinue && !ready}
          onclick={handleContinue}
        >
          Continue
        </MainButton>
      </div>
    {/if}
  </div>
</div>

<style>
  .setup-scene {
    position: absolute;
    left: 50%;
    top: 50%;
    z-index: 9999;
    width: clamp(540px, 54vw, 720px);
    height: clamp(440px, 58vh, 540px);
    transform: translate3d(-50%, -50%, 0);
    border-radius: 15px;
    background: #0c0c0c;
    overflow: hidden;
    -webkit-app-region: drag;
    transition:
      width 760ms cubic-bezier(0.22, 1, 0.36, 1),
      height 760ms cubic-bezier(0.22, 1, 0.36, 1),
      border-radius 760ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .setup-scene.is-expanded {
    width: 100%;
    height: 100%;
  }

  .setup-ambient {
    position: absolute;
    left: 50%;
    top: 44%;
    width: 380px;
    height: 190px;
    transform: translate3d(-50%, -50%, 0) scale(1);
    background: radial-gradient(
      ellipse at center,
      rgba(255, 255, 255, 0.055) 0%,
      rgba(255, 255, 255, 0.015) 45%,
      transparent 75%
    );
    pointer-events: none;
    transition:
      top 680ms cubic-bezier(0.22, 1, 0.36, 1),
      transform 680ms cubic-bezier(0.22, 1, 0.36, 1),
      opacity 680ms cubic-bezier(0.22, 1, 0.36, 1);
    animation: setup-glow-in 900ms cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  .setup-ambient.is-elevated {
    top: 18%;
    transform: translate3d(-50%, -50%, 0) scale(1.1);
    opacity: 0.72;
  }

  .setup-ambient.is-done {
    top: 50%;
    transform: translate3d(-50%, -50%, 0) scale(1.2);
    opacity: 0.55;
  }

  .setup-logo-anchor {
    position: absolute;
    top: 26px;
    left: 50%;
    z-index: 2;
    transform-origin: center top;
    transform: translate3d(-50%, 128px, 0) scale(0.92);
    transition: transform 660ms cubic-bezier(0.22, 1, 0.36, 1);
    pointer-events: none;
  }

  .setup-logo-anchor.is-elevated {
    transform: translate3d(-50%, 0, 0) scale(0.52);
  }

  .setup-logo-frame {
    width: 240px;
    height: 92px;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    animation: setup-logo-in 620ms cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  .setup-logo {
    width: 236px;
    height: auto;
    display: block;
    user-select: none;
    pointer-events: none;
  }

  .setup-question-stage {
    position: absolute;
    top: 98px;
    bottom: 76px;
    left: 28px;
    right: 28px;
    z-index: 5;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-start;
    text-align: center;
    pointer-events: none;
  }

  .setup-question-header {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .setup-question-title {
    max-width: 560px;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(24px, 2.8vw, 32px);
    font-weight: 700;
    line-height: 1.12;
    letter-spacing: -0.025em;
    color: #ffffff;
    margin: 0;
  }

  .setup-question-sub {
    max-width: 420px;
    margin-top: 10px;
    font-family: 'Medeo Sans Display', 'Medeo Sans', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 400;
    line-height: 1.42;
    color: rgba(255, 255, 255, 0.46);
  }

  .setup-stepper-zone {
    position: relative;
    flex: 1;
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .setup-stepper {
    width: 100%;
    max-width: 390px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    pointer-events: auto;
    -webkit-app-region: no-drag;
  }

  .stepper-chevron {
    background: transparent;
    border: none;
    outline: none;
    box-shadow: none;
    padding: 8px;
    margin: 0;
    color: rgba(255, 255, 255, 0.46);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    cursor: pointer;
    transition:
      color 160ms ease,
      transform 140ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .stepper-chevron:hover {
    color: #ffffff;
  }

  .stepper-chevron:active {
    transform: scale(0.88);
  }

  .stepper-value-slot {
    flex: 1;
    display: grid;
    place-items: center;
    height: 54px;
    overflow: visible;
  }

  .stepper-value {
    grid-area: 1 / 1;
    font-family: 'RF Dewi Extended', 'Medeo Sans', system-ui, sans-serif;
    font-size: 36px;
    font-weight: 700;
    line-height: 1.05;
    letter-spacing: -0.02em;
    color: #ffffff;
    white-space: nowrap;
    user-select: none;
  }

  .setup-rgb-wrap {
    position: absolute;
    left: 50%;
    top: calc(50% + 36px);
    translate: -50% 0;
    width: 248px;
    pointer-events: auto;
    -webkit-app-region: no-drag;
  }

  .setup-done-stage {
    position: absolute;
    inset: 0;
    z-index: 4;
    display: flex;
    align-items: center;
    justify-content: center;
    text-align: center;
    pointer-events: none;
  }

  .setup-done-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(30px, 4vw, 44px);
    font-weight: 700;
    line-height: 1.1;
    letter-spacing: -0.025em;
    color: #ffffff;
    margin: 0;
  }

  .setup-footer {
    position: absolute;
    left: 50%;
    bottom: 34px;
    z-index: 3;
    transform: translateX(-50%);
    height: 44px;
    display: grid;
    place-items: center;
    -webkit-app-region: no-drag;
    animation: setup-footer-in 680ms cubic-bezier(0.22, 1, 0.36, 1) 110ms both;
  }

  .setup-btn-slot {
    grid-area: 1 / 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .setup-footer :global(.setup-primary-btn) {
    min-width: 168px;
    height: 42px;
    padding: 0 30px;
    font-size: 15px;
    letter-spacing: -0.01em;
  }

  @keyframes setup-logo-in {
    from {
      opacity: 0;
      transform: translateY(10px) scale(0.95);
      filter: blur(8px);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
      filter: blur(0);
    }
  }

  @keyframes setup-footer-in {
    from {
      opacity: 0;
      transform: translate(-50%, 12px);
      filter: blur(6px);
    }
    to {
      opacity: 1;
      transform: translate(-50%, 0);
      filter: blur(0);
    }
  }

  @keyframes setup-glow-in {
    from {
      opacity: 0;
      transform: translate3d(-50%, -50%, 0) scale(0.85);
    }
    to {
      opacity: 1;
      transform: translate3d(-50%, -50%, 0) scale(1);
    }
  }
</style>
