import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { errorText, setSetupToken } from "./api";
import { Frame } from "./Frame";
import { useT } from "./i18n/I18nProvider";
import { linkOf } from "./import/source";
import { signIn, signUp } from "./mutations";
import { PASSWORD_MIN, secretProblem } from "./account/backup";
import { SecretPair } from "./account/SecretPair";
import { ImportDialog } from "./account/ImportDialog";
import { Field } from "./ui/Field";
import { Display } from "./ui/Display";
import { Arrow } from "./ui/Icon";
import { CAPTION, SURFACE, button } from "./ui/styles";

type AuthProps = { mode: "setup" | "login"; endpoint: string; waitingHost?: string; needsSetupToken?: boolean; signupOpen?: boolean; startCreating?: boolean };

const hostOf = (raw: string) => {
  try {
    return new URL(raw).host;
  } catch {
    return raw;
  }
};

/** The host an install link in the address bar points at, if the page is one; nothing of it is fetched before sign-in. */
export function waitingLinkHost(path: string, query: URLSearchParams): string | undefined {
  const link = linkOf(path, query);
  const address = link && ("listing" in link ? link.listing : link.mcp);
  return address ? hostOf(address) : undefined;
}

function Headline({ waitingHost }: { waitingHost?: string | undefined }) {
  const t = useT();
  return (
    <div>
      <Display size="hero">
        <span className="block">{t(waitingHost ? "auth.linkLine1" : "auth.line1")}</span>
        <em className="block text-lime">{t(waitingHost ? "auth.linkLine2" : "auth.line2")}</em>
      </Display>
      <p className="mt-6 max-w-lg text-lg text-pretty text-muted">{waitingHost ? t("auth.linkBody", { host: waitingHost }) : t("auth.lead")}</p>
    </div>
  );
}

function Points({ endpoint }: { endpoint: string }) {
  const t = useT();
  const where = hostOf(endpoint) + new URL(endpoint, window.location.origin).pathname;
  const points = [
    { title: t("auth.point1.title"), body: t("auth.point1.body") },
    { title: t("auth.point2.title"), body: t("auth.point2.body") },
    { title: t("auth.point3.title"), body: t("auth.point3.body") },
  ];
  return (
    <div className="@container">
      <ul className="max-w-xl border-t border-line">
        {points.map((point) => (
          <li key={point.title} className="grid gap-1 border-b border-line py-4 @md:grid-cols-[10rem_minmax(0,1fr)] @md:gap-6">
            <p className="font-medium">{point.title}</p>
            <p className="text-pretty text-muted">{point.body}</p>
          </li>
        ))}
      </ul>
      <p className={`${CAPTION} mt-4 flex items-center gap-2`}>
        <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-lime" />
        <span className="min-w-0 [overflow-wrap:anywhere]" translate="no">
          {where}
        </span>
      </p>
    </div>
  );
}

/** First-visit owner creation or sign-in as an editorial page: the serif headline with its lime answer, the form in a card beside it, and what the gateway does under the headline. An install link waiting in the address bar is named in the headline and opens once signed in. */
export function Auth({ mode, endpoint, waitingHost, needsSetupToken, signupOpen, startCreating }: AuthProps) {
  const t = useT();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");
  const [creating, setCreating] = useState(startCreating === true && mode === "login" && signupOpen === true);
  const [checked, setChecked] = useState(false);
  const enter = useMutation(signIn);
  const join = useMutation(signUp);
  const submit = creating ? join : enter;
  const [restoring, setRestoring] = useState(false);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!creating) return enter.mutate({ mode, username, password });
    setChecked(true);
    if (!secretProblem(password, again, PASSWORD_MIN)) join.mutate({ username: username.trim(), password });
  };
  const switchTo = (next: boolean) => {
    setCreating(next);
    setChecked(false);
    setAgain("");
    setPassword("");
    enter.reset();
    join.reset();
  };
  const heading = creating ? "auth.signup" : mode === "setup" ? "auth.setup" : "auth.login";
  return (
    <Frame width="wide">
      <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_26rem] lg:grid-rows-[auto_1fr] lg:gap-x-20 lg:gap-y-12">
        <Headline waitingHost={waitingHost} />
        <form className={`${SURFACE} w-full max-w-lg p-6 sm:p-8 lg:col-start-2 lg:max-w-none lg:row-span-2 lg:row-start-1 lg:self-center`} onSubmit={onSubmit} aria-labelledby="auth-title">
          <h2 id="auth-title" className="text-2xl leading-tight font-medium">
            {t(`${heading}.title`)}
          </h2>
          <p className="mt-2 mb-7 text-pretty text-muted">{t(`${heading}.intro`)}</p>
          <Field label={t("auth.username")} className="mb-2" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus />
          {needsSetupToken ? <Field label={t("auth.setupToken")} hint={t("auth.setupTokenHint")} className="mb-2" autoComplete="off" onChange={(e) => setSetupToken(e.target.value)} required /> : null}
          {creating ? (
            <SecretPair label={t("auth.password")} againLabel={t("account.againPassword")} hint={t("account.minHint", { min: PASSWORD_MIN })} min={PASSWORD_MIN} value={password} again={again} checked={checked} onChange={(value, repeat) => (setPassword(value), setAgain(repeat))} />
          ) : (
            <Field
              label={t("auth.password")}
              hint={mode === "setup" ? t("auth.passwordHint") : undefined}
              type="password"
              autoComplete={mode === "setup" ? "new-password" : "current-password"}
              minLength={mode === "setup" ? 10 : undefined}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          )}
          <p role="alert" className="mt-1 min-h-6 text-sm text-amber-400">
            {submit.isError ? errorText(submit.error) : ""}
          </p>
          <button type="submit" disabled={submit.isPending} className={`${button()} mt-2 w-full`}>
            {submit.isPending ? t("auth.working") : t(`${heading}.submit`)}
            <Arrow />
          </button>
          {mode === "setup" ? (
            <button type="button" className={`${button({ variant: "secondary" })} mt-3 w-full`} onClick={() => setRestoring(true)}>
              {t("auth.restore")}
            </button>
          ) : signupOpen ? (
            <button type="button" className={`${button({ variant: "secondary" })} mt-3 w-full`} onClick={() => switchTo(!creating)}>
              {t(creating ? "auth.signup.back" : "auth.signup.open")}
            </button>
          ) : (
            <p className="mt-4 text-sm text-pretty text-muted">{t("auth.signup.closed")}</p>
          )}
          <p className="mt-6 border-t border-line pt-5 text-sm text-muted">{t("auth.local")}</p>
        </form>
        {waitingHost ? null : <Points endpoint={endpoint} />}
      </div>
      {restoring ? <ImportDialog restoreOnly onClose={() => setRestoring(false)} /> : null}
    </Frame>
  );
}
