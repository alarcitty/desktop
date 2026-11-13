import type { PageId } from '../pages';

const DEFAULT_PAGE: PageId = 'home';

// Reactive page router with return-stack support for detail pages.
export const router = $state<{ current: PageId; previous: PageId }>({
  current: DEFAULT_PAGE,
  previous: DEFAULT_PAGE
});

export function navigate(aId: PageId): void
{
  if (router.current === aId)
  {
    return;
  }
  if (router.current !== 'anime')
  {
    router.previous = router.current;
  }
  router.current = aId;
}

export function goBack(): void
{
  const target = router.previous && router.previous !== 'anime' ? router.previous : DEFAULT_PAGE;
  router.current = target;
}
