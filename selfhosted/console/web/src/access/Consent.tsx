import { useMutation, useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, errorCode, errorText } from "../api";
import type { MessageKey } from "../i18n";
import { useT } from "../i18n/I18nProvider";
import { Frame } from "../Frame";
import { decideConsent } from "../mutations";
import { Field } from "../ui/Field";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { Display } from "../ui/Display";
import { CAPTION, SURFACE, button } from "../ui/styles";

type Destination = { kind: "loopback"; port?: string } | { kind: "app"; app: string } | { kind: "scheme"; scheme: string };

type ConsentRequest = { client: string; redirect: string; destination: Destination };

const PASSWORD_REFUSALS: Partial<Record<string, MessageKey>> = { wrong_password: "consent.wrongPassword", slow_down: "consent.tooMany", password_required: "consent.passwordHint" };

const whereTo = (destination: Destination, t: ReturnType<typeof useT>) => {
  switch (destination.kind) {
    case "loopback":
      return destination.port ? t("consent.to.loopback", { port: destination.port }) : t("consent.to.loopbackAnyPort");
    case "app":
      return destination.app;
    case "scheme":
      return t("consent.to.scheme", { scheme: destination.scheme });
  }
};

/** The owner's answer to an AI app asking to connect by OAuth: a warning that the app named itself, where the access goes, allow with the owner password or deny, then the browser goes back to the app. */
export function Consent({ id }: { id: string }) {
  const t = useT();
  const path = `/api/oauth/requests/${encodeURIComponent(id)}`;
  const request = useQuery({ queryKey: ["oauth-request", id], queryFn: () => api<ConsentRequest>("GET", path), retry: false, staleTime: Infinity });
  const [password, setPassword] = useState("");
  const decide = useMutation(decideConsent);
  const answer = (approve: boolean) => decide.mutate({ request: id, approve, password }, { onSuccess: (result) => window.location.assign(result.redirect) });
  const refusal = decide.isError ? PASSWORD_REFUSALS[errorCode(decide.error) ?? ""] : undefined;
  const onAllow = (event: FormEvent) => {
    event.preventDefault();
    answer(true);
  };
  const client = request.data?.client ?? "";
  const done = decide.isSuccess ? (decide.variables.approve ? t("consent.done", { client }) : t("consent.denied", { client })) : undefined;
  return (
    <Frame width="narrow">
      <section aria-labelledby="consent-title" className={`${SURFACE} arrive p-6 sm:p-8`}>
        {request.isPending ? (
          <p role="status" className="text-muted">
            {t("app.loading")}
          </p>
        ) : request.isError ? (
          <>
            <Display id="consent-title" size="title">
              {t("consent.expiredTitle")}
            </Display>
            <p className="mt-3 text-pretty text-muted">{t("consent.expired")}</p>
          </>
        ) : (
          <>
            <Notice live={false} className="mb-6">
              <span className="flex items-start gap-2.5">
                <Icon name="shield" className="mt-0.5 size-4 text-amber-400" />
                <span>
                  <strong className="font-medium text-amber-400">{t("consent.warning")}</strong> {t("consent.selfNamed")}
                </span>
              </span>
            </Notice>
            <Display id="consent-title" size="title" className="[overflow-wrap:anywhere]">
              {t("consent.title", { client })}
            </Display>
            <p className="mt-4 text-pretty text-muted">{t("consent.body", { client })}</p>
            <p className="mt-5 text-sm">
              <span className="text-muted">{t("consent.goesTo")}</span> <strong className="font-medium">{whereTo(request.data.destination, t)}</strong>
            </p>
            <p className={`${CAPTION} mt-1 [overflow-wrap:anywhere]`} translate="no">
              {t("consent.returns", { target: request.data.redirect })}
            </p>
            {done ? (
              <Notice tone="lime" className="mt-6">
                {done}
              </Notice>
            ) : (
              <form className="mt-6" onSubmit={onAllow}>
                <Field
                  label={t("consent.password")}
                  hint={t("consent.passwordHint")}
                  error={refusal ? t(refusal) : undefined}
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  required
                />
                <div className="mt-1 flex flex-wrap gap-3">
                  <button type="submit" className={button()} disabled={decide.isPending}>
                    {t("consent.allow")}
                  </button>
                  <button type="button" className={button({ variant: "secondary" })} disabled={decide.isPending} onClick={() => answer(false)}>
                    {t("common.deny")}
                  </button>
                </div>
              </form>
            )}
            {decide.isError && !refusal ? <Notice className="mt-4">{errorText(decide.error)}</Notice> : null}
          </>
        )}
      </section>
    </Frame>
  );
}
