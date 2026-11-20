import { useEffect, useLayoutEffect, useRef, type KeyboardEvent } from "react";
import { Select } from "./Select";
import { TOUCH_ROW } from "./styles";
import { TABS_OVERFLOW_FROM, edgesOf, revealOf, stepOf } from "./tabs";

/** One tab: the value it shows, its text, what it means on hover, and `mono` when the text is a machine name. */
export type TabOption<T extends string> = { value: T; text: string; hint?: string; mono?: boolean };

type TabsProps<T extends string> = { label: string; value: T; options: readonly TabOption<T>[]; onChange(value: T): void; panel: string; className?: string };

/** The id of the tab showing `value` in the tabs that control `panel`, for the panel's `aria-labelledby`. */
export const tabId = (panel: string, value: string) => `${panel}-tab-${encodeURIComponent(value)}`;

const ROOM = 24;

const CHOSEN = '[role="tab"][aria-selected="true"]';

const still = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

function placeLine(list: HTMLElement, line: HTMLElement, snap: boolean) {
  const tab = list.querySelector<HTMLElement>(CHOSEN);
  if (snap) line.style.transition = "none";
  line.style.opacity = tab ? "1" : "0";
  if (tab) line.style.transform = `translateX(${tab.offsetLeft}px) scaleX(${tab.offsetWidth})`;
  if (!snap) return;
  void line.offsetWidth;
  line.style.removeProperty("transition");
}

function markEdges(strip: HTMLElement) {
  const { start, end } = edgesOf(strip.scrollLeft, strip.scrollWidth, strip.clientWidth);
  strip.toggleAttribute("data-fade-start", start);
  strip.toggleAttribute("data-fade-end", end);
}

/** A row of tabs that switch the view of one panel, over a hairline: text tabs, the chosen one in paper with a paper underline that glides to it on transform alone. The arrows, Home and End move and choose, focus stays on one tab, and past six tabs the row scrolls sideways with faded edges and a `Select` of every tab beside it. Pair it with `<div id={panel} role="tabpanel" aria-labelledby={tabId(panel, value)}>`. */
export function Tabs<T extends string>({ label, value, options, onChange, panel, className = "" }: TabsProps<T>) {
  const strip = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const line = useRef<HTMLSpanElement>(null);
  const placed = useRef(false);
  const at = options.findIndex((option) => option.value === value);

  useLayoutEffect(function followChosenTab() {
    const row = strip.current;
    const tabs = list.current;
    const bar = line.current;
    if (!row || !tabs || !bar) return;
    placeLine(tabs, bar, !placed.current);
    const tab = tabs.querySelector<HTMLElement>(CHOSEN);
    const left = tab ? revealOf({ left: tabs.offsetLeft + tab.offsetLeft, width: tab.offsetWidth }, { left: row.scrollLeft, width: row.clientWidth }, ROOM) : undefined;
    if (left !== undefined) row.scrollTo({ left, behavior: placed.current && !still() ? "smooth" : "instant" });
    placed.current = true;
  });

  useEffect(function watchRowSize() {
    const row = strip.current;
    const tabs = list.current;
    const bar = line.current;
    if (!row || !tabs || !bar) return;
    const observer = new ResizeObserver(() => {
      placeLine(tabs, bar, true);
      markEdges(row);
    });
    observer.observe(row);
    observer.observe(tabs);
    return () => observer.disconnect();
  }, []);

  const onKeyDown = (event: KeyboardEvent) => {
    const next = stepOf(event.key, at, options.length);
    if (next === undefined) return;
    const option = options[next];
    if (!option) return;
    event.preventDefault();
    if (option.value !== value) onChange(option.value);
    list.current?.querySelectorAll<HTMLElement>('[role="tab"]')[next]?.focus();
  };

  return (
    <div className={`flex min-w-0 items-center gap-3 border-b border-line ${className}`}>
      <div ref={strip} onScroll={(event) => markEdges(event.currentTarget)} className="tabs-strip no-scrollbar relative -mx-1.5 -mb-px min-w-0 flex-1 overflow-x-auto px-1.5 pt-1.5">
        <div ref={list} role="tablist" aria-label={label} onKeyDown={onKeyDown} className="relative flex w-max gap-6 pb-1.5">
          {options.map((option, index) => {
            const chosen = option.value === value;
            return (
              <button
                key={option.value}
                id={tabId(panel, option.value)}
                type="button"
                role="tab"
                aria-selected={chosen}
                aria-controls={panel}
                tabIndex={chosen || (at < 0 && index === 0) ? 0 : -1}
                title={option.hint}
                translate={option.mono ? "no" : undefined}
                onClick={() => !chosen && onChange(option.value)}
                className={`relative h-9 shrink-0 px-0.5 whitespace-nowrap text-muted transition-colors duration-250 hover:text-paper aria-selected:text-paper ${option.mono ? "font-mono text-[0.8125rem]" : "text-sm font-medium"} ${TOUCH_ROW}`}
              >
                {option.text}
              </button>
            );
          })}
          <span ref={line} aria-hidden="true" className="tab-line pointer-events-none absolute bottom-0 left-0 h-0.5 w-px origin-left bg-paper" />
        </div>
      </div>
      {options.length > TABS_OVERFLOW_FROM ? <Select<T> label={label} value={value} options={options} onChange={onChange} className="mb-px w-44 max-w-[40%] shrink-0" /> : null}
    </div>
  );
}
