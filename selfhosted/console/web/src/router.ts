import { useSyncExternalStore, type MouseEvent } from "react";

const subscribe = (onChange: () => void) => {
  window.addEventListener("popstate", onChange);
  return () => window.removeEventListener("popstate", onChange);
};

const snapshot = () => window.location.pathname + window.location.search;

/** Reads the current path and query from the browser history. */
export function useLocation() {
  const href = useSyncExternalStore(subscribe, snapshot, snapshot);
  const url = new URL(href, window.location.origin);
  return { path: url.pathname, query: url.searchParams };
}

/** Moves to another console page without reloading; `replace` rewrites the current entry instead of adding one, and `keepScroll` stays where the page is, whether the page or the shell's content surface scrolls. */
export function navigate(to: string, { replace = false, keepScroll = false }: { replace?: boolean; keepScroll?: boolean } = {}) {
  if (replace) window.history.replaceState(null, "", to);
  else window.history.pushState(null, "", to);
  if (!keepScroll) {
    window.scrollTo({ top: 0 });
    document.getElementById("main")?.scrollTo({ top: 0 });
  }
  window.dispatchEvent(new PopStateEvent("popstate"));
}

/** The click handler of a link drawn as an anchor: a plain click moves within the console without reloading, and a modified one (new tab, new window, download) is left to the browser. */
export const openLink = (href: string) => (event: MouseEvent) => {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  navigate(href);
};
