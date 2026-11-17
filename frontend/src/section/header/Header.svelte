<script lang="ts">
  import Search from "@lucide/svelte/icons/search";
  import SlidersHorizontal from "@lucide/svelte/icons/sliders-horizontal";
  import { cubicOut } from "svelte/easing";
  import MilaLogo from "../../themes/MilaLogo.svelte";
  import Input from "../../marlakit/input/Input.svelte";
  import SocialButton from "../../marlakit/button/SocialButton.svelte";
  import GitButton from "../../marlakit/button/GitButton.svelte";
  import ComboBox from "../../marlakit/combo/ComboBox.svelte";
  import LayerBlur from "../../marlakit/blur/LayerBlur.svelte";
  import type { ComboOption, ComboSection } from "../../marlakit/combo/types";
  import { pages } from "../../pages";
  import { router, navigate } from "../../app/router.svelte";
  import { search, openSearch, setSearchAnchor } from "../../app/search.svelte";
  import { auth, initAuth, loginWithGoogle, logout } from "../../app/auth.svelte";
  import {
    invokeCheckAndInstallUpdate,
    subscribeAppUpdateStatus,
    type AppUpdateStatus
  } from "../../app/bindings.svelte";
  import {
    i18n,
    isLocaleCode,
    setLocale,
    SUPPORTED_LOCALES,
    t
  } from "../../app/i18n.svelte";

  interface Props {
    hidden?: boolean;
    scrolled?: boolean;
  }

  let { hidden = false, scrolled = false }: Props = $props();
  const isHidden = $derived(hidden && !search.active);

  let searchWrap = $state<HTMLElement | null>(null);

  // the popup morphs out of (and back into) this element
  $effect(() => {
    setSearchAnchor(searchWrap);
  });

  // first typed letter opens the search popup
  function handleSearchInput() {
    if (search.query.trim().length > 0) {
      openSearch();
    }
  }

  let avatarBroken = $state(false);
  let menuOpen = $state(false);
  let updateChecking = $state(false);
  let updateProgress = $state<number | null>(null);

  $effect(() => {
    void initAuth();
  });

  $effect(() => {
    return subscribeAppUpdateStatus((status: AppUpdateStatus) => {
      if (status.state === "downloading" || status.state === "installing") {
        updateChecking = false;
        updateProgress = typeof status.progress === "number" ? status.progress : 0;
        return;
      }
      if (status.state === "up_to_date" || status.state === "error" || status.state === "idle") {
        updateChecking = false;
        updateProgress = null;
      }
    });
  });

  $effect(() => {
    if (auth.user?.avatar_url) {
      avatarBroken = false;
    }
    if (!auth.user) {
      menuOpen = false;
    }
  });

  function handleGoogleLogin() {
    void loginWithGoogle();
  }

  async function handleCheckReleases() {
    if (updateChecking || updateProgress !== null) return;
    updateChecking = true;
    updateProgress = null;
    try {
      const res = await invokeCheckAndInstallUpdate();
      if (!res) {
        await fetch("https://api.github.com/repos/mila-desktop/desktop/releases/latest", {
          headers: { Accept: "application/vnd.github+json" }
        }).catch(() => {});
        updateChecking = false;
        updateProgress = null;
        return;
      }
      if (res.state === "downloading" || res.state === "installing") {
        updateChecking = false;
        updateProgress = typeof res.progress === "number" ? res.progress : 0;
        return;
      }
      updateChecking = false;
      updateProgress = null;
    } catch {
      updateChecking = false;
      updateProgress = null;
    }
  }

  function handleAccountSelect(opt: ComboOption) {
    const val = String(opt.value);
    if (val.startsWith("lang:")) {
      const code = val.slice("lang:".length);
      if (isLocaleCode(code)) {
        setLocale(code);
      }
      return;
    }
    if (val === "logout") {
      menuOpen = false;
      void logout();
    }
  }

  const navLabels: Record<string, string> = {
    home: 'nav.home',
    library: 'nav.library',
    news: 'nav.news',
    'watch-together': 'nav.watchTogether'
  };

  const userDisplayName = $derived(
    auth.user?.name?.trim() || auth.user?.email?.split("@")[0] || t("header.account")
  );
  const userInitial = $derived(userDisplayName.slice(0, 1).toUpperCase());
  const accountSections = $derived<ComboSection[]>([
    {
      id: "language",
      title: t("header.language"),
      options: SUPPORTED_LOCALES.map((loc) => ({
        value: `lang:${loc.code}`,
        label: loc.nativeLabel,
        selected: i18n.locale === loc.code,
        keepOpen: true
      }))
    },
    {
      id: "account",
      title: auth.user?.email || t("header.account"),
      options: [{ value: "logout", label: t("header.signOut"), disabled: auth.loading }]
    }
  ]);

  function revealBlur(
    _node: HTMLElement,
    params: { duration?: number; delay?: number; x?: number; scale?: number; blur?: number } = {}
  ) {
    const duration = params.duration ?? 380;
    const delay = params.delay ?? 0;
    const x = params.x ?? 0;
    const scale = params.scale ?? 1;
    const blur = params.blur ?? 5;
    return {
      duration,
      delay,
      easing: cubicOut,
      css: (t: number) => {
        const inv = 1 - t;
        const curX = inv * x;
        const curScale = scale + (1 - scale) * t;
        const curBlur = inv * blur;
        return `opacity: ${t}; transform: translate3d(${curX}px, 0, 0) scale(${curScale}); filter: blur(${curBlur}px);`;
      }
    };
  }
</script>

<header class="header" class:is-hidden={isHidden} class:is-scrolled={scrolled}>
  {#if auth.user}
    <div
      class="header-avatar-ambient"
      aria-hidden="true"
      in:revealBlur={{ duration: 600, delay: 180, blur: 0 }}
      out:revealBlur={{ duration: 240, blur: 0 }}
    >
      {#if auth.user.avatar_url && !avatarBroken}
        <LayerBlur src={auth.user.avatar_url} blur={88} opacity={0.68} scale={1.25} />
      {:else}
        <div class="header-avatar-ambient-fallback"></div>
      {/if}
    </div>
  {/if}
  <div class="header-left">
    <div class="logo-wrap">
      <MilaLogo class="logo-img" />
    </div>

    <!-- marlakit Input component (hidden while the search popup is morphing out of it) -->
    <div class="search-anchor" class:is-hidden={search.active} bind:this={searchWrap}>
      <Input
        bind:value={search.query}
        oninput={handleSearchInput}
        placeholder={t("header.searchPlaceholder")}
      >
        {#snippet icon()}
          <Search size={16} strokeWidth={1.8} />
        {/snippet}
        {#snippet trailingIcon()}
          <button
            type="button"
            class="search-filter-btn"
            aria-label={t("header.openFilters")}
            title={t("header.filters")}
            onclick={(e) => {
              e.stopPropagation();
              openSearch({ filters: true });
            }}
          >
            <SlidersHorizontal size={15} strokeWidth={1.8} />
          </button>
        {/snippet}
      </Input>
    </div>
  </div>

  <div class="header-right">
    <!-- Nav tabs -->
    <nav class="nav-tabs">
      {#each pages as tab (tab.id)}
        {@const TabIcon = tab.icon}
        {@const tabTitle = t(navLabels[tab.id] ?? tab.name)}
        <button
          type="button"
          class="tab-btn"
          class:active={router.current === tab.id}
          onclick={() => navigate(tab.id)}
          title={tabTitle}
          aria-label={tabTitle}
        >
          <TabIcon size={18} strokeWidth={1.7} />
          <span class="tab-name">{tabTitle}</span>
        </button>
      {/each}
    </nav>

    <div class="auth-slot">
      {#if auth.user}
        <div
          class="auth-slot-item"
          out:revealBlur={{ duration: 200, scale: 0.96, blur: 4 }}
        >
          <ComboBox
            bind:open={menuOpen}
            sections={accountSections}
            placement="bottom-right"
            minWidth={210}
            maxHeight={340}
            class="auth-profile"
            menuClass="auth-combo-menu"
            onselect={handleAccountSelect}
          >
            {#snippet trigger({ open, toggle })}
              <button
                type="button"
                class="auth-trigger"
                class:is-open={open}
                aria-haspopup="listbox"
                aria-expanded={open}
                onclick={toggle}
              >
                <span
                  class="auth-user-name"
                  in:revealBlur|global={{ duration: 420, delay: 160, x: 10, blur: 5 }}
                >
                  {userDisplayName}
                </span>
                {#if auth.user?.avatar_url && !avatarBroken}
                  <img
                    class="auth-avatar"
                    src={auth.user.avatar_url}
                    alt={userDisplayName}
                    referrerpolicy="no-referrer"
                    onerror={() => (avatarBroken = true)}
                    in:revealBlur|global={{ duration: 460, delay: 240, scale: 0.86, blur: 6 }}
                  />
                {:else}
                  <span
                    class="auth-avatar auth-avatar-fallback"
                    in:revealBlur|global={{ duration: 460, delay: 240, scale: 0.86, blur: 6 }}
                  >
                    {userInitial}
                  </span>
                {/if}
              </button>
            {/snippet}
            {#snippet footer()}
              <GitButton
                fullWidth
                loading={updateChecking}
                progress={updateProgress}
                onclick={() => void handleCheckReleases()}
              >
                {t("header.checkReleases")}
              </GitButton>
            {/snippet}
          </ComboBox>
        </div>
      {:else}
        <div
          class="auth-slot-item"
          in:revealBlur={{ duration: 280, delay: 100, scale: 0.96, blur: 4 }}
          out:revealBlur={{ duration: 240, scale: 0.94, blur: 5 }}
        >
          <!-- marlakit SocialButton component with loading state on click -->
          <SocialButton loading={auth.loading} onclick={handleGoogleLogin}>
            {#snippet icon()}
              <svg class="google-icon" width="18" height="18" viewBox="0 0 18 18">
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
            {t("header.signInGoogle")}
          </SocialButton>
        </div>
      {/if}
    </div>
  </div>
</header>

<style>
  .header {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    width: 100%;
    height: 72px;
    background: transparent;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 var(--page-pad-x);
    box-sizing: border-box;
    --wails-draggable: drag;
    user-select: none;
    z-index: 50;
    transition:
      top 0.34s cubic-bezier(0.22, 1, 0.36, 1),
      opacity 0.24s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .header::before {
    content: "";
    position: absolute;
    inset: 0 0 -18px 0;
    background: linear-gradient(
      180deg,
      var(--mila-header-fadeTop, rgba(12, 12, 12, 0.86)) 0%,
      var(--mila-header-fadeMid, rgba(12, 12, 12, 0.56)) 62%,
      var(--mila-header-fadeBottom, rgba(12, 12, 12, 0)) 100%
    );
    opacity: var(--mila-header-baseFadeOpacity, 0);
    pointer-events: none;
    z-index: -1;
    transition: opacity 0.26s ease;
  }

  .header.is-scrolled::before {
    opacity: 1;
  }

  .header.is-hidden {
    top: -76px;
    opacity: 0;
    pointer-events: none;
    --wails-draggable: no-drag;
  }

  .header-left {
    position: relative;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 16px;
    min-width: 0;
  }

  .search-anchor {
    display: flex;
    --wails-draggable: no-drag;
  }

  .search-anchor.is-hidden {
    visibility: hidden;
  }

  .search-filter-btn {
    width: 24px;
    height: 24px;
    margin: 0 -2px 0 0;
    padding: 0;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--mila-inputs-icon, rgba(255, 255, 255, 0.42));
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition:
      color 0.16s var(--ease-out),
      background-color 0.16s var(--ease-out),
      transform 0.1s ease;
  }

  .search-filter-btn:hover {
    color: var(--mila-text-primary, #ffffff);
    background: transparent;
  }

  .search-filter-btn:active {
    transform: scale(0.92);
  }

  .logo-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 66px;
  }

  :global(.logo-img) {
    height: 52px;
    width: auto;
    display: block;
    pointer-events: none;
  }

  .header-right {
    position: relative;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 24px;
  }

  .nav-tabs {
    display: flex;
    align-items: center;
    gap: 6px;
    --wails-draggable: no-drag;
  }

  .tab-btn {
    position: relative;
    background: transparent;
    border: none;
    padding: 8px 12px;
    margin: 0;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    color: var(--mila-header-navText, rgba(255, 255, 255, 0.5));
    font-family: 'Medeo Sans', system-ui, sans-serif;
    font-size: 14px;
    line-height: 1;
    white-space: nowrap;
    transition:
      color var(--hover-dur) var(--ease-out),
      transform 0.1s ease;
    border-radius: 10px;
  }

  .tab-btn:hover {
    color: var(--mila-header-navTextHover, rgba(255, 255, 255, 0.85));
  }

  .tab-btn.active {
    color: var(--mila-header-navTextActive, #ffffff);
  }

  .tab-btn:active {
    transform: scale(0.92);
  }

  .google-icon {
    width: 17px;
    height: 17px;
    flex-shrink: 0;
  }

  .header-avatar-ambient {
    position: absolute;
    top: 0;
    right: 0;
    width: 520px;
    height: 220px;
    pointer-events: none;
    user-select: none;
    z-index: 0;
    overflow: hidden;
    -webkit-mask-image: radial-gradient(
      ellipse 94% 94% at 100% 0%,
      rgba(0, 0, 0, 1) 0%,
      rgba(0, 0, 0, 0.72) 22%,
      rgba(0, 0, 0, 0.36) 44%,
      rgba(0, 0, 0, 0.12) 66%,
      rgba(0, 0, 0, 0) 86%
    );
    mask-image: radial-gradient(
      ellipse 94% 94% at 100% 0%,
      rgba(0, 0, 0, 1) 0%,
      rgba(0, 0, 0, 0.72) 22%,
      rgba(0, 0, 0, 0.36) 44%,
      rgba(0, 0, 0, 0.12) 66%,
      rgba(0, 0, 0, 0) 86%
    );
  }

  .header-avatar-ambient-fallback {
    width: 100%;
    height: 100%;
    background: radial-gradient(
      ellipse at 92% 10%,
      rgba(166, 227, 161, 0.32) 0%,
      rgba(137, 220, 235, 0.16) 42%,
      rgba(12, 12, 12, 0) 78%
    );
  }

  .auth-slot {
    display: inline-grid;
    align-items: center;
    justify-items: end;
    --wails-draggable: no-drag;
  }

  .auth-slot-item {
    grid-area: 1 / 1;
    display: inline-flex;
    align-items: center;
  }

  :global(.auth-profile) {
    --wails-draggable: no-drag;
  }

  :global(.auth-combo-menu) {
    top: 40px !important;
  }

  .auth-trigger {
    display: inline-flex;
    align-items: center;
    gap: 11px;
    padding: 0;
    margin: 0;
    background: transparent;
    border: none;
    cursor: pointer;
    color: var(--mila-text-primary, rgba(255, 255, 255, 0.92));
    transition: color var(--hover-dur) var(--ease-out);
  }

  .auth-trigger:hover,
  .auth-trigger.is-open {
    color: var(--mila-header-navTextActive, #ffffff);
  }

  .auth-user-name {
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 700;
    line-height: 1;
    letter-spacing: -0.015em;
  }

  .auth-avatar {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
  }

  .auth-avatar-fallback {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--mila-header-avatarBg, rgba(255, 255, 255, 0.12));
    color: var(--mila-header-avatarText, #fff);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 700;
  }

  @media (max-width: 1140px) {
    .search-anchor :global(.marlakit-input-wrap) {
      width: clamp(200px, 24vw, 290px);
    }

    .header-left {
      gap: 12px;
    }

    .header-right {
      gap: 16px;
    }
  }

  @media (max-width: 980px) {
    :global(.logo-img) {
      height: 44px;
    }

    .search-anchor :global(.marlakit-input-wrap) {
      width: clamp(170px, 21vw, 230px);
      height: 36px;
    }

    .header-right {
      gap: 12px;
    }

    .nav-tabs {
      gap: 2px;
    }

    .tab-btn {
      padding: 7px 9px;
      gap: 5px;
      font-size: 13px;
    }

    .auth-slot :global(.marlakit-social-btn) {
      height: 36px;
      padding: 0 12px;
      font-size: 13px;
      gap: 8px;
      white-space: nowrap;
    }

    .auth-avatar {
      width: 28px;
      height: 28px;
    }

    .auth-user-name {
      max-width: 110px;
      font-size: 12px;
    }
  }
</style>
