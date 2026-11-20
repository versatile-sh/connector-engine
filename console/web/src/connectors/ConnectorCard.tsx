import type { Connector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Bubble } from "../ui/Bubble";
import { Arrow } from "../ui/Icon";
import { LIT_EDGE } from "../ui/styles";
import { ConnectorIcon } from "./ConnectorIcon";
import { accessCounts, authState, serverHost } from "./summary";

const BAR = { on: "bg-paper", ask: "bg-lime" } as const;

/** One installed connector as a card that opens it: who it is, how its functions are set, and what it still needs. */
export function ConnectorCard({ connector }: { connector: Connector }) {
  const t = useT();
  const functions = connector.functions ?? [];
  const counts = accessCounts(connector);
  const auth = authState(connector);
  const changed = functions.filter((fn) => fn.state === "quarantined").length;
  const href = `/connectors/${encodeURIComponent(connector.id)}`;
  return (
    <li>
      <a
        href={href}
        onClick={(event) => {
          event.preventDefault();
          navigate(href);
        }}
        className={`group bubble ${LIT_EDGE} flex h-full flex-col border border-line-strong bg-ink-raised p-5 transition-[border-color,translate] duration-300 ease-(--ease-settle) hover:-translate-y-0.5 focus-visible:-translate-y-0.5`}
      >
        <div className="flex items-start gap-3.5">
          <ConnectorIcon src={connector.icon} title={connector.title} />
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-base leading-snug font-medium">{connector.title}</h2>
            <p className="truncate font-mono text-xs text-muted" translate="no">
              {connector.publisher ?? serverHost(connector) ?? connector.listing}
            </p>
          </div>
          <span className="mt-1 text-muted transition-[color,translate] duration-300 group-hover:translate-x-0.5 group-hover:text-lime">
            <Arrow />
          </span>
        </div>
        <p className="mt-3 line-clamp-2 min-h-10 text-sm text-muted [overflow-wrap:anywhere]">{connector.description}</p>
        <div aria-hidden="true" className="mt-4 flex h-1.5 overflow-hidden rounded-full bg-line">
          {(["on", "ask"] as const).map((access) =>
            counts[access] > 0 ? <span key={access} className={BAR[access]} style={{ width: `${(counts[access] / functions.length) * 100}%` }} /> : null,
          )}
        </div>
        <div className="mt-3 flex flex-wrap gap-1.5">
          <Bubble>{t("connectors.functions", { count: functions.length })}</Bubble>
          {counts.on > 0 ? <Bubble>{t("access.count.on", { count: counts.on })}</Bubble> : null}
          {counts.ask > 0 ? <Bubble tone="outline">{t("access.count.ask", { count: counts.ask })}</Bubble> : null}
          {changed > 0 ? <Bubble tone="amber">{t("connectors.changed", { count: changed })}</Bubble> : null}
          {auth === "key" ? <Bubble tone="amber">{t("connectors.needsKey")}</Bubble> : null}
          {auth === "signIn" ? <Bubble tone="amber">{t("connectors.needsSignIn")}</Bubble> : null}
          {auth === "set" ? <Bubble tone="outline">{t("connectors.credentialSet")}</Bubble> : null}
          {connector.blocked ? <Bubble tone="amber">{connector.blocked}</Bubble> : null}
        </div>
      </a>
    </li>
  );
}

/** The shape of a connector card while the list loads. */
export function ConnectorCardSkeleton() {
  return (
    <li aria-hidden="true" className="bubble border border-line bg-ink-raised p-5">
      <div className="flex items-center gap-3.5">
        <div className="shimmer size-11 rounded-full" />
        <div className="flex-1">
          <div className="shimmer bubble-word h-5 w-32" />
          <div className="shimmer bubble-word mt-2 h-3 w-20" />
        </div>
      </div>
      <div className="shimmer bubble-word mt-4 h-3 w-full" />
      <div className="shimmer bubble-word mt-2 h-3 w-2/3" />
      <div className="mt-5 flex gap-1.5">
        <div className="shimmer bubble-word h-5 w-20" />
        <div className="shimmer bubble-word h-5 w-14" />
      </div>
    </li>
  );
}
