import { useState } from "react";
import type { ActivitySummary } from "../api";
import { HourlyChart } from "../activity/HourlyChart";
import { useT } from "../i18n/I18nProvider";
import { Segmented } from "../ui/Segmented";
import { CHART_RANGES, hoursOf, type ChartRange } from "./dashboard";

/** The activity page's chart over the dashboard's one read, with a toggle between the last 24 hours by hour and the seven days by day; both ranges come from the same answer, so the toggle needs no wait and no second read. */
export function CallsChart({ summary }: { summary: ActivitySummary }) {
  const t = useT();
  const [range, setRange] = useState<ChartRange>("24h");
  return (
    <HourlyChart
      hours={hoursOf(summary, range)}
      aside={<Segmented<ChartRange> label={t("dashboard.range")} size="sm" value={range} options={CHART_RANGES.map((value) => ({ value, text: t(`dashboard.range.${value}`) }))} onChange={setRange} />}
    />
  );
}
