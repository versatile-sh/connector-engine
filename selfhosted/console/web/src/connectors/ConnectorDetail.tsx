import { useMutation, useQuery, type UseMutationResult } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type Connector } from "../api";
import { hrefOf } from "../activity/activity";
import { useT } from "../i18n/I18nProvider";
import { removeConnector } from "../mutations";
import { connectorsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { Display } from "../ui/Display";
import { Arrow, Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { Sheet } from "../ui/Sheet";
import { CAPTION, PAGE, QUIET_LINK, SKELETON, SKELETON_LINE, SPLIT, SUBTITLE, WELL, button } from "../ui/styles";
import { tabId } from "../ui/Tabs";
import { Tag } from "../ui/Tag";
import { useViewer } from "../people/useViewer";
import { ConnectorChoices } from "./ConnectorChoices";
import { ConnectorIcon } from "./ConnectorIcon";
import { Credentials } from "./Credentials";
import { ProfileTabs } from "./ProfileBar";
import { serverHost } from "./summary";

const PROFILE_PANEL = "profile-panel";

function Back() {
  const t = useT();
  return (
    <a
      href="/"
      onClick={(event) => {
        event.preventDefault();
        navigate("/");
      }}
      className={`${QUIET_LINK} inline-flex items-center gap-1.5 self-start text-sm text-muted`}
    >
      <Arrow back />
      {t("detail.back")}
    </a>
  );
}

function Remove({ connector, remove, onClose }: { connector: Connector; remove: UseMutationResult<unknown, Error, string>; onClose(): void }) {
  const t = useT();
  return (
    <Sheet title={t("detail.removeTitle", { title: connector.title })} closeLabel={t("common.close")} onClose={onClose}>
      <p className="text-muted">{t("detail.removeBody")}</p>
      <p role="alert" className="mt-3 min-h-5 text-sm text-amber-400">
        {remove.isError ? errorText(remove.error) : ""}
      </p>
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        <button type="button" autoFocus className={button({ variant: "secondary" })} onClick={onClose}>
          {t("common.cancel")}
        </button>
        <button type="button" disabled={remove.isPending} className={button({ variant: "danger" })} onClick={() => remove.mutate(connector.id, { onSuccess: () => navigate("/", { replace: true }) })}>
          <Icon name="trash" className="size-4" />
          {t("detail.removeConfirm")}
        </button>
      </div>
    </Sheet>
  );
}

/** One connector in full: who it is, the tabs of its profiles over every function and item with its access for the default or one key, beside them the one credential every key shares and the connector's facts, and, for a viewer who manages connectors, its removal behind a confirmation. */
export function ConnectorDetail({ id }: { id: string }) {
  const t = useT();
  const { query } = useLocation();
  const connectors = useQuery(connectorsQuery);
  const [removing, setRemoving] = useState(false);
  const remove = useMutation(removeConnector);
  const manage = useViewer().can("connectors.manage");
  if (connectors.isPending) {
    return (
      <div aria-hidden="true" className="flex flex-col gap-8">
        <div className={`${SKELETON_LINE} h-4 w-28`} />
        <div className="flex items-center gap-5">
          <div className="shimmer size-16 rounded-2xl" />
          <div className="flex-1">
            <div className={`${SKELETON_LINE} h-10 w-64 max-w-full`} />
            <div className={`${SKELETON_LINE} mt-3 h-3 w-48`} />
          </div>
        </div>
        <div className={`${SKELETON} h-96 rounded-2xl`} />
      </div>
    );
  }
  if (connectors.isError) return <Notice>{errorText(connectors.error)}</Notice>;
  const connector = connectors.data.find((c) => c.id === id);
  if (!connector) {
    return (
      <div className="flex flex-col gap-6">
        <Back />
        <Notice>{t("detail.gone")}</Notice>
      </div>
    );
  }
  const profile = connector.profiles.find((p) => p.agent === query.get("profile"));
  const tabs = connector.profiles.length > 0;
  const host = serverHost(connector);
  const facts = [
    { label: t("detail.facts.prefix"), value: `${connector.alias}__*` },
    { label: t("detail.facts.listing"), value: `${connector.listing}@${connector.version}` },
    ...(host ? [{ label: t("detail.facts.server"), value: host }] : []),
  ];
  return (
    <div className={`swap-in ${PAGE}`}>
      <Back />
      <header className="mb-2 flex flex-wrap items-start gap-x-6 gap-y-5">
        <ConnectorIcon src={connector.icon} title={connector.title} size="lg" />
        <div className="min-w-0 flex-1 basis-full sm:basis-56">
          <div className="flex flex-wrap items-baseline gap-x-4 gap-y-4">
            <Display className="[overflow-wrap:anywhere]">{connector.title}</Display>
            <div className="flex flex-wrap items-center gap-2 sm:ml-auto">
              <a
                href={hrefOf({ range: "24h", connector: connector.id })}
                className={button({ variant: "secondary", size: "sm" })}
                onClick={(event) => {
                  event.preventDefault();
                  navigate(hrefOf({ range: "24h", connector: connector.id }));
                }}
              >
                <Icon name="activity" className="size-4" />
                {t("detail.activity")}
              </a>
              {manage ? (
                <button type="button" className={button({ variant: "danger", size: "sm" })} onClick={() => setRemoving(true)}>
                  <Icon name="trash" className="size-4" />
                  {t("common.remove")}
                </button>
              ) : null}
            </div>
          </div>
          <p className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-muted empty:hidden">
            {connector.publisher ? <span>{t("import.by", { publisher: connector.publisher })}</span> : null}
            {connector.blocked ? <Tag tone="amber" dot>{connector.blocked}</Tag> : null}
          </p>
          {connector.description ? <p className="mt-4 max-w-2xl text-pretty text-muted">{connector.description}</p> : null}
        </div>
      </header>
      <article className="flex flex-col">
        {tabs ? <ProfileTabs connector={connector} agent={profile?.agent} panel={PROFILE_PANEL} className="mb-6" /> : null}
        <div className={SPLIT}>
          <div id={PROFILE_PANEL} role={tabs ? "tabpanel" : undefined} aria-labelledby={tabs ? tabId(PROFILE_PANEL, profile?.agent ?? "") : undefined} className="min-w-0">
            <ConnectorChoices connector={connector} agent={profile?.agent} />
          </div>
          <aside className="flex flex-col gap-6 lg:self-start lg:tall:sticky lg:tall:top-6">
            <Credentials connector={connector} />
            <section aria-labelledby="facts-title" className={`${WELL} p-5`}>
              <h2 id="facts-title" className={SUBTITLE}>
                {t("detail.facts")}
              </h2>
              <dl className="mt-3 grid gap-4 text-sm">
                {facts.map((fact) => (
                  <div key={fact.label} className="min-w-0">
                    <dt className={CAPTION}>{fact.label}</dt>
                    <dd className="mt-1 truncate font-mono text-xs leading-5 text-paper" title={fact.value} translate="no">
                      {fact.value}
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
          </aside>
        </div>
      </article>
      {removing ? <Remove connector={connector} remove={remove} onClose={() => setRemoving(false)} /> : null}
    </div>
  );
}
