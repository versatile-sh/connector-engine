import { useMutation, useQuery } from "@tanstack/react-query";
import { errorText, type Approval, type ApprovalChoice } from "../api";
import { listChoices } from "../approve/choices";
import { DecisionButtons } from "../approve/DecisionButtons";
import { useLocale, useT } from "../i18n/I18nProvider";
import type { Locale } from "../i18n";
import { decideApproval } from "../mutations";
import { approvalsQuery, connectorsQuery, skillsQuery } from "../queries";
import { navigate } from "../router";
import { Bubble } from "../ui/Bubble";
import { Icon } from "../ui/Icon";
import { LongList } from "../ui/LongList";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { ACCENT_BAR, SURFACE, button } from "../ui/styles";

const fromNow = (iso: string, locale: Locale) => {
  const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  return Math.abs(seconds) < 90 ? format.format(seconds, "second") : format.format(Math.round(seconds / 60), "minute");
};

function Held({ approval, connector, skill, deciding, onDecide }: { approval: Approval; connector: string; skill?: string; deciding: ApprovalChoice | undefined; onDecide(choice: ApprovalChoice): void }) {
  const t = useT();
  const locale = useLocale();
  return (
    <li className={`arrive bubble relative flex flex-col gap-4 border border-line-strong bg-ink-raised py-5 pr-5 pl-6 sm:flex-row sm:items-center ${deciding ? "opacity-60" : ""} transition-opacity duration-300`}>
      <span aria-hidden="true" className={`${ACCENT_BAR} bg-lime`} />
      <span aria-hidden="true" className="grid size-10 shrink-0 place-items-center rounded-full bg-lime text-ink">
        <Icon name="hand" className="size-5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <code className="font-mono text-base font-semibold break-all" translate="no">
            {skill ?? approval.function}
          </code>
          {skill ? <Bubble>{t("approvals.skill")}</Bubble> : approval.class ? <Bubble>{approval.class}</Bubble> : null}
        </div>
        {skill ? null : <p className="mt-0.5 text-sm text-muted">{t("approvals.on", { connector })}</p>}
        <p className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 font-mono text-xs text-muted [overflow-wrap:anywhere]">
          <span>{t("approvals.from", { agent: approval.agent })}</span>
          <span title={approval.session}>{t("approvals.session", { id: approval.session.slice(0, 8) })}</span>
          <span title={new Date(approval.expires_at).toLocaleString(locale)}>{t("approvals.expires", { when: fromNow(approval.expires_at, locale) })}</span>
        </p>
      </div>
      <div className="min-w-0 sm:max-w-md">
        <DecisionButtons choices={listChoices(approval.mode)} deciding={deciding} onChoose={onDecide} />
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
      <header className="mb-8">
        <PageTitle section={t("nav.approvals")} title={t("approvals.title")}>
          {approvals.isSuccess ? <Bubble tone={list.length > 0 ? "accent" : "neutral"}>{list.length}</Bubble> : null}
        </PageTitle>
        <p className="mt-3 max-w-2xl text-muted">{t("approvals.intro")}</p>
      </header>
      {approvals.isError ? <Notice>{errorText(approvals.error)}</Notice> : null}
      {decide.isError ? (
        <div className="mb-4">
          <Notice>{errorText(decide.error)}</Notice>
        </div>
      ) : null}
      {approvals.isPending ? (
        <ul aria-hidden="true" className="flex flex-col gap-3">
          {[0, 1].map((row) => (
            <li key={row} className="shimmer bubble h-24" />
          ))}
        </ul>
      ) : list.length === 0 && !approvals.isError ? (
        <div className={`${SURFACE} arrive flex flex-col items-center px-6 py-14 text-center`}>
          <span aria-hidden="true" className="grid size-14 place-items-center rounded-full border border-line-strong text-muted">
            <Icon name="hand" className="size-6" />
          </span>
          <h2 className="mt-4 font-serif text-3xl leading-none font-normal tracking-[-0.02em]">{t("approvals.empty.title")}</h2>
          <p className="mt-2 max-w-md text-muted">{t("approvals.empty.body")}</p>
          <button type="button" className={`${button({ variant: "secondary", size: "sm" })} mt-6`} onClick={() => navigate("/")}>
            {t("approvals.empty.action")}
          </button>
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
