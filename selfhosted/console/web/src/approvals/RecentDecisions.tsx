import type { DecisionRow } from "../api";
import type { Named } from "../activity/activity";
import type { MessageKey } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { CAPTION, SKELETON_LINE, SURFACE, TITLE } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { useNow } from "../ui/useNow";
import { decisionOf, relativeOf, type DecisionKind } from "./recent";

const LABEL: Record<DecisionKind, MessageKey> = { approved: "approvals.recent.approved", denied: "approvals.recent.denied", expired: "approvals.recent.expired", cancelled: "approvals.recent.cancelled" };

const TONE: Record<DecisionKind, { tone: "paper" | "amber" | "muted"; dot: boolean }> = { approved: { tone: "paper", dot: false }, denied: { tone: "amber", dot: true }, expired: { tone: "muted", dot: false }, cancelled: { tone: "muted", dot: false } };

type RecentDecisionsProps = { decisions: readonly DecisionRow[] | undefined; failed: boolean; named: ReadonlyMap<string, Named>; owners: boolean };

/** The last decisions, newest first, each with how it ended, the function or skill and its connector, the key that asked and, to the right, who answered and when; with everyone's, the owner of the call too. */
export function RecentDecisions({ decisions, failed, named, owners }: RecentDecisionsProps) {
  const t = useT();
  const locale = useLocale();
  const now = useNow();
  return (
    <section aria-labelledby="recent-decisions-title" className={`${SURFACE} min-w-0 p-5 sm:p-6`}>
      <h2 id="recent-decisions-title" className={TITLE}>
        {t("approvals.recent.title")}
      </h2>
      {!decisions && failed ? (
        <p className="mt-2 text-sm text-pretty text-muted">{t("approvals.unavailable")}</p>
      ) : !decisions ? (
        <div aria-hidden="true" className="mt-4 flex flex-col gap-3">
          {[0, 1, 2].map((row) => (
            <div key={row} className={`${SKELETON_LINE} h-9 w-full`} />
          ))}
        </div>
      ) : decisions.length === 0 ? (
        <p className="mt-2 text-sm text-pretty text-muted">{t("approvals.recent.empty")}</p>
      ) : (
        <ul className="mt-3 flex flex-col divide-y divide-line">
          {decisions.map((row) => {
            const kind = decisionOf(row.state);
            const owner = named.get(row.connector);
            const what = owner?.skill ? owner.title : row.function;
            return (
              <li key={row.id} className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-4 gap-y-1 py-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    {kind ? (
                      <Tag tone={TONE[kind].tone} dot={TONE[kind].dot}>
                        {t(LABEL[kind])}
                      </Tag>
                    ) : (
                      <Tag>{row.state}</Tag>
                    )}
                    <code className="min-w-0 truncate font-mono text-sm text-paper" translate="no">
                      {what}
                    </code>
                  </div>
                  <p className={`${CAPTION} mt-1 flex flex-wrap gap-x-2`}>
                    {owner && !owner.skill ? <span className="truncate">{t("approvals.on", { connector: owner.title })}</span> : null}
                    <span className="truncate [overflow-wrap:anywhere]">{t("approvals.from", { agent: row.agent })}</span>
                    {owners && row.owner?.username ? <span className="truncate">{row.owner.username}</span> : null}
                  </p>
                </div>
                <p className={`${CAPTION} text-right`}>
                  {row.decided_by ? <span className="block">{t("approvals.recent.by", { name: row.decided_by })}</span> : null}
                  <time dateTime={row.at} title={new Date(row.at).toLocaleString(locale)} className="block">
                    {relativeOf(row.at, now, locale)}
                  </time>
                </p>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
