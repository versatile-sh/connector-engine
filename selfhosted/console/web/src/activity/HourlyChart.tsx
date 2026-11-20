import type { HourCount } from "../api";
import type { Locale } from "../i18n";
import { useLocale, useT } from "../i18n/I18nProvider";
import { CAPTION, SURFACE, TITLE } from "../ui/styles";
import { barsOf, type Bar } from "./activity";

const HEIGHT = 120;
const STEP = 10;
const GAP = 2.5;

const SERIES = [
  { key: "ok", fill: "fill-paper/45", swatch: "bg-paper/45", label: "outcome.ok" },
  { key: "failed", fill: "fill-amber-400", swatch: "bg-amber-400", label: "outcome.error" },
  { key: "denied", fill: "fill-red-400/80", swatch: "bg-red-400/80", label: "outcome.denied" },
  { key: "waiting", fill: "fill-paper", swatch: "bg-paper", label: "outcome.pending" },
] as const;

const whenOf = (bar: Bar, locale: Locale) =>
  new Intl.DateTimeFormat(locale, bar.unit === "hour" ? { weekday: "short", hour: "2-digit", minute: "2-digit" } : { weekday: "short", day: "numeric", month: "short" }).format(new Date(bar.start));

function Stack({ bar, x, max, label }: { bar: Bar; x: number; max: number; label: string }) {
  let top = HEIGHT;
  return (
    <g>
      <title>{label}</title>
      <rect x={x} y={0} width={STEP} height={HEIGHT} className="fill-transparent" />
      {SERIES.map(({ key, fill }) => {
        const height = (bar[key] / max) * (HEIGHT - 4);
        top -= height;
        return <rect key={key} x={x + GAP / 2} y={top} width={STEP - GAP} height={height} className={`chart-bar ${fill}`} />;
      })}
    </g>
  );
}

/** The calls of the period as stacked bars, per hour over up to two days and per local day beyond, quiet paper for answered calls, full paper for waiting ones and colour only for failed and denied ones, with a legend; screen readers get the same numbers as a table. */
export function HourlyChart({ hours }: { hours: readonly HourCount[] }) {
  const t = useT();
  const locale = useLocale();
  const bars = barsOf(hours);
  const max = Math.max(1, ...bars.map((bar) => bar.total));
  const total = bars.reduce((sum, bar) => sum + bar.total, 0);
  const daily = bars[0]?.unit === "day";
  const title = t(daily ? "activity.chart.daily" : "activity.chart.hourly");
  const width = Math.max(bars.length, 1) * STEP;
  const ticks = [...new Set([0, Math.floor((bars.length - 1) / 2), bars.length - 1])].flatMap((index) => bars[index] ?? []);
  const labelOf = (bar: Bar) => t("activity.chart.bar", { when: whenOf(bar, locale), total: bar.total, ok: bar.ok, failed: bar.failed, denied: bar.denied, waiting: bar.waiting });
  return (
    <figure aria-labelledby="chart-title" className={`${SURFACE} min-w-0 p-5 sm:p-6`}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <figcaption id="chart-title" className={TITLE}>
          {title}
        </figcaption>
        <span className={CAPTION}>{t("activity.chart.summary", { total, max: total === 0 ? 0 : max, unit: t(daily ? "activity.chart.day" : "activity.chart.hour") })}</span>
      </div>
      <svg aria-hidden="true" viewBox={`0 0 ${width} ${HEIGHT}`} preserveAspectRatio="none" className="mt-5 block h-36 w-full overflow-visible">
        <line x1={0} x2={width} y1={HEIGHT - 0.5} y2={HEIGHT - 0.5} vectorEffect="non-scaling-stroke" className="stroke-line-strong" />
        {bars.map((bar, index) => (
          <Stack key={bar.start} bar={bar} x={index * STEP} max={max} label={labelOf(bar)} />
        ))}
      </svg>
      <div aria-hidden="true" className="mt-2 flex justify-between gap-2 font-mono text-[0.6875rem] text-muted">
        {ticks.map((bar) => (
          <span key={bar.start} className="truncate">
            {whenOf(bar, locale)}
          </span>
        ))}
      </div>
      <ul aria-hidden="true" className="mt-4 flex flex-wrap gap-x-4 gap-y-1 font-mono text-[0.6875rem] text-muted">
        {SERIES.map(({ key, swatch, label }) => (
          <li key={key} className="flex items-center gap-1.5">
            <span className={`size-2 rounded-sm ${swatch}`} />
            {t(label)}
          </li>
        ))}
      </ul>
      <table className="sr-only">
        <caption>{title}</caption>
        <thead>
          <tr>
            <th scope="col">{t("activity.when")}</th>
            <th scope="col">{t("activity.calls")}</th>
          </tr>
        </thead>
        <tbody>
          {bars
            .filter((bar) => bar.total > 0)
            .map((bar) => (
              <tr key={bar.start}>
                <th scope="row">{whenOf(bar, locale)}</th>
                <td>{labelOf(bar)}</td>
              </tr>
            ))}
        </tbody>
      </table>
    </figure>
  );
}
