import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useReducer, useState, type FormEvent } from "react";
import { api, errorText, type AgentToken } from "../api";
import { useT } from "../i18n/I18nProvider";
import { keysQuery } from "../queries";
import { Bubble } from "../ui/Bubble";
import { CopyBox } from "../ui/CopyBox";
import { Field } from "../ui/Field";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { HAIRLINE, SURFACE, button } from "../ui/styles";
import { ClientPicker } from "./ClientPicker";
import { KeyRow } from "./KeyRow";
import { revokeStep, type RevokeEvent } from "./revoke";

const intoView = (element: HTMLElement | null) => {
  element?.scrollIntoView({ block: "start", behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
};

function Created({ token }: { token: AgentToken }) {
  const t = useT();
  return (
    <section ref={intoView} aria-labelledby="created-title" className="arrive vignette-top bubble-sent relative mb-8 scroll-mt-24 overflow-hidden border border-lime/50 bg-ink-raised p-5 sm:p-6">
      <span aria-hidden="true" className={HAIRLINE} />
      <div className="flex items-start gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-full bg-lime text-ink">
          <Icon name="key" className="size-[1.1rem]" />
        </span>
        <div className="min-w-0">
          <h2 id="created-title" className="text-lg leading-snug font-medium [overflow-wrap:anywhere]">
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

/** Connect an AI app: one endpoint for every MCP client, access keys made and revoked here, and for a new key the exact way into each app. */
export function AccessPage() {
  const t = useT();
  const queryClient = useQueryClient();
  const keys = useQuery(keysQuery);
  const [name, setName] = useState("");
  const [days, setDays] = useState(30);
  const mint = useMutation({
    mutationFn: () => api<AgentToken>("POST", "/api/agents", { name, ttl_hours: days * 24 }),
    onSuccess: () => {
      setName("");
      void queryClient.invalidateQueries({ queryKey: ["agents"] });
      void queryClient.invalidateQueries({ queryKey: ["session"] });
    },
  });
  const [flow, dispatch] = useReducer(revokeStep, { kind: "idle" });
  const revoke = useMutation({
    mutationFn: (agent: string) => api("DELETE", `/api/agents/${encodeURIComponent(agent)}`),
    onSuccess: async (_, agent) => {
      dispatch({ type: "revoked" });
      if (mint.data?.agent === agent) mint.reset();
      await queryClient.invalidateQueries({ queryKey: ["session"] });
      await queryClient.invalidateQueries({ queryKey: ["agents"] });
    },
    onError: () => dispatch({ type: "failed" }),
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    mint.mutate();
  };
  const onRevokeEvent = (event: RevokeEvent) => {
    if (event.type === "ask") revoke.reset();
    dispatch(event);
  };
  const onConfirm = () => {
    if (flow.kind !== "confirming") return;
    dispatch({ type: "confirm" });
    revoke.mutate(flow.name);
  };
  const maxDays = Math.max(1, Math.floor((keys.data?.max_ttl_hours ?? 720) / 24));
  const endpoint = keys.data?.mcp_url ?? "";
  const list = keys.data?.agents ?? [];
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8">
        <PageTitle section={t("nav.access")} title={t("keys.title")} />
        <p className="mt-3 max-w-2xl text-muted">{t("keys.intro")}</p>
      </header>
      {endpoint ? (
        <section aria-label={t("clients.title")} className={`${SURFACE} mb-10 p-5`}>
          <ClientPicker url={endpoint} />
        </section>
      ) : null}
      <h2 className="font-display text-xl font-bold tracking-[-0.03em]">{t("keys.advanced")}</h2>
      <p className="mt-1 mb-5 max-w-2xl text-sm text-muted">{t("keys.advancedIntro")}</p>
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
              {list.map((key) => (
                <KeyRow key={key.name} entry={key} flow={flow} error={revoke.isError ? errorText(revoke.error) : undefined} dispatch={onRevokeEvent} onConfirm={onConfirm} />
              ))}
            </ul>
          )}
        </section>
      </div>
    </section>
  );
}
