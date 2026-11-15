import type { Component } from 'svelte';
import House from '@lucide/svelte/icons/house';
import Library from '@lucide/svelte/icons/library';
import Newspaper from '@lucide/svelte/icons/newspaper';
import Import from '@lucide/svelte/icons/import';

import Home from './home/Home.svelte';
import LibraryPage from './library/Library.svelte';
import News from './news/News.svelte';
import ImportPage from './import/Import.svelte';
import { createLazyModuleLoader } from '../api/providers/lazy';

export type PageId = 'home' | 'library' | 'news' | 'import' | 'anime';

export interface PageDef
{
  id: PageId;
  name: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  icon: Component<any>;
  component: Component;
}

// Single source of truth for top header tabs. Order = order of header tabs.
export const pages: PageDef[] = [
  { id: 'home', name: 'Home', icon: House, component: Home },
  { id: 'library', name: 'Library', icon: Library, component: LibraryPage },
  { id: 'news', name: 'News', icon: Newspaper, component: News },
  { id: 'import', name: 'Import', icon: Import, component: ImportPage }
];

export const loadAnimePage = createLazyModuleLoader(() => import('./anime/AnimePage.svelte').then((m) => m.default));

export const defaultPage: PageId = 'home';

export function getPage(aId: PageId): PageDef
{
  return pages.find((p) => p.id === aId) ?? pages[0];
}
