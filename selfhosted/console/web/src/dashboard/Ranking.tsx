import type { ReactNode } from "react";
import { openLink } from "../router";
import { CAPTION, SURFACE, TITLE } from "../ui/styles";
import { shareOf } from "./dashboard";

/** One row of a ranking: what it names, the icon before it, its calls and how many of them failed, and where it opens, when it opens anywhere. */
export type RankRow = { key: string; label: ReactNode; icon?: ReactNode; total: number; failed: string | undefined; calls: string; href?: string | undefined };

const ROW = "grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 rounded-lg px-2 py-2.5 transition-colors duration-200";

function Row({ row, busiest }: { row: RankRow; busiest: number }) {
  const body = (
    <>
      {row.icon ?? <span />}
      <span className="min-w-0">
        <span className="block truncate text-sm text-paper">{row.label}</span>
        {row.failed ? <span className="block font-mono text-[0.6875rem] text-amber-400">{row.failed}</span> : null}
        <span aria-hidden="true" className="mt-1.5 block h-0.5 overflow-hidden rounded-full bg-line">
          <span className="block h-full rounded-full bg-paper/30" style={{ width: `${shareOf(row.total, busiest)}%` }} />
        </span>
      </span>
      <span aria-hidden="true" className="font-mono text-xs text-muted-light tabular-nums">
        {row.total}
      </span>
      <span className="sr-only">{row.calls}</span>
    </>
  );
  const href = row.href;
  return href ? (
    <a
      href={href}
      onClick={openLink(href)}
      className={`${ROW} hover:bg-paper/5 focus-visible:bg-paper/5`}
    >
      {body}
    </a>
  ) : (
    <div className={ROW}>{body}</div>
  );
}

type RankingProps = { id: string; title: string; caption: string; rows: readonly RankRow[] };

/** The few busiest of a kind, as quiet rows on dividers with a hairline under each name that shows its share of the busiest, never numbered: the name, the icon, its calls and, in amber, how many failed; a row opens the activity narrowed to it. */
export function Ranking({ id, title, caption, rows }: RankingProps) {
  const busiest = Math.max(0, ...rows.map((row) => row.total));
  return (
    <section aria-labelledby={id} className={`${SURFACE} flex min-w-0 flex-col p-4 sm:p-5`}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 px-1">
        <h2 id={id} className={TITLE}>
          {title}
        </h2>
        <span className={CAPTION}>{caption}</span>
      </div>
      <ul className="-mx-1 mt-2 flex flex-col divide-y divide-line">
        {rows.map((row) => (
          <li key={row.key}>
            <Row row={row} busiest={busiest} />
          </li>
        ))}
      </ul>
    </section>
  );
}
