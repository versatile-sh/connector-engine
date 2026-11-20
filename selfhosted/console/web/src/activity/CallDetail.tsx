import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { errorText, type ActivityDetail } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { KindLabel } from "../connectors/KindLabel";
import { useLocale, useT } from "../i18n/I18nProvider";
import { activityCallQuery } from "../queries";
import { Notice } from "../ui/Notice";
import { Sheet } from "../ui/Sheet";
import { CAPTION, SKELETON } from "../ui/styles";
import { approvalStateKey, durationText, kindOf, prettyJSON, skillFile, timeOf, type Named } from "./activity";
import { OutcomeTag } from "./OutcomeTag";

const PRE = "mt-2 max-h-80 overflow-auto rounded-xl border border-line bg-ink-sunken p-4 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-paper";

function Captured({ id, title, kept, text, cut, off, none }: { id: string; title: string; kept: boolean; text: string | undefined; cut: boolean | undefined; off: string; none: string }) {
  const t = useT();
  return (
    <section aria-labelledby={id} className="mt-6">
      <h3 id={id} className="text-sm font-medium">
        {title}
      </h3>
      {!kept ? (
        <p className="mt-1.5 text-sm text-muted">{off}</p>
      ) : text ? (
        <>
          <pre tabIndex={0} aria-labelledby={id} className={PRE} translate="no">
            {cut ? text : prettyJSON(text)}
          </pre>
          {cut ? <p className={`${CAPTION} mt-1.5`}>{t("activity.detail.cut")}</p> : null}
        </>
      ) : (
        <p className="mt-1.5 text-sm text-muted">{none}</p>
      )}
    </section>
  );
}

function Facts({ call, connector }: { call: ActivityDetail; connector: string }) {
  const t = useT();
  const locale = useLocale();
  const kind = kindOf(call.class);
  const approval = approvalStateKey(call.approval_state);
  const facts: { label: string; value: ReactNode }[] = [
    { label: t("activity.when"), value: <time dateTime={call.at}>{timeOf(call.at, locale)}</time> },
    { label: t("common.app"), value: call.agent },
    ...(call.skill
      ? [
          { label: t("activity.detail.skill"), value: connector === call.skill ? call.skill : `${connector} (${call.skill})` },
          { label: t("activity.detail.file"), value: skillFile(call.path) },
        ]
      : [{ label: t("activity.connector"), value: connector }, ...(kind ? [{ label: t("activity.detail.kind"), value: <KindLabel kind={kind} /> }] : [])]),
    { label: t("activity.outcome"), value: [t(`outcome.${call.outcome}`), call.code, call.detail].filter(Boolean).join(" · ") },
    { label: t("activity.detail.duration"), value: durationText(call.duration_ms) },
    ...(call.approval ? [{ label: t("activity.detail.approval"), value: approval ? t(approval) : (call.approval_state ?? call.approval) }] : []),
    ...(call.result_bytes ? [{ label: t("activity.detail.size"), value: t("activity.detail.bytes", { count: call.result_bytes }) }] : []),
    ...(call.session ? [{ label: t("common.session"), value: call.session }] : []),
  ];
  return (
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
      {facts.map((fact) => (
        <div key={fact.label} className="contents">
          <dt className="text-muted">{fact.label}</dt>
          <dd className="min-w-0 font-mono text-xs leading-5 break-words">{fact.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** One call in full, in the modal window: who called what and how it ended, then the arguments as indented JSON and the result excerpt, or why they are not here. */
export function CallDetail({ id, named, onClose }: { id: string; named: ReadonlyMap<string, Named>; onClose(): void }) {
  const t = useT();
  const detail = useQuery(activityCallQuery(id));
  const call = detail.data;
  const connector = call?.connector ? named.get(call.connector) : undefined;
  const title = connector?.title ?? call?.alias ?? call?.connector ?? "?";
  return (
    <Sheet title={call?.skill ?? call?.function ?? t("activity.calls")} closeLabel={t("common.close")} onClose={onClose} wide>
      {detail.isPending ? (
        <div aria-hidden="true" className="flex flex-col gap-3">
          <div className={`${SKELETON} h-10`} />
          <div className={`${SKELETON} h-40`} />
        </div>
      ) : detail.isError ? (
        <Notice>{errorText(detail.error)}</Notice>
      ) : call ? (
        <div className="swap-in">
          <div className="mb-5 flex items-center gap-3">
            <ConnectorIcon src={connector?.icon} title={title} size="sm" />
            <p className="min-w-0 flex-1 truncate font-medium">{title}</p>
            <OutcomeTag outcome={call.outcome} title={call.code} />
          </div>
          <Facts call={call} connector={title} />
          {call.captured ? (
            <>
              <Captured
                id="call-arguments"
                title={t("common.arguments")}
                kept={call.kept_arguments}
                text={call.arguments}
                cut={call.arguments_cut}
                off={t("activity.detail.argumentsOff")}
                none={t("activity.detail.noArguments")}
              />
              <Captured
                id="call-result"
                title={t("activity.detail.result")}
                kept={call.kept_result || call.outcome === "pending"}
                text={call.result}
                cut={call.result_cut}
                off={t("activity.detail.resultOff")}
                none={t("activity.detail.noResult")}
              />
            </>
          ) : (
            <p className="mt-6 text-sm text-muted">{t("activity.detail.uncaptured")}</p>
          )}
        </div>
      ) : null}
    </Sheet>
  );
}
