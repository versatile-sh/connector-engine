import type { ReactNode } from "react";
import type { ApprovalChoice, ApprovalView } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { Notice } from "../ui/Notice";
import { CAPTION, SURFACE } from "../ui/styles";
import { clock, secondsLeft, summarizeArguments } from "./choices";
import { DecisionButtons } from "./DecisionButtons";
import { Outcome } from "./Outcome";
import { useNow } from "./useNow";

type ApprovalCardProps = {
  approval: ApprovalView;
  signedIn: boolean;
  first: boolean;
  deciding: ApprovalChoice | undefined;
  error: string | undefined;
  onChoose(choice: ApprovalChoice): void;
};

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-0.5 border-b border-line py-2.5 last:border-b-0 sm:grid-cols-[7rem_minmax(0,1fr)] sm:gap-4">
      <dt className={CAPTION}>{label}</dt>
      <dd className="min-w-0 text-sm">{children}</dd>
    </div>
  );
}

function Warning({ children }: { children: ReactNode }) {
  return <p className="bubble border border-amber/40 border-l-4 border-l-amber bg-amber/5 py-2.5 pr-3 pl-4 text-sm">{children}</p>;
}

/** One held call laid out for a decision: the connector, the function and its nature, the app and its session, the time left, the warnings, the arguments, and the answers or how it ended. */
export function ApprovalCard({ approval, signedIn, first, deciding, error, onChoose }: ApprovalCardProps) {
  const t = useT();
  const locale = useLocale();
  const pending = approval.state === "pending";
  const now = useNow(pending);
  const left = secondsLeft(approval.expires_at, now);
  const open = pending && left > 0;
  const summary = summarizeArguments(approval.arguments);
  return (
    <article aria-labelledby="approve-title" className={`${SURFACE} arrive p-5 sm:p-7`}>
      <div className="flex items-center gap-3">
        <ConnectorIcon src={signedIn ? approval.connector.icon : undefined} title={approval.connector.title} />
        <div className="min-w-0">
          <p className="truncate font-medium">{approval.connector.title}</p>
          <p className="truncate font-mono text-xs text-muted" translate="no">
            {approval.connector.alias}
          </p>
        </div>
        <span className="ml-auto">
          <Bubble tone={approval.kind === "destructive" ? "amber" : "neutral"}>{t(`kind.${approval.kind}`)}</Bubble>
        </span>
      </div>
      <h1 id="approve-title" className="text-lit mt-6 font-serif text-4xl leading-[1.02] font-normal tracking-[-0.02em] break-words sm:text-5xl">
        {approval.function_title ?? approval.function}
      </h1>
      <code className="mt-2 block font-mono text-sm break-all text-muted" translate="no">
        {approval.function}
      </code>
      <dl className="mt-5">
        <Fact label={t("approve.app")}>
          <span className="font-mono" translate="no">
            {approval.app}
          </span>
        </Fact>
        <Fact label={t("approve.session")}>
          <span className="font-mono" title={approval.session} translate="no">
            {approval.session.slice(0, 12)}
          </span>
          <span className="mt-0.5 block text-[0.8125rem] leading-snug text-muted">{t("approve.sessionNote")}</span>
        </Fact>
        <Fact label={t("approve.expires")}>
          <span className={`font-mono tabular-nums ${open && left < 60 ? "text-amber" : ""}`} title={new Date(approval.expires_at).toLocaleString(locale)}>
            {open ? t("approve.left", { time: clock(left) }) : t("approve.closed")}
          </span>
        </Fact>
      </dl>
      {open && (approval.may_have_run || approval.reaches_outside) ? (
        <div className="mt-4 flex flex-col gap-2">
          {approval.may_have_run ? <Warning>{t("approve.mayHaveRun")}</Warning> : null}
          {approval.reaches_outside ? <Warning>{t("approve.reachesOutside")}</Warning> : null}
        </div>
      ) : null}
      {summary ? (
        <details className="group mt-4 bubble border border-line-strong bg-ink-sunken">
          <summary className="flex cursor-pointer items-center justify-between gap-3 px-4 py-2.5 text-sm font-medium select-none">
            {t("approve.arguments")}
            {summary.cut > 0 ? <span className={CAPTION}>{t("approve.cut", { count: summary.cut })}</span> : null}
          </summary>
          <pre className="no-scrollbar max-h-80 overflow-auto border-t border-line px-4 py-3 font-mono text-[0.8125rem] leading-relaxed whitespace-pre text-paper">{summary.text}</pre>
        </details>
      ) : null}
      <div className="mt-6">
        {open ? (
          <>
            <DecisionButtons choices={approval.choices} deciding={deciding} size="md" stacked onChoose={onChoose} />
            {error ? (
              <div className="mt-3">
                <Notice>{error}</Notice>
              </div>
            ) : null}
          </>
        ) : (
          <Outcome approval={pending ? { ...approval, state: "expired" } : approval} first={first} />
        )}
      </div>
    </article>
  );
}
