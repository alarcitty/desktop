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
    icon?: 'telegram' | 'discord' | 'github';
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
          { label: '@mila_watcher', href: 'https://t.me/mila_watcher', icon: 'telegram' },
          { label: 'Discord', href: 'https://discord.gg/kAM9gqMsEq', icon: 'discord' },
          { label: 'GitHub', href: 'https://github.com/mila-desktop/desktop', icon: 'github' },
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
                          {#if item.icon === 'telegram'}
                            <svg class="credits-social-icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                              <path d="M21.94 4.66c.24-1.08-.4-1.5-1.36-1.15L2.76 10.38c-1.2.47-1.18 1.14-.21 1.44l4.57 1.43 10.6-6.68c.5-.31.95-.14.58.19l-8.59 7.75-.32 4.74c.47 0 .68-.21.94-.46l2.26-2.19 4.7 3.47c.87.48 1.49.23 1.71-.8l3.94-14.61Z" />
                            </svg>
                          {:else if item.icon === 'discord'}
                            <svg class="credits-social-icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                              <path d="M20.32 4.37A19.8 19.8 0 0 0 15.42 2.85c-.21.38-.46.89-.63 1.29a18.27 18.27 0 0 0-5.58 0c-.17-.4-.42-.91-.63-1.29A19.74 19.74 0 0 0 3.68 4.37C.58 9.02-.26 13.56.16 18.03a19.9 19.9 0 0 0 6.01 3.04c.48-.66.91-1.36 1.28-2.1-.7-.26-1.37-.58-2-.96.17-.12.33-.25.49-.38 3.85 1.79 8.03 1.79 11.84 0 .16.13.32.26.49.38-.63.38-1.3.7-2 .96.37.74.8 1.44 1.28 2.1a19.84 19.84 0 0 0 6.02-3.04c.5-5.19-.85-9.69-3.25-13.66ZM8.02 15.27c-1.18 0-2.15-1.08-2.15-2.42 0-1.33.95-2.42 2.15-2.42s2.17 1.09 2.15 2.42c0 1.34-.95 2.42-2.15 2.42Zm7.96 0c-1.18 0-2.15-1.08-2.15-2.42 0-1.33.95-2.42 2.15-2.42s2.17 1.09 2.15 2.42c0 1.34-.95 2.42-2.15 2.42Z" />
                            </svg>
                          {:else if item.icon === 'github'}
                            <svg class="credits-social-icon" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                              <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.55 0-.27-.01-1.17-.02-2.13-3.2.7-3.88-1.36-3.88-1.36-.52-1.33-1.28-1.69-1.28-1.69-1.05-.71.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.74.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.19-3.09-.12-.29-.52-1.47.11-3.06 0 0 .97-.31 3.18 1.18a11.03 11.03 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.63 1.59.23 2.77.11 3.06.74.8 1.19 1.83 1.19 3.09 0 4.42-2.69 5.4-5.26 5.68.42.36.79 1.07.79 2.16 0 1.56-.01 2.82-.01 3.2 0 .31.21.67.8.55A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5Z" />
                            </svg>
                          {/if}
                          <span>{item.label}</span>
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
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 400;
    line-height: 1.38;
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.88));
    text-align: left;
  }

  .credits-social-icon {
    width: 14px;
    height: 14px;
    flex-shrink: 0;
    opacity: 0.78;
    transition: opacity 0.16s var(--ease-out);
  }

  .credits-link:hover .credits-social-icon {
    opacity: 1;
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
