import { useMutation, useQuery } from "@tanstack/react-query";
import { errorText, type Approval, type ApprovalChoice } from "../api";
import { listChoices } from "../approve/choices";
import { DecisionButtons } from "../approve/DecisionButtons";
import { useLocale, useT } from "../i18n/I18nProvider";
import type { Locale } from "../i18n";
import { decideApproval } from "../mutations";
import { approvalsQuery, connectorsQuery, skillsQuery } from "../queries";
import { navigate } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { LongList } from "../ui/LongList";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { CAPTION, PANEL, SKELETON, SURFACE, button } from "../ui/styles";
import { Tag } from "../ui/Tag";

const fromNow = (iso: string, locale: Locale) => {
  const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  return Math.abs(seconds) < 90 ? format.format(seconds, "second") : format.format(Math.round(seconds / 60), "minute");
};

function Held({ approval, connector, skill, deciding, onDecide }: { approval: Approval; connector: string; skill?: string; deciding: ApprovalChoice | undefined; onDecide(choice: ApprovalChoice): void }) {
  const t = useT();
  const locale = useLocale();
  return (
    <li className={`${SURFACE} arrive @container transition-opacity duration-250 ${deciding ? "opacity-60" : ""}`}>
      <div className="flex flex-col gap-4 p-5 @3xl:flex-row @3xl:items-center">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <Tag tone="lime" dot>
              {t("approvals.waiting")}
            </Tag>
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
        <div className="min-w-0 @3xl:max-w-md">
          <DecisionButtons choices={listChoices(approval.mode)} deciding={deciding} onChoose={onDecide} />
        </div>
      </div>
    </li>
  );
}

/** Calls of functions set to ask, held until the owner approves or denies each one; the list refreshes on its own. */
export function ApprovalsPage() {
  const t = useT();
  const approvals = useQuery(approvalsQuery);
  const connectors = useQuery(connectorsQuery);
  const skills = useQuery(skillsQuery);
  const decide = useMutation(decideApproval);
  const list = approvals.data ?? [];
  const titleOf = (id: string) => connectors.data?.find((c) => c.id === id)?.title ?? id;
  const skillOf = (id: string) => skills.data?.find((s) => s.id === id)?.title;
  return (
    <section aria-labelledby="page-title">
      <header className="mb-10">
        <PageTitle section={t("nav.approvals")} title={t("approvals.title")} count={approvals.isSuccess ? list.length : undefined}>
          {t("approvals.intro")}
        </PageTitle>
      </header>
      {approvals.isError ? <Notice className="mb-4">{errorText(approvals.error)}</Notice> : null}
      {decide.isError ? <Notice className="mb-4">{errorText(decide.error)}</Notice> : null}
      {approvals.isPending ? (
        <ul aria-hidden="true" className="flex flex-col gap-3">
          {[0, 1].map((row) => (
            <li key={row} className={`${SKELETON} h-32`} />
          ))}
        </ul>
      ) : list.length === 0 && !approvals.isError ? (
        <div className={`${PANEL} p-4 sm:p-8`}>
          <EmptyState title={t("approvals.empty.title")} body={t("approvals.empty.body")}>
            <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => navigate("/")}>
              {t("approvals.empty.action")}
            </button>
          </EmptyState>
        </div>
      ) : (
        <LongList rows={list} textOf={(approval) => `${skillOf(approval.connector) ?? approval.function} ${titleOf(approval.connector)} ${approval.agent}`} label={t("approvals.filter")} whole>
          {(shown) => (
            <ul aria-live="polite" className="flex flex-col gap-3">
              {shown.map((approval) => (
                <Held
                  key={approval.id}
                  approval={approval}
                  connector={titleOf(approval.connector)}
                  skill={skillOf(approval.connector)}
                  deciding={decide.isPending && decide.variables.id === approval.id ? decide.variables.choice : undefined}
                  onDecide={(choice) => decide.mutate({ id: approval.id, choice })}
                />
              ))}
            </ul>
          )}
        </LongList>
      )}
    </section>
  );
}
