import type { ActivitySummary } from "../api";
import { hrefOf } from "../activity/activity";
import type { MessageKey } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { deltaOf, failedShare, type Delta } from "./dashboard";
import { PairCard, StatCard } from "./StatCard";

const DELTA: Record<Delta["kind"], MessageKey> = { up: "dashboard.delta.up", down: "dashboard.delta.down", same: "dashboard.delta.same", new: "dashboard.delta.new", none: "dashboard.delta.none" };

type KpisProps = { summary: ActivitySummary; waiting: number | undefined; connectors: number | undefined; keys: number | undefined };

/** The four numbers at the head of the dashboard, as equal cards: the last day's calls against the day before, how many failed or were denied, the calls held for approval, and how many connectors and access keys are in use; the counts that come from other reads show their shape until they arrive. */
export function Kpis({ summary, waiting, connectors, keys }: KpisProps) {
  const t = useT();
  const number = new Intl.NumberFormat(useLocale());
  const { day } = summary;
  const delta = deltaOf(day.total, summary.previous_day.total);
  const share = failedShare(day);
  const count = (value: number | undefined) => (value === undefined ? undefined : number.format(value));
  return (
    <section aria-label={t("dashboard.stats")} className="grid gap-6 @xl:grid-cols-2 @4xl:grid-cols-4">
      <StatCard label={t("dashboard.calls")} href={hrefOf({ range: "24h" })} value={number.format(day.total)} note={t(DELTA[delta.kind], "percent" in delta ? { percent: delta.percent } : {})} />
      <StatCard
        label={t("dashboard.failed")}
        href={hrefOf({ range: "24h" })}
        value={number.format(day.failed)}
        note={share === undefined || day.failed === 0 ? t("dashboard.failed.none") : t("dashboard.failed.share", { percent: share })}
        mark={day.failed > 0 ? "care" : undefined}
      />
      <StatCard
        label={t("dashboard.waiting")}
        href="/approvals"
        value={count(waiting)}
        note={t(waiting ? "dashboard.waiting.some" : "dashboard.waiting.none")}
        mark={waiting ? "live" : undefined}
      />
      <PairCard
        label={t("dashboard.connected")}
        left={{ href: "/connectors", value: count(connectors), noun: t("dashboard.connectors", { count: connectors ?? 0 }) }}
        right={{ href: "/access", value: count(keys), noun: t("dashboard.keys", { count: keys ?? 0 }) }}
      />
    </section>
  );
}
