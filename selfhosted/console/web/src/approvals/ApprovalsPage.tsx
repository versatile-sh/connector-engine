import { useMutation, useQuery } from "@tanstack/react-query";
import { errorText, type Approval, type ApprovalChoice } from "../api";
import { DecisionButtons } from "../approve/DecisionButtons";
import { useLocale, useT } from "../i18n/I18nProvider";
import type { Locale } from "../i18n";
import { cancelApproval, decideApproval } from "../mutations";
import { useViewer } from "../people/useViewer";
import { offersScope, scopedHref, scopeOf } from "../people/viewer";
import { connectorsQuery, scopedApprovalsQuery, skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { rowMotion, type RowPhase } from "../ui/kept";
import { LongList } from "../ui/LongList";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { CAPTION, PAGE, SKELETON, SURFACE, button } from "../ui/styles";
import { Toolbar } from "../ui/Toolbar";
import { ScopeSwitch } from "../people/ScopeSwitch";
import { Tag } from "../ui/Tag";
import { heldViewOf, type HeldView } from "./held";

const HELD_GRID = "grid grid-cols-[repeat(auto-fill,minmax(min(100%,34rem),1fr))] gap-6";

const fromNow = (iso: string, locale: Locale) => {
  const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  return Math.abs(seconds) < 90 ? format.format(seconds, "second") : format.format(Math.round(seconds / 60), "minute");
};

type HeldProps = { approval: Approval; view: HeldView; owner: boolean; connector: string; skill?: string | undefined; deciding: ApprovalChoice | undefined; cancelling: boolean; phase: RowPhase; onDecide(choice: ApprovalChoice): void; onCancel(): void };

function Held({ approval, view, owner, connector, skill, deciding, cancelling, phase, onDecide, onCancel }: HeldProps) {
  const t = useT();
  const locale = useLocale();
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
          <p className={`${CAPTION} mt-2 flex flex-wrap gap-x-3 gap-y-0.5`}>
            <span className="[overflow-wrap:anywhere]">{t("approvals.from", { agent: approval.agent })}</span>
            <span title={approval.session}>{t("approvals.session", { id: approval.session.slice(0, 8) })}</span>
            <span title={new Date(approval.expires_at).toLocaleString(locale)}>{t("approvals.expires", { when: fromNow(approval.expires_at, locale) })}</span>
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

/** Calls of functions set to ask, held until someone approves or denies each one, in a grid of cards on the content surface; the list refreshes on its own. Once a second user exists, a viewer who may see everyone's calls picks Mine or Everyone, and a call waiting on someone else offers what the server allows: its answers on a shared connector, and Cancel. */
export function ApprovalsPage() {
  const t = useT();
  const viewer = useViewer();
  const { path, query } = useLocation();
  const offered = offersScope(viewer, "approvals.any");
  const scope = scopeOf(query, offered);
  const approvals = useQuery(scopedApprovalsQuery(scope));
  const connectors = useQuery(connectorsQuery);
  const skills = useQuery(skillsQuery);
  const decide = useMutation(decideApproval);
  const cancel = useMutation(cancelApproval);
  const list = approvals.data ?? [];
  const modeOf = (id: string) => connectors.data?.find((c) => c.id === id)?.credential_mode;
  const titleOf = (id: string) => connectors.data?.find((c) => c.id === id)?.title ?? id;
  const skillOf = (id: string) => skills.data?.find((s) => s.id === id)?.title;
  const scopeSwitch = offered ? <ScopeSwitch scope={scope} onChange={(next) => navigate(scopedHref(path, query, next), { replace: true, keepScroll: true })} /> : null;
  const drawn = approvals.isPending || (list.length === 0 && !approvals.isError);
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader title={t("approvals.title")} count={approvals.isSuccess ? list.length : undefined} scope={scope === "everyone" ? t("scope.count") : undefined}>
        {t("approvals.intro")}
      </PageHeader>
      {scopeSwitch && drawn ? <Toolbar>{scopeSwitch}</Toolbar> : null}
      {approvals.isError ? <Notice>{errorText(approvals.error)}</Notice> : null}
      {decide.isError ? <Notice>{errorText(decide.error)}</Notice> : null}
      {cancel.isError ? <Notice>{errorText(cancel.error)}</Notice> : null}
      {approvals.isPending ? (
        <ul aria-hidden="true" className={HELD_GRID}>
          {[0, 1].map((row) => (
            <li key={row} className={`${SKELETON} h-32`} />
          ))}
        </ul>
      ) : list.length === 0 && !approvals.isError ? (
        <EmptyState title={t("approvals.empty.title")} body={t("approvals.empty.body")}>
          <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => navigate("/")}>
            {t("approvals.empty.action")}
          </button>
        </EmptyState>
      ) : (
        <LongList rows={list} textOf={(approval) => `${skillOf(approval.connector) ?? approval.function} ${titleOf(approval.connector)} ${approval.agent}`} keyOf={(approval) => approval.id} label={t("approvals.filter")} whole bar toolbar={scopeSwitch}>
          {(shown) => (
            <ul aria-live="polite" className={HELD_GRID}>
              {shown.map(({ row: approval, phase }) => (
                <Held
                  key={approval.id}
                  phase={phase}
                  approval={approval}
                  view={heldViewOf(approval, viewer, modeOf(approval.connector))}
                  owner={scope === "everyone"}
                  connector={titleOf(approval.connector)}
                  skill={skillOf(approval.connector)}
                  deciding={decide.isPending && decide.variables.id === approval.id ? decide.variables.choice : undefined}
                  cancelling={cancel.isPending && cancel.variables === approval.id}
                  onDecide={(choice) => decide.mutate({ id: approval.id, choice })}
                  onCancel={() => cancel.mutate(approval.id)}
                />
              ))}
            </ul>
          )}
        </LongList>
      )}
    </section>
  );
}
