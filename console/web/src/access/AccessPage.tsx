import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, errorText, type AgentToken } from "../api";
import { useLocale, useT } from "../i18n/I18nProvider";
import { keysQuery } from "../queries";
import { Bubble } from "../ui/Bubble";
import { CopyBox } from "../ui/CopyBox";
import { Field } from "../ui/Field";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { SURFACE, button } from "../ui/styles";
import { ClientPicker } from "./ClientPicker";

const intoView = (element: HTMLElement | null) => {
  element?.scrollIntoView({ block: "start", behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
};

function Created({ token }: { token: AgentToken }) {
  const t = useT();
  return (
    <section ref={intoView} aria-labelledby="created-title" className="arrive bubble-sent mb-8 scroll-mt-24 border border-lime/50 bg-ink-raised p-5 sm:p-6">
      <div className="flex items-start gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-lime text-ink">
          <Icon name="key" className="size-[1.1rem]" />
        </span>
        <div className="min-w-0">
          <h2 id="created-title" className="text-lg leading-snug font-medium">
            {t("keys.created", { name: token.agent })}
          </h2>
          <p className="text-sm text-muted">{t("keys.once")}</p>
        </div>
      </div>
      <CopyBox label={t("keys.key")} text={token.token} className="mt-4" />
      <div className="mt-6 border-t border-line pt-5">
        <ClientPicker url={token.mcp_url} accessKey={token.token} />
      </div>
    </section>
  );
}

/** Connect an AI app: one endpoint for every MCP client, access keys made here, and for a new key the exact way into each app. */
export function AccessPage() {
  const t = useT();
  const locale = useLocale();
  const queryClient = useQueryClient();
  const keys = useQuery(keysQuery);
  const [name, setName] = useState("");
  const [days, setDays] = useState(30);
  const mint = useMutation({
    mutationFn: () => api<AgentToken>("POST", "/api/agents", { name, ttl_hours: days * 24 }),
    onSuccess: () => {
      setName("");
      void queryClient.invalidateQueries({ queryKey: ["agents"] });
    },
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    mint.mutate();
  };
  const maxDays = Math.max(1, Math.floor((keys.data?.max_ttl_hours ?? 720) / 24));
  const endpoint = keys.data?.mcp_url ?? "";
  const list = keys.data?.agents ?? [];
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8">
        <h1 id="page-title" className="font-serif text-5xl leading-[0.98] font-normal tracking-[-0.02em] sm:text-6xl">
          {t("keys.title")}
        </h1>
        <p className="mt-3 max-w-2xl text-muted">{t("keys.intro")}</p>
        {endpoint ? <CopyBox label={t("keys.endpoint")} text={endpoint} tone="card" className="mt-5 max-w-xl" /> : null}
      </header>
      {mint.data ? <Created key={mint.data.token} token={mint.data} /> : null}
      <div className="grid items-start gap-6 lg:grid-cols-[22rem_minmax(0,1fr)]">
        <form onSubmit={onSubmit} aria-labelledby="new-key" className={`${SURFACE} p-5`}>
          <h2 id="new-key" className="mb-3 text-base font-medium">
            {t("keys.new")}
          </h2>
          <Field label={t("keys.name")} hint={t("keys.nameHint")} mono placeholder={t("keys.namePlaceholder")} value={name} onChange={(event) => setName(event.target.value)} required autoComplete="off" spellCheck={false} />
          <Field
            label={t("keys.days")}
            hint={t("keys.daysHint", { max: maxDays })}
            type="number"
            min={1}
            max={maxDays}
            value={days}
            onChange={(event) => setDays(Number(event.target.value))}
            error={mint.isError ? errorText(mint.error) : undefined}
          />
          <button type="submit" disabled={mint.isPending} className={`${button({ variant: "accent" })} w-full`}>
            <Icon name="key" className="size-4" />
            {mint.isPending ? t("keys.creating") : t("keys.create")}
          </button>
          <p className="mt-4 text-[0.8125rem] leading-snug text-muted">{t("keys.restart")}</p>
        </form>
        <section aria-labelledby="keys-title" className={`${SURFACE} p-5`}>
          <div className="mb-2 flex items-center gap-2.5">
            <h2 id="keys-title" className="text-base font-medium">
              {t("keys.list")}
            </h2>
            {keys.isSuccess ? <Bubble>{list.length}</Bubble> : null}
          </div>
          {keys.isError ? (
            <Notice>{errorText(keys.error)}</Notice>
          ) : keys.isPending ? (
            <ul aria-hidden="true">
              {[0, 1, 2].map((row) => (
                <li key={row} className="flex items-center justify-between border-b border-line py-3 last:border-b-0">
                  <div className="shimmer bubble-word h-4 w-32" />
                  <div className="shimmer bubble-word h-3 w-40" />
                </li>
              ))}
            </ul>
          ) : list.length === 0 ? (
            <p className="py-2 text-sm text-muted">{t("keys.empty", { endpoint })}</p>
          ) : (
            <ul>
              {list.map((key) => {
                const expired = new Date(key.expires_at) < new Date();
                return (
                  <li key={key.name} className="flex flex-wrap items-center gap-x-4 gap-y-1 border-b border-line py-3 last:border-b-0">
                    <span className={`size-2 shrink-0 rounded-full ${expired ? "bg-amber" : "bg-lime"}`} aria-hidden="true" />
                    <code className="min-w-0 flex-1 truncate font-mono text-sm font-semibold" translate="no">
                      {key.name}
                    </code>
                    <span className="font-mono text-xs text-muted">{t("keys.until", { date: new Date(key.expires_at).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) })}</span>
                    {expired ? <Bubble tone="amber">{t("keys.expired")}</Bubble> : null}
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      </div>
    </section>
  );
}
