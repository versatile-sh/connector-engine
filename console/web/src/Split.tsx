import type { ReactNode } from "react";

/** The live front page in two halves: the pitch on lime that never moves, and the panel on ink where things happen. On a phone the pitch sits on top. */
export function Split({ pitch, panel }: { pitch: ReactNode; panel: ReactNode }) {
  return (
    <div className="relative isolate flex min-h-dvh flex-col bg-ink lg:flex-row">
      <section className="no-scrollbar relative isolate flex shrink-0 flex-col overflow-hidden bg-lime px-6 py-6 text-ink sm:px-10 lg:sticky lg:top-0 lg:h-dvh lg:w-1/2 lg:overflow-y-auto lg:px-14 lg:py-10">
        <div aria-hidden="true" className="backdrop-glow absolute inset-0 -z-10" />
        <div aria-hidden="true" className="backdrop-grid-ink absolute inset-0 -z-10" />
        {pitch}
      </section>
      <main id="main" tabIndex={-1} className="relative flex min-w-0 flex-1 flex-col px-6 py-6 sm:px-10 lg:px-14 lg:py-10">
        {panel}
      </main>
    </div>
  );
}

/** One line the pitch says, as a speech bubble on lime: ink from the site, paper when it is the visitor's own. */
export function Said({ mine = false, live = false, children }: { mine?: boolean; live?: boolean; children: ReactNode }) {
  return (
    <li className={`flex ${mine ? "justify-end" : "justify-start"}`}>
      <span
        className={`inline-flex min-w-0 max-w-[24rem] items-center gap-2 [overflow-wrap:anywhere] px-4 py-2.5 text-[0.9375rem] leading-snug ${mine ? "bubble-sent bg-paper font-mono text-sm text-ink" : "bubble bg-ink text-paper"}`}
      >
        {live ? <span aria-hidden="true" className="pulse size-2 shrink-0 rounded-full bg-ink" /> : null}
        {children}
      </span>
    </li>
  );
}
