import type { CallOutcome } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Tag } from "../ui/Tag";

const TONE = { ok: "muted", error: "amber", denied: "muted", pending: "lime", running: "muted" } as const;

/** What a call came to, as a small mono tag: quiet when it answered, amber with a dot when it failed, lime with a dot while it waits for the owner. */
export function OutcomeTag({ outcome, title }: { outcome: CallOutcome; title?: string | undefined }) {
  const t = useT();
  return (
    <Tag tone={TONE[outcome]} dot={outcome === "error" || outcome === "pending"} title={title}>
      {t(`outcome.${outcome}`)}
    </Tag>
  );
}
