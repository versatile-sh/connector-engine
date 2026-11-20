import type { ActivityCall } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { KindLabel } from "../connectors/KindLabel";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { ACCENT_BAR, CAPTION, PRESS } from "../ui/styles";
import { approvalStateKey, durationText, kindOf, skillFile, timeOf, type Named } from "./activity";
import { OutcomeBubble } from "./OutcomeBubble";

const ROW = `grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 py-3 pr-3 pl-4 text-left hover:bg-paper/5 focus-visible:bg-paper/5 @lg:pr-4 ${PRESS} active:scale-[0.995]`;

const BAR = { error: "bg-amber", denied: "bg-amber/50", pending: "bg-lime" } as const;

function Asked({ state }: { state: string | undefined }) {
  const t = useT();
  const key = approvalStateKey(state);
  return <Bubble {...(key ? { title: t(key) } : {})}>{t("activity.asked")}</Bubble>;
}

type CallListProps = { calls: readonly ActivityCall[]; named: ReadonlyMap<string, Named>; onOpen(id: string): void };

/** The calls newest first, one row each: the icon, the function or the skill with its class and approval, when, through which connector or file and from which app, then the outcome and the duration; a failed or waiting call carries a coloured bar. */
export function CallList({ calls, named, onOpen }: CallListProps) {
  const t = useT();
  const locale = useLocale();
  return (
    <ol className="@container -mx-1 flex flex-col">
      {calls.map((call) => {
        const owner = call.connector ? named.get(call.connector) : undefined;
        const title = owner?.title ?? call.alias ?? call.connector ?? "?";
        const kind = kindOf(call.class);
        const when = timeOf(call.at, locale);
        const bar = call.outcome === "error" || call.outcome === "denied" || call.outcome === "pending" ? BAR[call.outcome] : undefined;
        return (
          <li key={call.id} className="arrive relative border-b border-line last:border-b-0">
            {bar ? <span aria-hidden="true" className={`${ACCENT_BAR} ${bar}`} /> : null}
            <button type="button" className={ROW} onClick={() => onOpen(call.id)}>
              <ConnectorIcon src={owner?.icon} title={title} size="sm" />
              <span className="min-w-0">
                <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <code className="font-mono text-sm font-semibold break-all text-paper" translate="no">
                    {call.skill ?? call.function}
                  </code>
                  {call.skill ? <Bubble tone="outline">{t("activity.skill")}</Bubble> : null}
                  {kind && !call.skill ? <KindLabel kind={kind} /> : null}
                  {call.approval ? <Asked state={call.approval_state} /> : null}
                </span>
                <span className={`${CAPTION} mt-1 flex flex-wrap gap-x-2`}>
                  <time dateTime={call.at}>{when}</time>
                  <span aria-hidden="true">·</span>
                  <span className="truncate" translate={call.skill ? "no" : undefined}>
                    {call.skill ? skillFile(call.path) : title}
                  </span>
                  <span aria-hidden="true">·</span>
                  <span className="truncate" translate="no">
                    {call.agent}
                  </span>
                </span>
              </span>
              <span className="flex flex-col items-end gap-1 @lg:flex-row @lg:items-center @lg:gap-3">
                <OutcomeBubble outcome={call.outcome} title={call.code} />
                <span className={`${CAPTION} tabular-nums @lg:w-14 @lg:text-right`}>{durationText(call.duration_ms)}</span>
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}
