import type { CallOutcome } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";

const TONE = { ok: "outline", error: "amber", denied: "neutral", pending: "accent", running: "neutral" } as const;

/** What a call came to, as a small bubble: lime when it answered, amber when it failed, filled when it waits for the owner. */
export function OutcomeBubble({ outcome, title }: { outcome: CallOutcome; title?: string | undefined }) {
  const t = useT();
  return (
    <Bubble tone={TONE[outcome]} {...(title ? { title } : {})}>
      {t(`outcome.${outcome}`)}
    </Bubble>
  );
}
