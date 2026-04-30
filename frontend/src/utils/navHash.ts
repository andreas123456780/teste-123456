// navHrefForHash rewrites in-page anchor links so they work from any
// route. On the home page (`/`) it keeps the bare hash so the
// browser's native scroll-to-anchor still fires without a reload.
// From any other route (`/login`, `/cadastro`, `/minha-conta`, etc.)
// it expands to `/#anchor` so the click navigates to the home page
// and the anchor still resolves once it loads. Without this, clicking
// a header/footer nav anchor on /cadastro just appends `#products` to
// the current URL with no effect because the section doesn't exist
// on that page.
export function navHrefForHash(hash: string): string {
  // Absolute URLs and bare paths (e.g. "/sobre") pass through
  // untouched — they already navigate to the right place from any
  // route.
  if (typeof window === "undefined") return hash;
  if (!hash.startsWith("#")) return hash;
  const path = window.location.pathname;
  if (path === "/" || path === "") return hash;
  return `/${hash}`;
}
