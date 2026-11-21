import { QueryClient, QueryClientProvider, type QueryKey } from "@tanstack/react-query";
import { createElement, type ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { I18nProvider } from "../i18n/I18nProvider";

const place = { href: "http://console.test/" };

const stub = {
  get location() {
    return new URL(place.href);
  },
  history: { pushState() {}, replaceState() {} },
  matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent: () => true,
  scrollTo() {},
  localStorage: { getItem: () => "en", setItem() {} },
};

Object.assign(globalThis, { window: stub, document: { documentElement: {}, activeElement: null } });

/** What a test page starts from: the address it is at, the answers already cached and the queries that already failed, by query key. */
export type Seed = { path?: string; data?: readonly (readonly [QueryKey, unknown])[]; failed?: readonly QueryKey[] };

/** Renders an element of the console to its HTML, in English, at `path`, with `data` already in the query cache and the `failed` queries already in error, as the server would draw its first frame. */
export function renderMarkup(element: ReactElement, { path = "/", data = [], failed = [] }: Seed = {}): string {
  place.href = new URL(path, "http://console.test").href;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity, enabled: false } } });
  for (const [key, value] of data) client.setQueryData(key, value);
  for (const queryKey of failed) client.getQueryCache().build(client, { queryKey }).setState({ status: "error", fetchStatus: "idle", error: new Error("The console is down.") });
  return renderToStaticMarkup(createElement(I18nProvider, null, createElement(QueryClientProvider, { client }, element)));
}
