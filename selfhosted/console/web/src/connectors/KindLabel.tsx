import type { Kind } from "../api";
import { useT } from "../i18n/I18nProvider";

const TONE: Record<Kind, string> = { read: "text-sky-400", write: "text-amber-400", destructive: "text-red-400" };

/** What a function does by its own description, as the site's small mono label: reads, changes or deletes. It informs; the owner's choice is off, on or ask. */
export function KindLabel({ kind }: { kind: Kind }) {
  const t = useT();
  return <span className={`font-mono text-xs whitespace-nowrap ${TONE[kind]}`}>{t(`kind.${kind}`)}</span>;
}
