import type { Kind } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Tag } from "../ui/Tag";

const TONE: Record<Kind, string> = { read: "text-sky-400", write: "text-amber-400", destructive: "text-red-400" };

/** What a function does, as the site's small mono label: reads, changes or deletes, marked estimated when its server declares nothing and the console read it from its name. It informs; the owner's choice is off, on or ask. */
export function KindLabel({ kind, estimated = false }: { kind: Kind; estimated?: boolean }) {
  const t = useT();
  return (
    <span className={`font-mono text-xs whitespace-nowrap ${TONE[kind]}`} title={estimated ? t("kind.estimatedHint") : undefined}>
      {t(`kind.${kind}`)}
      {estimated ? <span className="text-muted"> · {t("kind.estimated")}</span> : null}
    </span>
  );
}

/** The flag on a row set on whose review or server declares it destructive: the engine still asks once in each session. */
export function AsksOnce() {
  const t = useT();
  return (
    <Tag tone="amber" title={t("kind.cappedHint")}>
      {t("kind.capped")}
    </Tag>
  );
}
