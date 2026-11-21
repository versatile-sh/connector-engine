import type { ActivityCall } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { KindLabel } from "../connectors/KindLabel";
import { useLocale, useT } from "../i18n/I18nProvider";
import { rowMotion } from "../ui/kept";
import { CAPTION, PRESS } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { useKept } from "../ui/useKept";
import { AccessKeyName } from "./AccessKeyName";
import { approvalStateKey, durationText, kindOf, skillFile, timeOf, type Named } from "./activity";
import { OutcomeTag } from "./OutcomeTag";

const ROW = `grid w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 rounded-lg px-3 py-3 text-left hover:bg-paper/5 focus-visible:bg-paper/5 @lg:px-4 ${PRESS} active:scale-[0.995]`;

function Asked({ state }: { state: string | undefined }) {
  const t = useT();
  const key = approvalStateKey(state);
  return <Tag title={key ? t(key) : undefined}>{t("activity.asked")}</Tag>;
}

type CallListProps = { calls: readonly ActivityCall[]; named: ReadonlyMap<string, Named>; owners?: boolean; scope: string; onOpen(id: string): void };

/** The calls newest first, a new one sliding in at the top unless the `scope` (the filter and how many pages are loaded) changed, one row each: the icon, the function or the skill with its class and approval, when, through which connector or file and from which app, then the outcome and the duration; with `owners`, whose call it was. */
export function CallList({ calls, named, owners = false, scope, onOpen }: CallListProps) {
  const t = useT();
  const locale = useLocale();
  const drawn = useKept(calls, (call) => call.id, scope);
  return (
    <ol className="@container -mx-1 flex flex-col">
      {drawn.map(({ row: call, phase }) => {
        const owner = call.connector ? named.get(call.connector) : undefined;
        const title = owner?.title ?? call.alias ?? call.connector ?? "?";
        const kind = kindOf(call.class);
        const when = timeOf(call.at, locale);
        return (
          <li key={call.id} {...rowMotion(phase)} className="border-b border-line last:border-b-0">
            <button type="button" className={ROW} onClick={() => onOpen(call.id)}>
              <ConnectorIcon src={owner?.icon} title={title} size="sm" />
              <span className="min-w-0">
                <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <code className="font-mono text-sm font-medium break-all text-paper" translate="no">
                    {call.skill ?? call.function}
                  </code>
                  {call.skill ? <Tag>{t("activity.skill")}</Tag> : null}
                  {kind && !call.skill ? <KindLabel kind={kind} /> : null}
                  {call.approval ? <Asked state={call.approval_state} /> : null}
                  {owners && call.owner ? <Tag>{call.owner.username || t("people.gone")}</Tag> : null}
                </span>
                <span className={`${CAPTION} mt-1 flex flex-wrap gap-x-2`}>
                  <time dateTime={call.at}>{when}</time>
                  <span aria-hidden="true">·</span>
                  <span className="truncate" translate={call.skill ? "no" : undefined}>
                    {call.skill ? skillFile(call.path) : title}
                  </span>
                  <span aria-hidden="true">·</span>
                  <AccessKeyName name={call.agent} />
                </span>
              </span>
              <span className="flex flex-col items-end gap-1 @lg:flex-row @lg:items-center @lg:gap-3">
                <OutcomeTag outcome={call.outcome} title={call.code} />
                <span className={`${CAPTION} tabular-nums @lg:w-14 @lg:text-right`}>{durationText(call.duration_ms)}</span>
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}
