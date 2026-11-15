/**
 * Teleports a fixed overlay node into `.viewport` (fallback `document.body`)
 * so `position: fixed; inset: 0` centers against the live viewport even when
 * page ancestors apply `transform` or `contain`.
 */
export function portal(node: HTMLElement) {
  const host =
    (typeof document !== 'undefined' ? document.querySelector('.viewport') : null) ??
    (typeof document !== 'undefined' ? document.body : null);
  if (host && node.parentElement !== host) {
    host.appendChild(node);
  }
  return {
    destroy() {
      if (node.parentNode) {
        node.parentNode.removeChild(node);
      }
    }
  };
}
