import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, errorText, setCsrf } from "./api";
import { Wordmark } from "./Brand";
import { useT } from "./i18n/I18nProvider";
import { LanguageSwitch } from "./LanguageSwitch";
import { Said, Split } from "./Split";
import { Field } from "./ui/Field";
import { Arrow } from "./ui/Icon";
import { button } from "./ui/styles";

type AuthProps = { mode: "setup" | "login"; endpoint: string; waitingHost?: string };

const hostOf = (raw: string) => {
  try {
    return new URL(raw).host;
  } catch {
    return raw;
  }
};

/** The host an install link in the address bar points at, if the page is one; nothing of it is fetched before sign-in. */
export function waitingLinkHost(path: string, query: URLSearchParams): string | undefined {
  const link = path === "/import" ? (query.get("listing") ?? query.get("mcp")) : null;
  return link ? hostOf(link) : undefined;
}

function Pitch({ endpoint, waitingHost }: { endpoint: string; waitingHost?: string }) {
  const t = useT();
  const where = hostOf(endpoint) + new URL(endpoint, window.location.origin).pathname;
  return (
    <>
      <Wordmark onAccent className="self-start" />
      <div className="flex flex-1 flex-col justify-center pt-8 pb-2 lg:py-0">
        <h1 className="font-display text-[2.25rem] leading-[1.04] font-bold tracking-[-0.04em] sm:text-5xl xl:text-6xl">
          <span className="block">{t(waitingHost ? "auth.linkLine1" : "auth.line1")}</span>
          <span className="block">{t(waitingHost ? "auth.linkLine2" : "auth.line2")}</span>
          <span className="bubble-word mt-[0.34em] inline-block bg-ink px-[0.5em] py-[0.24em] font-mono text-[0.42em] leading-[1.15] font-medium tracking-[-0.02em] text-bubble">
            {waitingHost ?? t("auth.answer")}
          </span>
        </h1>
        {waitingHost ? (
          <p className="mt-6 max-w-md text-ink/80">{t("auth.linkBody", { host: waitingHost })}</p>
        ) : (
          <ul aria-label={t("auth.answer")} className="mt-10 hidden max-w-md flex-col gap-2.5 sm:flex">
            <Said>
              <strong className="font-semibold">{t("auth.point1.title")}.</strong> {t("auth.point1.body")}
            </Said>
            <Said>
              <strong className="font-semibold">{t("auth.point2.title")}.</strong> {t("auth.point2.body")}
            </Said>
            <Said>
              <strong className="font-semibold">{t("auth.point3.title")}.</strong> {t("auth.point3.body", { endpoint: where })}
            </Said>
            <Said mine live>
              <span translate="no">{where}</span>
            </Said>
          </ul>
        )}
      </div>
    </>
  );
}

/** First-visit owner creation or sign-in as the live front page: the pitch on lime, the form on ink. An install link waiting in the address bar is named on the lime half and opens once signed in. */
export function Auth({ mode, endpoint, waitingHost }: AuthProps) {
  const t = useT();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const submit = useMutation({
    mutationFn: () => api<{ csrf: string }>("POST", mode === "setup" ? "/api/setup" : "/api/login", { username, password }),
    onSuccess: (result) => {
      setCsrf(result.csrf);
      void queryClient.invalidateQueries({ queryKey: ["session"] });
    },
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    submit.mutate();
  };
  return (
    <Split
      pitch={<Pitch endpoint={endpoint} waitingHost={waitingHost} />}
      panel={
        <>
          <LanguageSwitch className="self-end" />
          <div className="flex flex-1 items-center justify-center py-8">
            <form className="ride-in w-full max-w-md" onSubmit={onSubmit} aria-labelledby="auth-title">
              <h2 id="auth-title" className="font-display text-3xl leading-[1.1] font-bold tracking-[-0.04em]">
                {t(mode === "setup" ? "auth.setup.title" : "auth.login.title")}
              </h2>
              <p className="mt-2 mb-7 text-muted">{t(mode === "setup" ? "auth.setup.intro" : "auth.login.intro")}</p>
              <Field label={t("auth.username")} className="mb-2" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus />
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
              <p role="alert" className="mt-1 min-h-6 text-sm text-amber">
                {submit.isError ? errorText(submit.error) : ""}
              </p>
              <button type="submit" disabled={submit.isPending} className={`${button({ variant: "accent", size: "lg" })} mt-2 w-full`}>
                {submit.isPending ? t("auth.working") : t(mode === "setup" ? "auth.setup.submit" : "auth.login.submit")}
                <Arrow />
              </button>
              <p className="mt-6 border-t border-line pt-5 text-sm text-muted">{t("auth.local")}</p>
            </form>
          </div>
        </>
      }
    />
  );
}
