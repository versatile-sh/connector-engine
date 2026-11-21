import { useMutation, useQuery } from "@tanstack/react-query";
import { useReducer, useState, type FormEvent } from "react";
import { errorText, type AgentToken } from "../api";
import { useT } from "../i18n/I18nProvider";
import { mintKey, revokeKey } from "../mutations";
import { ScopeSwitch } from "../people/ScopeSwitch";
import { useViewer } from "../people/useViewer";
import { offersScope, scopedHref, scopeOf } from "../people/viewer";
import { scopedKeysQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { CopyBox } from "../ui/CopyBox";
import { Field } from "../ui/Field";
import { Icon } from "../ui/Icon";
import { LongList } from "../ui/LongList";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { HAIRLINE, LEAD, PAGE, SKELETON_LINE, SUBTITLE, SURFACE, TITLE, WELL, button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { ClientPicker } from "./ClientPicker";
import { KeyRow } from "./KeyRow";
import { SlotPicker } from "./SlotPicker";
import { UnclaimedSlots } from "./UnclaimedSlots";
import { revokeStep, type RevokeEvent } from "./revoke";

const intoView = (element: HTMLElement | null) => {
  element?.scrollIntoView({ block: "start", behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
};

function Created({ token, endpoint }: { token: AgentToken; endpoint: string }) {
  const t = useT();
  return (
    <section ref={intoView} aria-labelledby="created-title" className={`${WELL} arrive relative scroll-mt-24 overflow-hidden p-5 sm:p-6`}>
      <span aria-hidden="true" className={HAIRLINE} />
      <div className="flex items-start gap-3">
        <span className="grid size-9 shrink-0 place-items-center rounded-full border border-line-strong text-lime">
          <Icon name="key" className="size-[1.1rem]" />
        </span>
        <div className="min-w-0">
          <h2 id="created-title" className={`${TITLE} [overflow-wrap:anywhere]`}>
            {t("keys.created", { name: token.agent })}
          </h2>
          <p className="text-sm text-muted">{t("keys.once")}</p>
        </div>
      </div>
      <CopyBox label={t("keys.key")} text={token.token} className="mt-4" />
      <div className="mt-6 border-t border-line pt-5">
        <ClientPicker url={endpoint} accessKey={token.token} heading="h3" />
      </div>
    </section>
  );
}

/** Connect an AI app: one endpoint for every MCP client in a card of its own, then, under Advanced, a card to make an access key beside the card of the keys made and revoked here, and for a new key the exact way into each app. Once a second user exists, a viewer who may see everyone's keys picks Mine or Everyone in the list's heading, and each key then names its owner. */
export function AccessPage({ endpoint }: { endpoint: string }) {
  const t = useT();
  const { path, query } = useLocation();
  const offered = offersScope(useViewer(), "keys.any");
  const scope = scopeOf(query, offered);
  const keys = useQuery(scopedKeysQuery(scope));
  const [name, setName] = useState("");
  const [days, setDays] = useState(30);
  const [slot, setSlot] = useState("");
  const mint = useMutation(mintKey);
  const [flow, dispatch] = useReducer(revokeStep, { kind: "idle" });
  const revoke = useMutation(revokeKey);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    mint.mutate(
      { name, ttl_hours: days * 24, slot: slot || undefined },
      {
        onSuccess: () => {
          setName("");
          setSlot("");
        },
      },
    );
  };
  const onRevokeEvent = (event: RevokeEvent) => {
    if (event.type === "ask") revoke.reset();
    dispatch(event);
  };
  const onConfirm = () => {
    if (flow.kind !== "confirming") return;
    dispatch({ type: "confirm" });
    revoke.mutate(flow.name, {
      onSuccess: (_, agent) => {
        dispatch({ type: "revoked" });
        if (mint.data?.agent === agent) mint.reset();
      },
      onError: () => dispatch({ type: "failed" }),
    });
  };
  const maxDays = Math.max(1, Math.floor((keys.data?.max_ttl_hours ?? 720) / 24));
  const list = keys.data?.agents ?? [];
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader title={t("keys.title")}>{t("keys.intro")}</PageHeader>
      <section aria-label={t("clients.title")} className={`${SURFACE} p-5 sm:p-6`}>
        <ClientPicker url={endpoint} />
      </section>
      <section aria-labelledby="advanced-title" className="mt-4 flex flex-col gap-6">
        <div>
          <h2 id="advanced-title" className={TITLE}>
            {t("keys.advanced")}
          </h2>
          <p className={`${LEAD} mt-1 text-sm`}>{t("keys.advancedIntro")}</p>
        </div>
        {mint.data ? <Created key={mint.data.token} token={mint.data} endpoint={endpoint} /> : null}
        <div className="grid items-start gap-6 lg:grid-cols-[22rem_minmax(0,1fr)] lg:gap-10">
          <form onSubmit={onSubmit} aria-labelledby="new-key" className={`${SURFACE} p-5`}>
            <h2 id="new-key" className={`${SUBTITLE} mb-4`}>
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
            <SlotPicker value={slot} onChange={setSlot} className="mb-5" />
            <button type="submit" disabled={mint.isPending} className={`${button({ variant: "primary" })} w-full`}>
              <Icon name="key" className="size-4" />
              {mint.isPending ? t("keys.creating") : t("keys.create")}
            </button>
            <p className="mt-4 text-[0.8125rem] leading-snug text-pretty text-muted">{t("keys.skillsOff")}</p>
          </form>
          <section aria-labelledby="keys-title" className={`${SURFACE} p-5`}>
            <div className={offered ? "mb-3 flex flex-wrap items-center gap-x-3 gap-y-2" : "mb-3 flex items-baseline gap-3"}>
              <h2 id="keys-title" className={SUBTITLE}>
                {t("keys.list")}
              </h2>
              {keys.isSuccess ? <Tag>{scope === "everyone" ? `${t("scope.count")} · ${list.length}` : list.length}</Tag> : null}
              {offered ? <ScopeSwitch scope={scope} onChange={(next) => navigate(scopedHref(path, query, next), { replace: true, keepScroll: true })} className="ml-auto" /> : null}
            </div>
            {keys.isError ? (
              <Notice>{errorText(keys.error)}</Notice>
            ) : keys.isPending ? (
              <ul aria-hidden="true" className="-mx-5 border-t border-line">
                {[0, 1, 2].map((row) => (
                  <li key={row} className="flex items-center justify-between border-b border-line px-4 py-3.5 last:border-b-0">
                    <div className={`${SKELETON_LINE} h-4 w-32`} />
                    <div className={`${SKELETON_LINE} h-3 w-40`} />
                  </li>
                ))}
              </ul>
            ) : list.length === 0 ? (
              <p className="py-2 text-sm text-pretty text-muted">{t("keys.empty")}</p>
            ) : (
              <LongList rows={list} textOf={(key) => key.name} keyOf={(key) => key.name} label={t("keys.filter")}>
                {(shown) => (
                  <ul className="@container -mx-5 mt-3 border-t border-line">
                    {shown.map(({ row: key, phase }) => (
                      <KeyRow key={key.name} phase={phase} entry={key} owner={scope === "everyone" ? key.owner : undefined} flow={flow} error={revoke.isError ? errorText(revoke.error) : undefined} dispatch={onRevokeEvent} onConfirm={onConfirm} />
                    ))}
                  </ul>
                )}
              </LongList>
            )}
          </section>
        </div>
      </section>
      <UnclaimedSlots />
    </section>
  );
}
