import { useSyncExternalStore } from "react";

const subscribe = (onChange: () => void) => {
  window.addEventListener("popstate", onChange);
  return () => window.removeEventListener("popstate", onChange);
};

const snapshot = () => window.location.pathname + window.location.search;

/** Reads the current path and query from the browser history. */
export function useLocation() {
  const href = useSyncExternalStore(subscribe, snapshot);
  const url = new URL(href, window.location.origin);
  return { path: url.pathname, query: url.searchParams };
}

/** Moves to another console page without reloading; `replace` rewrites the current entry instead of adding one. */
export function navigate(to: string, { replace = false }: { replace?: boolean } = {}) {
  if (replace) window.history.replaceState(null, "", to);
  else window.history.pushState(null, "", to);
  window.scrollTo({ top: 0 });
  window.dispatchEvent(new PopStateEvent("popstate"));
}
