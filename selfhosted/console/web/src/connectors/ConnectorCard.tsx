import type { Connector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Arrow } from "../ui/Icon";
import { LIT_CARD, SKELETON_LINE, SURFACE } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { AccessBar } from "./AccessBar";
import { ConnectorIcon } from "./ConnectorIcon";
import { accessCounts, authState, serverHost } from "./summary";
import { rowMotion, type RowPhase } from "../ui/kept";

/** One installed connector as a card that opens it: who it is, how its functions are set, and what it still needs. */
export function ConnectorCard({ connector, phase }: { connector: Connector; phase?: RowPhase }) {
  const t = useT();
  const functions = connector.functions ?? [];
  const counts = accessCounts(functions);
  const auth = authState(connector);
  const changed = functions.filter((fn) => fn.state === "quarantined").length;
  const href = `/connectors/${encodeURIComponent(connector.id)}`;
  return (
    <li {...rowMotion(phase)}>
      <a
        href={href}
        onClick={(event) => {
          event.preventDefault();
          navigate(href);
        }}
        className={`group ${LIT_CARD} flex h-full flex-col p-5`}
      >
        <div className="flex items-start gap-3.5">
          <ConnectorIcon src={connector.icon} title={connector.title} />
          <div className="min-w-0 flex-1">
            <h2 className="truncate leading-snug font-medium">{connector.title}</h2>
            <p className="truncate font-mono text-xs text-muted" translate="no">
              {connector.publisher ?? serverHost(connector) ?? connector.listing}
            </p>
          </div>
          <span className="mt-1 text-muted transition-[color,translate] duration-250 group-hover:translate-x-0.5 group-hover:text-lime">
            <Arrow />
          </span>
        </div>
        <p className="mt-3 line-clamp-2 min-h-10 text-sm text-muted">{connector.description}</p>
        <div className="mt-auto pt-4">
          <AccessBar counts={counts} />
          <div className="mt-2.5 flex flex-wrap gap-x-3 gap-y-0.5">
            <Tag>{t("connectors.functions", { count: functions.length })}</Tag>
            {counts.on > 0 ? <Tag tone="paper">{t("access.count.on", { count: counts.on })}</Tag> : null}
            {counts.ask > 0 ? <Tag tone="lime">{t("access.count.ask", { count: counts.ask })}</Tag> : null}
            {changed > 0 ? <Tag tone="amber" dot>{t("connectors.changed", { count: changed })}</Tag> : null}
            {auth === "key" ? <Tag tone="amber" dot>{t("connectors.needsKey")}</Tag> : null}
            {auth === "signIn" ? <Tag tone="amber" dot>{t("connectors.needsSignIn")}</Tag> : null}
            {auth === "set" ? <Tag>{t("connectors.credentialSet")}</Tag> : null}
            {connector.blocked ? <Tag tone="amber" dot>{connector.blocked}</Tag> : null}
          </div>
        </div>
      </a>
    </li>
  );
}

/** The shape of a connector card while the list loads. */
export function ConnectorCardSkeleton() {
  return (
    <li aria-hidden="true" className={`${SURFACE} p-5`}>
      <div className="flex items-center gap-3.5">
        <div className="shimmer size-11 rounded-xl" />
        <div className="flex-1">
          <div className={`${SKELETON_LINE} h-5 w-32`} />
          <div className={`${SKELETON_LINE} mt-2 h-3 w-20`} />
        </div>
      </div>
      <div className={`${SKELETON_LINE} mt-4 h-3 w-full`} />
      <div className={`${SKELETON_LINE} mt-2 h-3 w-2/3`} />
      <div className={`${SKELETON_LINE} mt-5 h-1.5 w-full`} />
    </li>
  );
}
