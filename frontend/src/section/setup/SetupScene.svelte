<script lang="ts">
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import MainButton from '../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../marlakit/button/SecondButton.svelte';
  import SocialButton from '../../marlakit/button/SocialButton.svelte';
  import RgbLine from '../../marlakit/slider/RgbLine.svelte';
  import MilaLogo from '../../themes/MilaLogo.svelte';
  import { auth, initAuth, loginWithGoogle } from '../../app/auth.svelte';
  import { i18n, setLocale, SUPPORTED_LOCALES, t } from '../../app/i18n.svelte';
  import {
    setCustomHue,
    setThemeMode,
    SUPPORTED_THEMES,
    theme,
    type ThemeMode
  } from '../../app/theme.svelte';

  type SetupStep = 'welcome' | 'question' | 'done' | 'outro';
  type QualityPref = '1080p' | '720p' | 'auto';

  interface Props {
    ready?: boolean;
    onContinue: () => void;
  }

  let { ready = true, onContinue }: Props = $props();

  const QUALITY_KEY = 'mila.setup.quality';
  const qualityValues: QualityPref[] = ['1080p', '720p', 'auto'];

  function readStorage<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
    try {
      const saved = window.localStorage.getItem(key) as T | null;
      if (saved && allowed.includes(saved)) return saved;
    } catch {
      // Ignore restricted storage contexts
    }
    return fallback;
  }

  let step = $state<SetupStep>('welcome');
  let qIndex = $state(0);
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
        title: t('setup.chooseLanguage'),
        subtitle: t('setup.chooseLanguageSub'),
        ariaLabel: t('setup.chooseLanguage'),
        valueKey: i18n.locale,
        valueLabel:
          SUPPORTED_LOCALES.find((o) => o.code === i18n.locale)?.nativeLabel ?? 'English'
      };
    }
    if (qIndex === 1) {
      const themeLabelMap: Record<ThemeMode, string> = {
        dark: t('setup.themeDark'),
        light: t('setup.themeLight'),
        custom: t('setup.themeCustom')
      };
      return {
        title: t('setup.chooseTheme'),
        subtitle: t('setup.chooseThemeSub'),
        ariaLabel: t('setup.chooseTheme'),
        valueKey: `${i18n.locale}-${theme.mode}`,
        valueLabel: themeLabelMap[theme.mode]
      };
    }
    const qualityLabelMap: Record<QualityPref, string> = {
      '1080p': '1080p',
      '720p': '720p',
      auto: t('setup.qualityAuto')
    };
    return {
      title: t('setup.defaultQuality'),
      subtitle: t('setup.defaultQualitySub'),
      ariaLabel: t('setup.defaultQuality'),
      valueKey: `${i18n.locale}-${selectedQuality}`,
      valueLabel: qualityLabelMap[selectedQuality]
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

  function handleSkip() {
    if (ready) {
      onContinue();
      return;
    }
    pendingContinue = true;
  }

  function handleGoogleSignIn() {
    void loginWithGoogle();
  }

  $effect(() => {
    void initAuth();
  });

  function cycleCurrent(delta: 1 | -1) {
    slideDir = delta;
    if (qIndex === 0) {
      const idx = SUPPORTED_LOCALES.findIndex((o) => o.code === i18n.locale);
      const next =
        SUPPORTED_LOCALES[(idx + delta + SUPPORTED_LOCALES.length) % SUPPORTED_LOCALES.length].code;
      setLocale(next);
      return;
    }
    if (qIndex === 1) {
      const idx = SUPPORTED_THEMES.indexOf(theme.mode);
      const next = SUPPORTED_THEMES[(idx + delta + SUPPORTED_THEMES.length) % SUPPORTED_THEMES.length];
      setThemeMode(next);
      return;
    }
    const idx = qualityValues.indexOf(selectedQuality);
    const next = qualityValues[(idx + delta + qualityValues.length) % qualityValues.length];
    selectedQuality = next;
    saveStorage(QUALITY_KEY, next);
  }

  function handleHueChange(nextHue: number) {
    setCustomHue(nextHue);
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
        <MilaLogo class="setup-logo" />
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

          {#if qIndex === 1 && theme.mode === 'custom'}
            <div
              class="setup-rgb-wrap"
              in:revealBlur={{ duration: 320, y: 8, scale: 0.95, blur: 6 }}
              out:revealBlur={{ duration: 220, y: 6, scale: 0.95, blur: 5 }}
            >
              <RgbLine hue={theme.hue} onchange={handleHueChange} />
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
      <h1 class="setup-done-title">{t('setup.allDone')}</h1>
    </div>
  {/if}

  <div class="setup-footer">
    {#if step === 'welcome'}
      <div
        class="setup-btn-slot setup-welcome-actions"
        out:revealBlur={{ duration: 240, y: 8, scale: 0.95, blur: 6 }}
      >
        <SecondButton class="setup-skip-btn" loading={pendingContinue && !ready} onclick={handleSkip}>
          {t('setup.skip')}
        </SecondButton>
        <MainButton class="setup-primary-btn" onclick={handleWelcome}>
          {t('setup.welcome')}
        </MainButton>
        {#if !auth.user}
          <SocialButton class="setup-google-btn" loading={auth.loading} onclick={handleGoogleSignIn}>
            {#snippet icon()}
              <svg width="17" height="17" viewBox="0 0 18 18" aria-hidden="true">
                <path
                  fill="#4285F4"
                  d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.616z"
                />
                <path
                  fill="#34A853"
                  d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332C2.438 15.983 5.482 18 9 18z"
                />
                <path
                  fill="#FBBC05"
                  d="M3.964 10.707c-.18-.54-.282-1.117-.282-1.707s.102-1.167.282-1.707V4.961H.957C.347 6.175 0 7.55 0 9s.347 2.825.957 4.039l3.007-2.332z"
                />
                <path
                  fill="#EA4335"
                  d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0 5.482 0 2.438 2.017.957 4.961L3.964 7.293C4.672 5.166 6.656 3.58 9 3.58z"
                />
              </svg>
            {/snippet}
            {t('header.signInGoogle')}
          </SocialButton>
        {/if}
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
          {t('setup.continue')}
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
    background: var(--mila-setup-cardBg, #0c0c0c);
    overflow: hidden;
    -webkit-app-region: drag;
    transition:
      width 760ms cubic-bezier(0.22, 1, 0.36, 1),
      height 760ms cubic-bezier(0.22, 1, 0.36, 1),
      border-radius 760ms cubic-bezier(0.22, 1, 0.36, 1),
      background-color 360ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .setup-scene.is-expanded {
    width: 100%;
    height: 100%;
  }

  .setup-ambient {
    position: absolute;
    left: 50%;
    top: 44%;
    width: 420px;
    height: 220px;
    transform: translate3d(-50%, -50%, 0) scale(1);
    background: radial-gradient(
      ellipse at center,
      var(--mila-setup-ambientGlow, rgba(255, 255, 255, 0.06)) 0%,
      var(--mila-setup-ambientGlowSoft, rgba(255, 255, 255, 0.015)) 48%,
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

  :global(.setup-logo) {
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
    color: var(--mila-text-primary, #ffffff);
    margin: 0;
    transition: color 280ms ease;
  }

  .setup-question-sub {
    max-width: 420px;
    margin-top: 10px;
    font-family: 'Medeo Sans Display', 'Medeo Sans', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 400;
    line-height: 1.42;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.46));
    transition: color 280ms ease;
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
    color: var(--mila-setup-chevron, rgba(255, 255, 255, 0.46));
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
    color: var(--mila-setup-chevronHover, #ffffff);
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
    color: var(--mila-setup-valueText, #ffffff);
    white-space: nowrap;
    user-select: none;
    transition: color 260ms ease;
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
    color: var(--mila-text-primary, #ffffff);
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
    gap: 10px;
    flex-wrap: wrap;
  }

  .setup-footer :global(.setup-primary-btn) {
    min-width: 156px;
    height: 42px;
    padding: 0 26px;
    font-size: 15px;
    letter-spacing: -0.01em;
  }

  .setup-footer :global(.setup-skip-btn) {
    height: 42px;
    padding: 0 20px;
    font-size: 14px;
    border-radius: 21px;
  }

  .setup-footer :global(.setup-google-btn) {
    height: 42px;
    padding: 0 18px;
    border-radius: 21px;
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
