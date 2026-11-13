<script lang="ts">
  import Search from "@lucide/svelte/icons/search";
  import logoSrc from "../../assets/mila_header.svg";
  import Input from "../../marlakit/input/Input.svelte";
  import SocialButton from "../../marlakit/button/SocialButton.svelte";
  import { pages } from "../../pages";
  import { router, navigate } from "../../app/router.svelte";
  import { search, openSearch, setSearchAnchor } from "../../app/search.svelte";

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

  // loading state for google button (triggers on click)
  let isGoogleLoading = $state(false);

  function handleGoogleLogin() {
    isGoogleLoading = true;
    setTimeout(() => {
      isGoogleLoading = false;
    }, 2500);
  }
</script>

<header class="header">
  <div class="header-left">
    <div class="logo-wrap">
      <img src={logoSrc} alt="mila" class="logo-img" />
    </div>

    <!-- marlakit Input component (hidden while the search popup is morphing out of it) -->
    <div class="search-anchor" class:is-hidden={search.active} bind:this={searchWrap}>
      <Input
        bind:value={search.query}
        oninput={handleSearchInput}
        placeholder="Search something..."
      >
        {#snippet icon()}
          <Search size={16} strokeWidth={1.8} />
        {/snippet}
      </Input>
    </div>
  </div>

  <div class="header-right">
    <!-- Nav tabs -->
    <nav class="nav-tabs">
      {#each pages as tab (tab.id)}
        {@const TabIcon = tab.icon}
        <button
          type="button"
          class="tab-btn"
          class:active={router.current === tab.id}
          onclick={() => navigate(tab.id)}
          title={tab.name}
          aria-label={tab.name}
        >
          <TabIcon size={18} strokeWidth={1.7} />
          <span class="tab-name">{tab.name}</span>
        </button>
      {/each}
    </nav>

    <!-- marlakit SocialButton component with loading state on click -->
    <SocialButton loading={isGoogleLoading} onclick={handleGoogleLogin}>
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
      Continue with Google
    </SocialButton>
  </div>
</header>

<style>
  .header {
    position: sticky;
    top: 0;
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
    flex-shrink: 0;
    z-index: 50;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .search-anchor {
    display: flex;
    --wails-draggable: no-drag;
  }

  .search-anchor.is-hidden {
    visibility: hidden;
  }

  .logo-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 66px;
  }

  .logo-img {
    height: 52px;
    width: auto;
    display: block;
    pointer-events: none;
  }

  .header-right {
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
    color: rgba(255, 255, 255, 0.5);
    font-family: 'Medeo Sans', system-ui, sans-serif;
    font-size: 14px;
    line-height: 1;
    transition:
      color var(--hover-dur) var(--ease-out),
      transform 0.1s ease;
    border-radius: 10px;
  }

  .tab-btn:hover {
    color: rgba(255, 255, 255, 0.85);
  }

  .tab-btn.active {
    color: #ffffff;
  }

  .tab-btn:active {
    transform: scale(0.92);
  }

  .google-icon {
    width: 17px;
    height: 17px;
    flex-shrink: 0;
  }
</style>
