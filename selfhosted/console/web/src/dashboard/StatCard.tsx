import type { ReactNode } from "react";
import { openLink } from "../router";
import { Arrow } from "../ui/Icon";
import { SKELETON_LINE, SURFACE } from "../ui/styles";

const CARD = `${SURFACE} flex min-h-36 min-w-0 flex-col p-5`;

const NUMBER = "text-4xl leading-none font-medium tracking-tight text-paper tabular-nums";

/** What a stat card's note is marked with: a live lime dot, or an amber one for something that needs care. */
export type Mark = "live" | "care";

/** The small dot before a note; lime for something live, amber for something that needs care. */
export function MarkDot({ mark }: { mark: Mark }) {
  return <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${mark === "live" ? "bg-lime" : "bg-amber-400"}`} />;
}

/** The number of a stat card while it loads, in the card's own shape. */
export function StatSkeleton() {
  return (
    <>
      <div className={`${SKELETON_LINE} h-9 w-20`} />
      <div className={`${SKELETON_LINE} mt-3 h-3 w-36`} />
    </>
  );
}

type StatCardProps = { label: string; href: string; value: ReactNode | undefined; note: ReactNode; mark?: Mark | undefined };

/** One headline number of the dashboard as a card that opens what it counts: its label and an arrow on top, the number and a one-line note pinned to the foot, so a row of cards lines up; a number still loading shows its shape. */
export function StatCard({ label, href, value, note, mark }: StatCardProps) {
  return (
    <a
      href={href}
      onClick={openLink(href)}
      className={`${CARD} group transition-colors duration-250 ease-(--ease-settle) hover:border-paper/30 focus-visible:border-paper/30`}
    >
      <span className="flex items-start justify-between gap-3">
        <span className="text-sm text-muted">{label}</span>
        <span className="text-muted transition-[color,translate] duration-250 group-hover:translate-x-0.5 group-hover:text-paper">
          <Arrow />
        </span>
      </span>
      <span className="mt-auto block pt-5">
        {value === undefined ? (
          <StatSkeleton />
        ) : (
          <>
            <span className={`block ${NUMBER}`}>{value}</span>
            <span className="mt-2.5 flex items-center gap-2 text-sm text-muted">
              {mark ? <MarkDot mark={mark} /> : null}
              {note}
            </span>
          </>
        )}
      </span>
    </a>
  );
}

type CountLink = { href: string; value: ReactNode | undefined; noun: string };

/** A card of two numbers that each open their own page: the label on top, then the two counts side by side, each over its noun, pinned to the foot. */
export function PairCard({ label, left, right }: { label: string; left: CountLink; right: CountLink }) {
  return (
    <div className={CARD}>
      <span className="text-sm text-muted">{label}</span>
      <div className="mt-auto grid grid-cols-2 gap-4 pt-5">
        {[left, right].map((side) => (
          <a
            key={side.href}
            href={side.href}
            onClick={openLink(side.href)}
            className="group -m-1 block min-w-0 rounded-lg p-1 transition-colors duration-250 hover:bg-paper/5 focus-visible:bg-paper/5"
          >
            {side.value === undefined ? (
              <StatSkeleton />
            ) : (
              <>
                <span className={`block ${NUMBER}`}>{side.value}</span>
                <span className="mt-2.5 block truncate text-sm text-muted group-hover:text-paper">{side.noun}</span>
              </>
            )}
          </a>
        ))}
      </div>
    </div>
  );
}
