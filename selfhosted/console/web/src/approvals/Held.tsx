import { useQuery } from "@tanstack/react-query";
import type { Approval, ApprovalChoice } from "../api";
import { DecisionButtons } from "../approve/DecisionButtons";
import { useLocale, useT } from "../i18n/I18nProvider";
import { approvalArgsQuery } from "../queries";
import { navigate } from "../router";
import { rowMotion, type RowPhase } from "../ui/kept";
import { CAPTION, SKELETON_LINE, SURFACE, WELL, button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { useNow } from "../ui/useNow";
import type { HeldView } from "./held";
import { previewOf, relativeOf } from "./recent";

type HeldProps = { approval: Approval; view: HeldView; owner: boolean; connector: string; skill?: string | undefined; deciding: ApprovalChoice | undefined; cancelling: boolean; phase: RowPhase; onDecide(choice: ApprovalChoice): void; onCancel(): void };

function Arguments({ id }: { id: string }) {
  const t = useT();
  const args = useQuery(approvalArgsQuery(id));
  if (args.isError) return null;
  if (args.isPending) return <div aria-hidden="true" className={`${SKELETON_LINE} mt-3 h-8 w-full max-w-md`} />;
  const line = previewOf(args.data);
  if (!line) return null;
  return (
    <p className={`${WELL} mt-3 line-clamp-2 px-3 py-2 font-mono text-xs break-all text-muted`} translate="no">
      <span className="sr-only">{t("common.arguments")}: </span>
      {line}
    </p>
  );
}

/** One held call as a card of the waiting list: who it waits on and what it is, when it was asked and when it runs out, a preview of its arguments read once from the call's own page, the app and session behind it and, on the right or under it, the answers the server offers. */
export function Held({ approval, view, owner, connector, skill, deciding, cancelling, phase, onDecide, onCancel }: HeldProps) {
  const t = useT();
  const locale = useLocale();
  const now = useNow();
  return (
    <li {...rowMotion(phase)} className={`${SURFACE} @container transition-opacity duration-250 ${deciding || cancelling ? "opacity-60" : ""}`}>
      <div className="flex flex-col gap-4 p-5 @3xl:flex-row @3xl:items-center">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <Tag tone="lime" dot>
              {view.waitingFor ? t("approvals.waitingFor", { name: view.waitingFor }) : t("approvals.waiting")}
            </Tag>
            {owner && approval.owner && !view.waitingFor ? <Tag>{approval.owner.username}</Tag> : null}
            {skill ? <Tag>{t("approvals.skill")}</Tag> : approval.class ? <Tag>{approval.class}</Tag> : null}
          </div>
          <code className="mt-2 block font-mono text-base font-medium break-all text-paper" translate="no">
            {skill ?? approval.function}
          </code>
          {skill ? null : <p className="mt-0.5 text-sm text-muted">{t("approvals.on", { connector })}</p>}
          <Arguments id={approval.id} />
          <p className={`${CAPTION} mt-3 flex flex-wrap gap-x-3 gap-y-0.5`}>
            <span title={new Date(approval.created_at).toLocaleString(locale)}>{t("approvals.asked", { when: relativeOf(approval.created_at, now, locale) })}</span>
            <span className="[overflow-wrap:anywhere]">{t("approvals.from", { agent: approval.agent })}</span>
            <span title={approval.session}>{t("approvals.session", { id: approval.session.slice(0, 8) })}</span>
            <span title={new Date(approval.expires_at).toLocaleString(locale)}>{t("approvals.expires", { when: relativeOf(approval.expires_at, now, locale) })}</span>
          </p>
        </div>
        <div className={view.cancel || view.decidesAlone || view.open ? "flex min-w-0 flex-col gap-2 @3xl:max-w-md @3xl:items-end" : "min-w-0 @3xl:max-w-md"}>
          {view.decide.length > 0 ? <DecisionButtons choices={view.decide} deciding={deciding} onChoose={onDecide} /> : null}
          {view.cancel ? (
            <button type="button" disabled={cancelling || deciding !== undefined} className={`${button({ variant: "secondary", size: "sm" })} self-start @3xl:self-end ${cancelling ? "pulse" : ""}`} onClick={onCancel}>
              {t("approvals.cancel")}
            </button>
          ) : null}
          {view.open ? (
            <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => navigate(`/approve/${encodeURIComponent(approval.id)}`)}>
              {t("approvals.open")}
            </button>
          ) : null}
          {view.decidesAlone ? <p className={`${CAPTION} max-w-xs text-pretty`}>{t("approvals.decidesAlone", { name: view.decidesAlone })}</p> : null}
        </div>
      </div>
    </li>
  );
}
