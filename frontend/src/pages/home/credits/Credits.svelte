<script lang="ts">
  import { navigate } from '../../../app/router.svelte';
  import { openSearch } from '../../../app/search.svelte';
  import { onMount } from 'svelte';
  import { t } from '../../../app/i18n.svelte';
  import { invokeGetAppVersion } from '../../../app/bindings.svelte';

  interface Props {
    open: boolean;
  }

  interface CreditItem {
    label: string;
    href?: string;
    action?: () => void;
    muted?: boolean;
  }

  interface CreditGroup {
    title: string;
    items: CreditItem[];
  }

  let { open }: Props = $props();

  let drawerEl = $state<HTMLElement | null>(null);
  let revealRaf = 0;
  let appVersion = $state('');

  onMount(() => {
    void invokeGetAppVersion().then((value) => {
      appVersion = value;
    });
  });

  function scrollToSection(selector: string) {
    const vp = drawerEl?.closest('.marlakit-scroll-viewport') as HTMLElement | null;
    if (!vp) return;
    if (!selector) {
      vp.scrollTo({ top: 0, behavior: 'smooth' });
      return;
    }
    const target = vp.querySelector(selector) as HTMLElement | null;
    if (target) {
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }

  function openExternal(url: string) {
    window.open(url, '_blank', 'noopener,noreferrer');
  }

  const columns = $derived<CreditGroup[][]>([
    [
      {
        title: t('home.explore'),
        items: [
          { label: t('nav.home'), action: () => scrollToSection('') },
          { label: t('home.seasonalReleases'), action: () => scrollToSection('.releases-section') },
          { label: t('home.recommendations'), action: () => scrollToSection('.recommendations-section') },
          { label: t('nav.library'), action: () => navigate('library') },
          { label: t('nav.news'), action: () => navigate('news') },
          { label: t('home.search'), action: () => openSearch({ filters: true }) }
        ]
      }
    ],
    [
      {
        title: t('home.dataApis'),
        items: [
          { label: 'Shikimori', href: 'https://shikimori.one' },
          { label: 'AniLiberty', href: 'https://aniliberty.top' },
          { label: 'AnimeGo', href: 'https://animego.me' },
          { label: 'Jikan', href: 'https://jikan.moe' },
          { label: 'AniSkip', href: 'https://aniskip.com' },
          { label: 'Nyaa', href: 'https://nyaa.si' },
          { label: 'Kodik' }
        ]
      }
    ],
    [
      {
        title: t('home.libraries'),
        items: [
          { label: 'hls.js', href: 'https://github.com/video-dev/hls.js' },
          { label: 'anacrolix/torrent', href: 'https://github.com/anacrolix/torrent' },
          { label: 'FFmpeg', href: 'https://ffmpeg.org' },
          { label: 'Lucide', href: 'https://lucide.dev' }
        ]
      },
      {
        title: t('home.typography'),
        items: [
          { label: 'PP Monument Extended' },
          { label: 'RF Dewi Extended' },
          { label: 'Medeo Sans Display' }
        ]
      }
    ],
    [
      {
        title: t('home.socials'),
        items: [
          // Replace with project links when ready
          { label: 'Telegram', href: 'https://t.me' },
          { label: 'Discord', href: 'https://discord.com' },
          { label: 'GitHub', href: 'https://github.com' },
          ...(appVersion ? [{ label: `Mila ${appVersion}`, muted: true }] : [])
        ]
      }
    ]
  ]);

  $effect(() => {
    cancelAnimationFrame(revealRaf);
    if (!open || !drawerEl) return;

    const vp = drawerEl.closest('.marlakit-scroll-viewport') as HTMLElement | null;
    if (!vp) return;

    let cancelled = false;
    const cancel = () => {
      cancelled = true;
      cancelAnimationFrame(revealRaf);
    };
    vp.addEventListener('wheel', cancel, { passive: true, once: true });
    vp.addEventListener('mousedown', cancel, { passive: true, once: true });

    const startTop = vp.scrollTop;
    const startTs = performance.now();
    const durationMs = 560;

    const step = (now: number) => {
      if (cancelled || !drawerEl) return;
      const elapsed = now - startTs;
      const t = Math.min(1, elapsed / durationMs);
      const ease = 1 - Math.pow(1 - t, 3);
      const maxTop = Math.max(0, vp.scrollHeight - vp.clientHeight);
      if (maxTop > startTop) {
        vp.scrollTop = startTop + (maxTop - startTop) * ease;
      }
      if (t < 1) {
        revealRaf = requestAnimationFrame(step);
      } else {
        vp.removeEventListener('wheel', cancel);
        vp.removeEventListener('mousedown', cancel);
      }
    };

    revealRaf = requestAnimationFrame(step);
    return () => {
      cancelled = true;
      cancelAnimationFrame(revealRaf);
      vp.removeEventListener('wheel', cancel);
      vp.removeEventListener('mousedown', cancel);
    };
  });
</script>

<footer
  bind:this={drawerEl}
  class="credits-drawer"
  class:is-open={open}
  aria-hidden={!open}
>
  <div class="credits-clip">
    <div class="credits-inner">
      <div class="credits-grid">
        {#each columns as col, colIdx (colIdx)}
          <div class="credits-col">
            {#each col as group (group.title)}
              <section class="credits-group">
                <h3 class="credits-heading">{group.title}</h3>
                <ul class="credits-list">
                  {#each group.items as item (item.label)}
                    <li class="credits-row">
                      {#if item.href}
                        <button
                          type="button"
                          class="credits-link"
                          onclick={() => openExternal(item.href!)}
                        >
                          {item.label}
                        </button>
                      {:else if item.action}
                        <button
                          type="button"
                          class="credits-link"
                          onclick={item.action}
                        >
                          {item.label}
                        </button>
                      {:else}
                        <span class="credits-label" class:is-muted={item.muted}>{item.label}</span>
                      {/if}
                    </li>
                  {/each}
                </ul>
              </section>
            {/each}
          </div>
        {/each}
      </div>
    </div>
  </div>
</footer>

<style>
  .credits-drawer {
    display: grid;
    grid-template-rows: 0fr;
    opacity: 0;
    width: 100%;
    background: var(--mila-app-canvas, #0c0c0c);
    overflow: hidden;
    transition:
      background-color 360ms cubic-bezier(0.22, 1, 0.36, 1),
      grid-template-rows 0.56s cubic-bezier(0.22, 1, 0.36, 1),
      opacity 0.42s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .credits-drawer.is-open {
    grid-template-rows: 1fr;
    opacity: 1;
  }

  .credits-clip {
    min-height: 0;
    overflow: hidden;
  }

  .credits-inner {
    width: 100%;
    max-width: 1280px;
    margin: 0 auto;
    padding: clamp(44px, 5vw, 72px) var(--page-pad-x) clamp(48px, 5.5vw, 76px);
    box-sizing: border-box;
    transform: translateY(-10px);
    transition: transform 0.52s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .credits-drawer.is-open .credits-inner {
    transform: translateY(0);
  }

  .credits-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: clamp(28px, 4vw, 56px) clamp(20px, 3vw, 48px);
  }

  .credits-col {
    display: flex;
    flex-direction: column;
    gap: clamp(28px, 3.6vw, 42px);
    min-width: 0;
  }

  .credits-group {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .credits-heading {
    margin: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 400;
    line-height: 1.3;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.42));
    user-select: none;
  }

  .credits-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 11px;
  }

  .credits-row {
    margin: 0;
    padding: 0;
    line-height: 1.35;
  }

  .credits-link,
  .credits-label {
    display: inline-block;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 400;
    line-height: 1.38;
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.88));
    text-align: left;
  }

  .credits-link {
    margin: 0;
    padding: 0;
    border: none;
    outline: none;
    background: transparent;
    cursor: pointer;
    transition: color 0.16s var(--ease-out);
  }

  .credits-link:hover {
    color: var(--mila-text-primary, #ffffff);
  }

  .credits-label {
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.78));
    user-select: none;
  }

  .credits-label.is-muted {
    font-size: 12px;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.3));
  }

  @media (max-width: 960px) {
    .credits-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
