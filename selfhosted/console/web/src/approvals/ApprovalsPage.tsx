import { useMutation, useQuery } from "@tanstack/react-query";
import { namedOf } from "../activity/activity";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { cancelApproval, decideApproval } from "../mutations";
import { ScopeSwitch } from "../people/ScopeSwitch";
import { useViewer } from "../people/useViewer";
import { offersScope, scopedHref, scopeOf } from "../people/viewer";
import { connectorsQuery, notificationsQuery, recentDecisionsQuery, scopedApprovalsQuery, skillsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { LongList } from "../ui/LongList";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { PAGE, SKELETON, SPLIT } from "../ui/styles";
import { Toolbar } from "../ui/Toolbar";
import { AskFunctions } from "./AskFunctions";
import { Held } from "./Held";
import { heldViewOf } from "./held";
import { RecentDecisions } from "./RecentDecisions";
import { ReachYou } from "./ReachYou";

const LIST = "flex flex-col gap-6";

/** Calls of functions set to ask, held until someone approves or denies each one: the waiting calls as cards are the page's main column, or, with none, one compact card saying so, and under them the last decisions; beside them the connectors and functions set to Ask and where a held call reaches the viewer. The list refreshes on its own. Once a second user exists, a viewer who may see everyone's calls picks Mine or Everyone, and a call waiting on someone else offers what the server allows: its answers on a shared connector, and Cancel. */
export function ApprovalsPage() {
  const t = useT();
  const viewer = useViewer();
  const { path, query } = useLocation();
  const offered = offersScope(viewer, "approvals.any");
  const scope = scopeOf(query, offered);
  const approvals = useQuery(scopedApprovalsQuery(scope));
  const decisions = useQuery(recentDecisionsQuery(scope));
  const connectors = useQuery(connectorsQuery);
  const skills = useQuery(skillsQuery);
  const notifications = useQuery(notificationsQuery);
  const decide = useMutation(decideApproval);
  const cancel = useMutation(cancelApproval);
  const list = approvals.data ?? [];
  const named = namedOf(connectors.data ?? [], skills.data ?? []);
  const modeOf = (id: string) => connectors.data?.find((c) => c.id === id)?.credential_mode;
  const titleOf = (id: string) => named.get(id)?.title ?? id;
  const skillOf = (id: string) => skills.data?.find((s) => s.id === id)?.title;
  const everyone = scope === "everyone";
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader title={t("approvals.title")} count={approvals.isSuccess ? list.length : undefined} scope={everyone ? t("scope.count") : undefined}>
        {t("approvals.intro")}
      </PageHeader>
      {offered ? (
        <Toolbar>
          <ScopeSwitch scope={scope} onChange={(next) => navigate(scopedHref(path, query, next), { replace: true, keepScroll: true })} />
        </Toolbar>
      ) : null}
      {approvals.isError ? <Notice>{errorText(approvals.error)}</Notice> : null}
      {decide.isError ? <Notice>{errorText(decide.error)}</Notice> : null}
      {cancel.isError ? <Notice>{errorText(cancel.error)}</Notice> : null}
      <div className={SPLIT}>
        <div className="flex min-w-0 flex-col gap-6">
          {approvals.isPending ? (
            <ul aria-hidden="true" className={LIST}>
              {[0, 1].map((row) => (
                <li key={row} className={`${SKELETON} h-32`} />
              ))}
            </ul>
          ) : list.length === 0 ? (
            <EmptyState icon="shield" title={t("approvals.empty.title")} body={t("approvals.empty.body")} />
          ) : (
            <LongList rows={list} textOf={(approval) => `${skillOf(approval.connector) ?? approval.function} ${titleOf(approval.connector)} ${approval.agent}`} keyOf={(approval) => approval.id} label={t("approvals.filter")} whole bar>
              {(shown) => (
                <ul aria-live="polite" className={LIST}>
                  {shown.map(({ row: approval, phase }) => (
                    <Held
                      key={approval.id}
                      phase={phase}
                      approval={approval}
                      view={heldViewOf(approval, viewer, modeOf(approval.connector))}
                      owner={everyone}
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
          <RecentDecisions decisions={decisions.data} named={named} owners={everyone} />
        </div>
        <aside className="flex min-w-0 flex-col gap-6">
          <AskFunctions connectors={connectors.data} />
          <ReachYou data={notifications.data} />
        </aside>
      </div>
    </section>
  );
}
