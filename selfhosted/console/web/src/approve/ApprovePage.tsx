import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { errorText } from "../api";
import { Frame } from "../Frame";
import { useT } from "../i18n/I18nProvider";
import { decideApproval } from "../mutations";
import { approvalQuery, sessionQuery } from "../queries";
import { navigate } from "../router";
import { Display } from "../ui/Display";
import { Notice } from "../ui/Notice";
import { SKELETON, SURFACE, button } from "../ui/styles";
import { ApprovalCard } from "./ApprovalCard";

type ApprovePageProps = { id: string; token: string | null; signedIn: boolean };

const statusOf = (error: unknown): number => (typeof error === "object" && error !== null && "status" in error && typeof error.status === "number" ? error.status : 0);

function Message({ title, body, action }: { title: string; body: string; action?: ReactNode }) {
  return (
    <section className={`${SURFACE} arrive px-6 py-10 sm:px-8`}>
      <Display size="title" className="[overflow-wrap:anywhere]">
        {title}
      </Display>
      <p className="mt-4 max-w-sm text-pretty text-muted">{body}</p>
      {action ? <div className="mt-6">{action}</div> : null}
    </section>
  );
}

/** The focused page a popup, a chat link or the console opens for one held call: it reads with the session, else the link token, decides once, and then shows the outcome read-only. */
export function ApprovePage({ id, token, signedIn }: ApprovePageProps) {
  const t = useT();
  const queryClient = useQueryClient();
  const link = signedIn ? null : token;
  const approval = useQuery(approvalQuery(id, link));
  const decide = useMutation(decideApproval);
  const signIn = () => {
    navigate(`/approve/${encodeURIComponent(id)}`, { replace: true });
    void queryClient.invalidateQueries({ queryKey: sessionQuery.queryKey });
  };
  if (approval.data) {
    return (
      <Frame width="narrow">
        <ApprovalCard
          approval={approval.data}
          signedIn={signedIn}
          first={!decide.data?.already_decided}
          deciding={decide.isPending ? decide.variables.choice : undefined}
          error={decide.isError ? (statusOf(decide.error) === 401 ? t(link ? "approve.linkSpent" : "approve.sessionEnded") : errorText(decide.error)) : undefined}
          onChoose={(choice) => decide.mutate({ id, choice, token: link })}
        />
        {decide.isError && statusOf(decide.error) === 401 ? (
          <button type="button" className={`${button({ variant: "secondary", size: "sm" })} mt-4 self-center`} onClick={signIn}>
            {t("approve.signIn")}
          </button>
        ) : null}
      </Frame>
    );
  }
  if (approval.isPending) {
    return (
      <Frame width="narrow">
        <div aria-hidden="true" className={`${SKELETON} h-96`} />
        <p role="status" className="sr-only">
          {t("app.loading")}
        </p>
      </Frame>
    );
  }
  const status = statusOf(approval.error);
  return (
    <Frame width="narrow">
      {status === 401 ? (
        <Message
          title={t("approve.signInTitle")}
          body={t(link ? "approve.linkInvalid" : "approve.signInBody")}
          action={
            <button type="button" className={button({ variant: "primary" })} onClick={signIn}>
              {t("approve.signIn")}
            </button>
          }
        />
      ) : status === 404 ? (
        <Message title={t("approve.missingTitle")} body={t("approve.missingBody")} />
      ) : (
        <Notice
          action={
            <button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={() => void approval.refetch()}>
              {t("common.retry")}
            </button>
          }
        >
          {errorText(approval.error)}
        </Notice>
      )}
    </Frame>
  );
}
