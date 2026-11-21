import { useMutation, useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorText, type LinkFacts } from "../api";
import { PASSWORD_MIN, secretProblem } from "../account/backup";
import { SecretPair } from "../account/SecretPair";
import { Frame } from "../Frame";
import { useT } from "../i18n/I18nProvider";
import { acceptLink } from "../mutations";
import { linkQuery } from "../queries";
import { navigate } from "../router";
import { Display } from "../ui/Display";
import { EmptyState } from "../ui/EmptyState";
import { Field } from "../ui/Field";
import { Arrow } from "../ui/Icon";
import { Marked } from "../ui/Marked";
import { Notice } from "../ui/Notice";
import { SKELETON, SURFACE, button } from "../ui/styles";
import { isGone, linkViewOf, tokenOf } from "./invite";

function Use({ token, facts, onGone }: { token: string; facts: LinkFacts; onGone(): void }) {
  const t = useT();
  const join = facts.purpose === "join";
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState({ value: "", again: "" });
  const [checked, setChecked] = useState(false);
  const accept = useMutation(acceptLink);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setChecked(true);
    if (secretProblem(password.value, password.again, PASSWORD_MIN)) return;
    accept.mutate(
      { token, username: join ? username.trim() : "", password: password.value },
      { onSuccess: () => navigate("/", { replace: true }), onError: (error) => (isGone(error) ? onGone() : undefined) },
    );
  };
  return (
    <>
      <Display size="page">
        <Marked text={t(join ? "link.join.title" : "link.reset.title")} />
      </Display>
      <p className="mt-5 text-lg text-pretty text-muted">{join ? t("link.join.by", { name: facts.invited_by, role: t(`role.${facts.role}`) }) : t("link.reset.by", { name: facts.invited_by, user: facts.username })}</p>
      <form onSubmit={onSubmit} aria-label={t(join ? "link.join.submit" : "link.reset.submit")} className={`${SURFACE} mt-8 p-6 sm:p-8`}>
        {join ? <Field label={t("auth.username")} hint={t("account.usernameHint")} autoComplete="username" maxLength={64} required autoFocus value={username} onChange={(event) => setUsername(event.target.value)} /> : null}
        <SecretPair
          label={t("auth.password")}
          againLabel={t("account.againPassword")}
          hint={t("account.minHint", { min: PASSWORD_MIN })}
          min={PASSWORD_MIN}
          value={password.value}
          again={password.again}
          checked={checked}
          onChange={(value, again) => setPassword({ value, again })}
        />
        <p role="alert" className="mt-1 min-h-6 text-sm text-amber-400">
          {accept.isError && !isGone(accept.error) ? errorText(accept.error) : ""}
        </p>
        <button type="submit" disabled={accept.isPending} className={`${button()} mt-2 w-full`}>
          {accept.isPending ? t("auth.working") : t(join ? "link.join.submit" : "link.reset.submit")}
          <Arrow />
        </button>
      </form>
    </>
  );
}

/** The page a join or reset link opens, outside the sections: it reads the token from the address fragment once, asks the console what the link is for, and shows the form to join with a name and a password, the form to set a new password, or, for a link that no longer works, who to ask for a new one. */
export function InvitePage() {
  const t = useT();
  const [token] = useState(() => tokenOf(window.location.hash));
  const [spent, setSpent] = useState(false);
  const facts = useQuery({ ...linkQuery(token ?? ""), enabled: token !== undefined });
  const view = spent ? { kind: "gone" as const } : linkViewOf(token, facts.data, facts.error);
  return (
    <Frame width="narrow" home>
      {view.kind === "reading" ? (
        <div aria-hidden="true" className={`${SKELETON} h-80`} />
      ) : view.kind === "gone" ? (
        <EmptyState heading="h1" title={t("link.gone.title")} body={t("link.gone.body")}>
          <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={() => navigate("/", { replace: true })}>
            {t("link.gone.action")}
          </button>
        </EmptyState>
      ) : view.kind === "failed" ? (
        <Notice
          action={
            <button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={() => void facts.refetch()}>
              {t("common.retry")}
            </button>
          }
        >
          {errorText(view.error)}
        </Notice>
      ) : token ? (
        <Use token={token} facts={view.facts} onGone={() => setSpent(true)} />
      ) : null}
    </Frame>
  );
}
