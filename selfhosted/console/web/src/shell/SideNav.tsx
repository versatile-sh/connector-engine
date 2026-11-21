import { useLayoutEffect, useRef, useState, type MouseEvent } from "react";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Icon } from "../ui/Icon";
import { irisAt } from "../ui/motion";
import { EYEBROW, PRESS, TOUCH } from "../ui/styles";
import { navView, type NavEntry, type Section } from "./shell";

type SideNavProps = { section: Section; waiting: number; rail: boolean; onNavigate(): void };

type Iris = { section: Section; at: number; x: number; y: number; r: number };

const LINK = `group/item relative isolate flex h-10 items-center gap-3 rounded-xl border border-transparent px-3.5 text-sm text-muted hover:text-paper aria-[current=page]:text-paper ${TOUCH} ${PRESS}`;

const TIP =
  "pointer-events-none absolute top-1/2 left-full z-50 ml-3 -translate-y-1/2 rounded-lg border border-line-strong bg-ink-raised px-2.5 py-1.5 text-xs whitespace-nowrap text-paper opacity-0 shadow-lg shadow-black/40 transition-opacity duration-150 group-hover/item:opacity-100 group-focus-visible/item:opacity-100";

const BADGE = "arrive ml-auto rounded-full bg-paper/10 px-1.5 font-mono text-xs text-paper tabular-nums group-data-rail/side:absolute group-data-rail/side:top-0 group-data-rail/side:right-0 group-data-rail/side:ml-0";

const modified = (event: MouseEvent) => event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;

/** The sidebar's links in their groups, with one raised pill and a lime edge mark that glide to the current link, a soft iris where a link is pressed, and, in the rail, a tooltip with each link's name on hover and keyboard focus. */
export function SideNav({ section, waiting, rail, onNavigate }: SideNavProps) {
  const t = useT();
  const nav = useRef<HTMLElement>(null);
  const pill = useRef<HTMLSpanElement>(null);
  const [iris, setIris] = useState<Iris>();
  useLayoutEffect(
    function followTheCurrentLink() {
      const mark = pill.current;
      const list = nav.current;
      if (!mark || !list) return;
      const place = () => {
        const link = list.querySelector<HTMLElement>('[aria-current="page"]');
        if (!link) return;
        const to = `translateY(${link.offsetTop}px)`;
        if (mark.style.transform === to) return;
        const first = !mark.hasAttribute("data-placed");
        if (first) mark.style.transition = "none";
        mark.style.transform = to;
        if (!first) return;
        void mark.offsetWidth;
        mark.style.removeProperty("transition");
        mark.setAttribute("data-placed", "");
      };
      place();
      const observer = new ResizeObserver(place);
      observer.observe(list);
      return () => observer.disconnect();
    },
    [section],
  );
  const open = (item: NavEntry) => (event: MouseEvent<HTMLAnchorElement>) => {
    if (modified(event)) return;
    event.preventDefault();
    const link = event.currentTarget;
    const box = link.getBoundingClientRect();
    const point = event.detail === 0 ? undefined : { x: event.clientX - box.left, y: event.clientY - box.top };
    setIris({ section: item.section, at: event.timeStamp, ...irisAt(link.offsetWidth, link.offsetHeight, point) });
    navigate(item.href);
    onNavigate();
  };
  return (
    <nav ref={nav} aria-label={t("nav.label")} className="no-scrollbar relative min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 pb-2 group-data-rail/side:overflow-visible">
      <span
        ref={pill}
        aria-hidden="true"
        className="vignette-top pointer-events-none absolute inset-x-3 top-0 h-10 rounded-xl border border-line-strong bg-ink-raised opacity-0 transition-[transform,opacity] duration-250 ease-(--ease-glide) data-placed:opacity-100"
      >
        <span className="absolute top-1/2 left-1.5 h-4 w-0.5 -translate-y-1/2 rounded-full bg-lime" />
      </span>
      {navView(section, waiting).map((group) => (
        <div key={group.id} role="group" aria-labelledby={`nav-${group.id}`} className="pt-2 first:pt-0">
          <div className="relative flex h-7 items-center px-3.5">
            <span id={`nav-${group.id}`} className={`${EYEBROW} transition-opacity duration-250 group-data-rail/side:opacity-0`}>
              {t(group.label)}
            </span>
            <span aria-hidden="true" className="absolute inset-x-3.5 top-1/2 h-px bg-line opacity-0 transition-opacity duration-250 group-data-rail/side:opacity-100" />
          </div>
          <ul role="list" className="flex flex-col gap-0.5">
            {group.items.map((item) => (
              <li key={item.section}>
                <a href={item.href} aria-current={item.current ? "page" : undefined} className={LINK} onClick={open(item)}>
                  {iris?.section === item.section ? (
                    <span aria-hidden="true" className="pointer-events-none absolute inset-0 -z-10 overflow-hidden rounded-[inherit]">
                      <span
                        key={iris.at}
                        className="iris absolute -translate-1/2 rounded-full bg-[radial-gradient(closest-side,color-mix(in_srgb,var(--color-paper)_16%,transparent),transparent)] opacity-0"
                        style={{ left: iris.x, top: iris.y, width: iris.r * 2, height: iris.r * 2 }}
                      />
                    </span>
                  ) : null}
                  <Icon name={item.icon} className="size-5" />
                  <span className="min-w-0 flex-1 truncate transition-opacity duration-250 group-data-rail/side:opacity-0">{t(item.label)}</span>
                  {item.waiting > 0 ? (
                    <span className={BADGE}>
                      <span aria-hidden="true">{item.waiting}</span>
                      <span className="sr-only">{t("nav.waiting", { count: item.waiting })}</span>
                    </span>
                  ) : null}
                  {rail ? (
                    <span aria-hidden="true" className={TIP}>
                      {t(item.label)}
                    </span>
                  ) : null}
                </a>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}
