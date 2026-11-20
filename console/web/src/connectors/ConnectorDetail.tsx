import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, errorText, type Access, type Connector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { connectorsQuery } from "../queries";
import { navigate } from "../router";
import { Bubble } from "../ui/Bubble";
import { Field } from "../ui/Field";
import { Arrow, Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { Sheet } from "../ui/Sheet";
import { SKELETON, SURFACE, button } from "../ui/styles";
import { AccessPicker } from "./AccessPicker";
import { ConnectorIcon } from "./ConnectorIcon";
import { Credentials } from "./Credentials";
import { FunctionRow } from "./FunctionRow";
import { accessCounts, serverHost } from "./summary";

const FILTER_FROM = 8;

function Back() {
  const t = useT();
  return (
    <a
      href="/"
      onClick={(event) => {
        event.preventDefault();
        navigate("/");
      }}
      className={`${button({ variant: "quiet", size: "sm" })} -ml-4 mb-4 self-start`}
    >
      <Arrow back />
      {t("detail.back")}
    </a>
  );
}

function Functions({ connector }: { connector: Connector }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState("");
  const setAccess = useMutation({
    mutationFn: ({ name, access }: { name: string; access: Access }) =>
      api("PUT", `/api/connectors/${encodeURIComponent(connector.id)}/functions/${encodeURIComponent(name)}`, { access }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["connectors"] }),
  });
  const functions = connector.functions ?? [];
  const needle = filter.trim().toLowerCase();
  const shown = functions.filter((fn) => `${fn.name} ${fn.title ?? ""} ${fn.description ?? ""}`.toLowerCase().includes(needle));
  const counts = accessCounts(connector);
  return (
    <section aria-labelledby="functions-title" className={`${SURFACE} min-w-0 p-5 sm:p-6`}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <h2 id="functions-title" className="text-lg leading-snug font-medium">
            {t("detail.functions")}
          </h2>
          <Bubble>{functions.length}</Bubble>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {(["read", "write", "ask", "off"] as const).map((access) =>
            counts[access] > 0 ? (
              <Bubble key={access} tone={access === "ask" ? "outline" : "neutral"}>
                {t(`access.count.${access}`, { count: counts[access] })}
              </Bubble>
            ) : null,
          )}
        </div>
      </div>
      {functions.length >= FILTER_FROM ? (
        <Field label={t("detail.filter")} className="mt-4" type="search" mono value={filter} onChange={(event) => setFilter(event.target.value)} autoComplete="off" spellCheck={false} />
      ) : null}
      <p role="alert" className="min-h-0 text-sm text-amber empty:hidden">
        {setAccess.isError ? errorText(setAccess.error) : ""}
      </p>
      {functions.length === 0 ? (
        <p className="py-6 text-sm text-muted">{t("detail.none")}</p>
      ) : shown.length === 0 ? (
        <p className="py-6 text-sm text-muted">{t("detail.noMatch")}</p>
      ) : (
        <ul className="mt-2">
          {shown.map((fn) => (
            <FunctionRow
              key={fn.name}
              name={fn.name}
              title={fn.title}
              description={fn.description}
              flags={
                <>
                  <Bubble>{fn.class || t("detail.unclassified")}</Bubble>
                  {fn.state !== "active" ? (
                    <Bubble tone="amber" title={fn.state === "quarantined" ? t("detail.quarantined") : undefined}>
                      {fn.state}
                    </Bubble>
                  ) : null}
                </>
              }
            >
              <AccessPicker
                name={fn.name}
                value={setAccess.isPending && setAccess.variables.name === fn.name ? setAccess.variables.access : fn.access}
                disabled={setAccess.isPending && setAccess.variables.name === fn.name}
                onChange={(access) => setAccess.mutate({ name: fn.name, access })}
              />
            </FunctionRow>
          ))}
        </ul>
      )}
    </section>
  );
}

function Remove({ connector, onClose }: { connector: Connector; onClose(): void }) {
  const t = useT();
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: () => api("DELETE", `/api/connectors/${encodeURIComponent(connector.id)}`),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["connectors"] });
      navigate("/", { replace: true });
    },
  });
  return (
    <Sheet title={t("detail.removeTitle", { title: connector.title })} closeLabel={t("common.close")} onClose={onClose}>
      <p className="text-muted">{t("detail.removeBody")}</p>
      <p role="alert" className="mt-3 min-h-5 text-sm text-amber">
        {remove.isError ? errorText(remove.error) : ""}
      </p>
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" autoFocus className={button({ variant: "secondary" })} onClick={onClose}>
          {t("common.cancel")}
        </button>
        <button type="button" disabled={remove.isPending} className={button({ variant: "danger" })} onClick={() => remove.mutate()}>
          <Icon name="trash" className="size-4" />
          {t("detail.removeConfirm")}
        </button>
      </div>
    </Sheet>
  );
}

/** One connector in full: who it is, every function with its access, its credentials, and its removal behind a confirmation. */
export function ConnectorDetail({ id }: { id: string }) {
  const t = useT();
  const queryClient = useQueryClient();
  const connectors = useQuery(connectorsQuery);
  const [removing, setRemoving] = useState(false);
  if (connectors.isPending) {
    return (
      <div aria-hidden="true" className="flex flex-col gap-6">
        <div className="flex items-center gap-4">
          <div className="shimmer size-16 rounded-full" />
          <div className="flex-1">
            <div className={`${SKELETON} h-9 w-64 max-w-full`} />
            <div className="shimmer bubble-word mt-3 h-3 w-48" />
          </div>
        </div>
        <div className={`${SKELETON} h-80`} />
      </div>
    );
  }
  if (connectors.isError) return <Notice>{errorText(connectors.error)}</Notice>;
  const connector = connectors.data.find((c) => c.id === id);
  if (!connector) {
    return (
      <div className="flex flex-col">
        <Back />
        <Notice>{t("detail.gone")}</Notice>
      </div>
    );
  }
  const host = serverHost(connector);
  const facts = [
    { label: t("detail.facts.prefix"), value: `${connector.alias}__*` },
    { label: t("detail.facts.listing"), value: `${connector.listing}@${connector.version}` },
    ...(host ? [{ label: t("detail.facts.server"), value: host }] : []),
  ];
  return (
    <div className="swap-in flex flex-col">
      <Back />
      <header className="mb-8 flex flex-wrap items-start gap-5">
        <ConnectorIcon src={connector.icon} title={connector.title} size="lg" />
        <div className="min-w-48 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="font-serif text-5xl leading-none font-normal tracking-[-0.02em] break-words">{connector.title}</h1>
            {connector.blocked ? <Bubble tone="amber">{connector.blocked}</Bubble> : null}
          </div>
          {connector.publisher ? <p className="mt-1 text-sm text-muted">{t("import.by", { publisher: connector.publisher })}</p> : null}
          {connector.description ? <p className="mt-3 max-w-2xl text-muted">{connector.description}</p> : null}
        </div>
        <button type="button" className={button({ variant: "danger", size: "sm" })} onClick={() => setRemoving(true)}>
          <Icon name="trash" className="size-4" />
          {t("detail.remove")}
        </button>
      </header>
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <Functions connector={connector} />
        <aside className="flex flex-col gap-6">
          <Credentials connector={connector} onSaved={() => void queryClient.invalidateQueries({ queryKey: ["connectors"] })} />
          <dl className={`${SURFACE} grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 p-5 text-sm`}>
            {facts.map((fact) => (
              <div key={fact.label} className="contents">
                <dt className="text-muted">{fact.label}</dt>
                <dd className="min-w-0 truncate text-right font-mono text-xs leading-5" title={fact.value} translate="no">
                  {fact.value}
                </dd>
              </div>
            ))}
          </dl>
        </aside>
      </div>
      {removing ? <Remove connector={connector} onClose={() => setRemoving(false)} /> : null}
    </div>
  );
}
