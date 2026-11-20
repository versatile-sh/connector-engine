import { useMutation, useQuery } from "@tanstack/react-query";
import { api, errorText } from "../api";
import { Mark } from "../Brand";
import { useT } from "../i18n/I18nProvider";
import { LanguageSwitch } from "../LanguageSwitch";
import { Notice } from "../ui/Notice";
import { SURFACE, button } from "../ui/styles";

type ConsentRequest = { client: string; redirect: string; client_host?: string };

/** The owner's answer to an AI app asking to connect by OAuth: allow or deny, then the browser goes back to the app. */
export function Consent({ id }: { id: string }) {
  const t = useT();
  const path = `/api/oauth/requests/${encodeURIComponent(id)}`;
  const request = useQuery({ queryKey: ["oauth-request", id], queryFn: () => api<ConsentRequest>("GET", path), retry: false, staleTime: Infinity });
  const decide = useMutation({
    mutationFn: (approve: boolean) => api<{ redirect: string }>("POST", path, { approve }),
    onSuccess: (result) => window.location.assign(result.redirect),
  });
  const client = request.data?.client ?? "";
  const done = decide.isSuccess ? (decide.variables ? t("consent.done", { client }) : t("consent.denied", { client })) : undefined;
  return (
    <main className="grid min-h-dvh place-items-center px-6 py-10">
      <section aria-labelledby="consent-title" className={`${SURFACE} arrive w-full max-w-lg p-6 sm:p-8`}>
        <div className="mb-6 flex items-center justify-between">
          <Mark className="size-9" />
          <LanguageSwitch />
        </div>
        {request.isPending ? (
          <p role="status" className="text-muted">
            {t("app.loading")}
          </p>
        ) : request.isError ? (
          <>
            <h1 id="consent-title" className="font-display text-2xl font-bold tracking-[-0.03em]">
              {t("consent.expiredTitle")}
            </h1>
            <p className="mt-3 text-muted">{t("consent.expired")}</p>
          </>
        ) : (
          <>
            <h1 id="consent-title" className="font-display text-2xl leading-tight font-bold tracking-[-0.03em] sm:text-3xl">
              {t("consent.title", { client })}
            </h1>
            <p className="mt-3 text-muted">{t("consent.body", { client })}</p>
            <p className="mt-3 font-mono text-xs text-muted">
              {t("consent.returns", { target: request.data.redirect })}
              {request.data.client_host ? ` · ${t("consent.from", { host: request.data.client_host })}` : null}
            </p>
            {done ? (
              <p role="status" className="mt-6 font-semibold">
                {done}
              </p>
            ) : (
              <div className="mt-6 flex flex-wrap gap-3">
                <button type="button" className={button({ variant: "accent" })} disabled={decide.isPending} onClick={() => decide.mutate(true)}>
                  {t("consent.allow")}
                </button>
                <button type="button" className={button({ variant: "secondary" })} disabled={decide.isPending} onClick={() => decide.mutate(false)}>
                  {t("consent.deny")}
                </button>
              </div>
            )}
            {decide.isError ? (
              <div className="mt-4">
                <Notice>{errorText(decide.error)}</Notice>
              </div>
            ) : null}
          </>
        )}
      </section>
    </main>
  );
}
