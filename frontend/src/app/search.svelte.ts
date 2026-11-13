// Shared state for the global search popup.
// `open`   - target state (what the user asked for)
// `active` - popup is visible or still animating (header input stays hidden)
export const search = $state({ open: false, active: false, query: '' });

export interface AnchorRect
{
  left: number;
  top: number;
  width: number;
  height: number;
}

// plain (non-reactive) reference to the header search field
let anchorEl: HTMLElement | null = null;

export function setSearchAnchor(aEl: HTMLElement | null): void
{
  anchorEl = aEl;
}

export function getSearchAnchorRect(): AnchorRect | null
{
  if (!anchorEl)
  {
    return null;
  }
  const r = anchorEl.getBoundingClientRect();
  return { left: r.left, top: r.top, width: r.width, height: r.height };
}

export function openSearch(): void
{
  if (search.open)
  {
    return;
  }
  search.open = true;
  search.active = true;
}

export function closeSearch(): void
{
  search.open = false;
}

// called by the popup once the close animation is finished
export function finishSearchClose(): void
{
  search.active = false;
  search.query = '';
}
