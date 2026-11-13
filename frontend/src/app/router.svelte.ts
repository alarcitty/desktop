import { defaultPage, type PageId } from '../pages';

// Minimal reactive page router (no URLs needed for a desktop app).
export const router = $state<{ current: PageId }>({ current: defaultPage });

export function navigate(aId: PageId): void
{
  router.current = aId;
}
