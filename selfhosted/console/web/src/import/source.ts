import type { ImportSource } from "../api";

/** Where a pasted address points: an MCP server, or a listing that describes one. */
export type SourceKind = "mcp" | "listing";

/** A pasted address read as an import source, with the kind it turned out to be, or a refusal. */
export type SourceRead = { ok: true; kind: SourceKind; source: ImportSource } | { ok: false };

const httpURL = (text: string) => {
  try {
    const url = new URL(text.trim());
    return url.protocol === "http:" || url.protocol === "https:" ? url : undefined;
  } catch {
    return undefined;
  }
};

/** Reads a pasted address as an import source of the kind chosen; a console install link (`/import?listing=` or `?mcp=`) is unwrapped to the address it carries. Local reach and the secret header only ever apply to an MCP server. */
export function readSource(text: string, kind: SourceKind, options: { local: boolean; headerSecret: boolean }): SourceRead {
  const url = httpURL(text);
  if (!url) return { ok: false };
  if (url.pathname.endsWith("/import")) {
    const listing = url.searchParams.get("listing");
    const mcp = url.searchParams.get("mcp");
    if (listing) return httpURL(listing) ? { ok: true, kind: "listing", source: { listing: listing.trim() } } : { ok: false };
    if (mcp) return readSource(mcp, "mcp", options);
  }
  if (kind === "listing") return { ok: true, kind, source: { listing: url.href } };
  return { ok: true, kind, source: { mcp: url.href, ...(options.local ? { local: true } : {}), ...(options.headerSecret ? { header_secret: true } : {}) } };
}

const LINK_PATHS: ReadonlySet<string> = new Set(["/import", "/", "/connectors"]);

/** The address an install link in the console's address carries, `?listing=` or `?mcp=`: on `/import`, where the site sends them, and on the front pages `/` and `/connectors`, where older links and pasted ones land. */
export function linkOf(path: string, query: URLSearchParams): { listing: string } | { mcp: string } | undefined {
  if (!LINK_PATHS.has(path)) return undefined;
  const listing = query.get("listing");
  const mcp = query.get("mcp");
  return listing ? { listing } : mcp ? { mcp } : undefined;
}
