import type { ActivityCall, Connector } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { KindLabel } from "../connectors/KindLabel";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { CAPTION, PRESS } from "../ui/styles";
import { approvalStateKey, durationText, kindOf, timeOf } from "./activity";
import { OutcomeBubble } from "./OutcomeBubble";

const ROW = `grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 px-3 py-3 text-left hover:bg-paper/5 focus-visible:bg-paper/5 sm:px-4 ${PRESS} active:scale-[0.995]`;

function Asked({ state }: { state: string | undefined }) {
  const t = useT();
  const key = approvalStateKey(state);
  return <Bubble {...(key ? { title: t(key) } : {})}>{t("activity.asked")}</Bubble>;
}

type CallListProps = { calls: readonly ActivityCall[]; connectors: readonly Connector[]; onOpen(id: string): void };

/** The calls newest first, one row each: the connector's icon, the function with its class and approval, when, through which connector and from which app, and on the right the outcome and the duration. */
export function CallList({ calls, connectors, onOpen }: CallListProps) {
  const locale = useLocale();
  return (
    <ol className="-mx-1 flex flex-col">
      {calls.map((call) => {
        const connector = connectors.find((c) => c.id === call.connector);
        const title = connector?.title ?? call.alias ?? call.connector ?? "?";
        const kind = kindOf(call.class);
        const when = timeOf(call.at, locale);
        return (
          <li key={call.id} className="arrive border-b border-line last:border-b-0">
            <button type="button" className={ROW} onClick={() => onOpen(call.id)}>
              <ConnectorIcon src={connector?.icon} title={title} size="sm" />
              <span className="min-w-0">
                <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <code className="font-mono text-sm font-semibold break-all text-paper" translate="no">
                    {call.function}
                  </code>
                  {kind ? <KindLabel kind={kind} /> : null}
                  {call.approval ? <Asked state={call.approval_state} /> : null}
                </span>
                <span className={`${CAPTION} mt-1 flex flex-wrap gap-x-2`}>
                  <time dateTime={call.at}>{when}</time>
                  <span aria-hidden="true">·</span>
                  <span className="truncate">{title}</span>
                  <span aria-hidden="true">·</span>
                  <span className="truncate" translate="no">
                    {call.agent}
                  </span>
                </span>
              </span>
              <span className="flex flex-col items-end gap-1">
                <OutcomeBubble outcome={call.outcome} title={call.code} />
                <span className={`${CAPTION} tabular-nums`}>{durationText(call.duration_ms)}</span>
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}
