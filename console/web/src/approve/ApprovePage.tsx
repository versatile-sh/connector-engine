import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api, errorText, type ApprovalChoice, type Decision } from "../api";
import { Wordmark } from "../Brand";
import { useT } from "../i18n/I18nProvider";
import { LanguageSwitch } from "../LanguageSwitch";
import { approvalQuery } from "../queries";
import { navigate } from "../router";
import { Notice } from "../ui/Notice";
import { SKELETON, SURFACE, button } from "../ui/styles";
import { ApprovalCard } from "./ApprovalCard";

type ApprovePageProps = { id: string; token: string | null; signedIn: boolean };

const statusOf = (error: unknown): number => (typeof error === "object" && error !== null && "status" in error && typeof error.status === "number" ? error.status : 0);

function Frame({ children }: { children: ReactNode }) {
  return (
    <div className="relative isolate flex min-h-dvh flex-col overflow-x-clip">
      <div aria-hidden="true" className="backdrop-beam pointer-events-none absolute inset-x-0 top-0 -z-10 h-[28rem]" />
      <header className="mx-auto flex w-full max-w-xl flex-wrap items-center justify-between gap-3 px-5 pt-6">
        <Wordmark />
        <LanguageSwitch />
      </header>
      <main id="main" className="mx-auto flex w-full min-w-0 max-w-xl flex-1 flex-col justify-center px-5 py-10">
        {children}
      </main>
    </div>
  );
}

function Message({ title, body, action }: { title: string; body: string; action?: ReactNode }) {
  return (
    <section className={`${SURFACE} arrive flex flex-col items-center px-6 py-12 text-center`}>
      <h1 className="serif-room font-serif text-3xl leading-none font-normal tracking-[-0.02em] [overflow-wrap:anywhere]">{title}</h1>
      <p className="mt-3 max-w-sm text-muted">{body}</p>
      {action ? <div className="mt-6">{action}</div> : null}
    </section>
  );
}

/** The focused page a popup, a chat link or the console opens for one held call: it reads with the session, else the link token, decides once, and then shows the outcome read-only. */
export function ApprovePage({ id, token, signedIn }: ApprovePageProps) {
  const t = useT();
  const queryClient = useQueryClient();
  const link = signedIn ? null : token;
  const options = approvalQuery(id, link);
  const approval = useQuery(options);
  const decide = useMutation({
    mutationFn: (choice: ApprovalChoice) => api<Decision>("POST", `/api/approvals/${encodeURIComponent(id)}/decide`, link ? { choice, token: link } : { choice }),
    onSuccess: (result, choice) => {
      queryClient.setQueryData(options.queryKey, result.outcome);
      if (!signedIn) return;
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
      if (choice === "always") void queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
  const signIn = () => {
    navigate(`/approve/${encodeURIComponent(id)}`, { replace: true });
    void queryClient.invalidateQueries({ queryKey: ["session"] });
  };
  if (approval.data) {
    return (
      <Frame>
        <ApprovalCard
          approval={approval.data}
          signedIn={signedIn}
          first={!decide.data?.already_decided}
          deciding={decide.isPending ? decide.variables : undefined}
          error={decide.isError ? (statusOf(decide.error) === 401 ? t(link ? "approve.linkSpent" : "approve.sessionEnded") : errorText(decide.error)) : undefined}
          onChoose={(choice) => decide.mutate(choice)}
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
      <Frame>
        <div aria-hidden="true" className={`${SKELETON} h-96`} />
        <p role="status" className="sr-only">
          {t("app.loading")}
        </p>
      </Frame>
    );
  }
  const status = statusOf(approval.error);
  return (
    <Frame>
      {status === 401 ? (
        <Message
          title={t("approve.signInTitle")}
          body={t(link ? "approve.linkInvalid" : "approve.signInBody")}
          action={
            <button type="button" className={button({ variant: "accent" })} onClick={signIn}>
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
