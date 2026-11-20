import { useMutation, useQuery } from "@tanstack/react-query";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Wordmark } from "./Brand";
import { Footer } from "./Footer";
import type { MessageKey } from "./i18n";
import { useT } from "./i18n/I18nProvider";
import { LanguageSwitch } from "./LanguageSwitch";
import { signOut } from "./mutations";
import { useSetup } from "./onboarding/useSetup";
import { approvalsQuery, sessionQuery } from "./queries";
import { navigate, useLocation } from "./router";
import { Icon } from "./ui/Icon";
import { flip, irisAt } from "./ui/motion";
import { Notice } from "./ui/Notice";
import { ICON_BUTTON, PRESS, TOUCH, button } from "./ui/styles";
import { useCopy } from "./ui/useCopy";

/** The console's sections and the paths each one owns. */
export type Section = "connectors" | "skills" | "access" | "approvals" | "activity" | "notifications";

const NAV: readonly { section: Section; href: string; label: MessageKey }[] = [
  { section: "connectors", href: "/", label: "nav.connectors" },
  { section: "skills", href: "/skills", label: "nav.skills" },
  { section: "access", href: "/access", label: "nav.access" },
  { section: "approvals", href: "/approvals", label: "nav.approvals" },
  { section: "activity", href: "/activity", label: "nav.activity" },
  { section: "notifications", href: "/notifications", label: "nav.notifications" },
];

const OAUTH: Record<string, MessageKey> = {
  connected: "oauth.connected",
  refused: "oauth.refused",
  unknown_state: "oauth.unknown_state",
  issuer_mismatch: "oauth.issuer_mismatch",
  failed: "oauth.failed",
};

const NAV_LINK = `relative isolate inline-flex h-9 shrink-0 items-center gap-2 rounded-md px-3 text-sm max-lg:px-2 text-muted hover:text-paper aria-[current=page]:text-paper ${TOUCH} ${PRESS}`;

type Iris = { section: Section; at: number; x: number; y: number; r: number };

function Endpoint({ url }: { url: string }) {
  const t = useT();
  const { result, copy } = useCopy();
  return (
    <button type="button" onClick={() => void copy(url)} aria-label={t("shell.copyEndpoint", { url })} title={url} className={`bubble-word hidden h-9 min-w-0 items-center gap-2 border border-line-strong px-3 font-mono text-xs text-muted hover:border-paper/50 hover:text-paper xl:inline-flex ${PRESS}`}>
      <span className={`size-1.5 shrink-0 rounded-full ${result === "copied" ? "bg-lime" : "bg-lime/60"}`} aria-hidden="true" />
      <span className="min-w-0 truncate" translate="no">{url.replace(/^https?:\/\//, "")}</span>
      <Icon name={result === "copied" ? "check" : "copy"} className="size-3.5" />
    </button>
  );
}

const KEY_NOTICE: Partial<Record<string, MessageKey>> = { plaintext: "shell.keysPlaintext", locked: "shell.keysLocked" };

function KeyNotice() {
  const t = useT();
  const key = KEY_NOTICE[useQuery(sessionQuery).data?.key_protection ?? ""];
  if (!key) return null;
  return (
    <div className="mb-6">
      <Notice>{t(key)}</Notice>
    </div>
  );
}

function SetupProgress() {
  const t = useT();
  const setup = useSetup();
  if (!setup.visible) return null;
  return (
    <button type="button" onClick={() => navigate("/")} className={`${button({ variant: "secondary", size: "sm" })} arrive hidden font-mono text-xs sm:inline-flex`}>
      <span aria-hidden="true" className="flex gap-1">
        {setup.steps.map((step) => (
          <span key={step.id} className={`size-1.5 rounded-full transition-colors duration-500 ${step.done ? "bg-lime" : "bg-line-strong"}`} />
        ))}
      </span>
      {t("shell.setup", { done: setup.done, total: setup.total })}
    </button>
  );
}

/** The frame of every signed-in page: the lit backdrop, the top bar with the sections, the setup progress, the endpoint, the language and sign-out, the main landmark and the site's footer. */
export function Shell({ section, endpoint, children }: { section: Section; endpoint: string; children: ReactNode }) {
  const t = useT();
  const { path, query } = useLocation();
  const waiting = useQuery(approvalsQuery).data?.length ?? 0;
  const logout = useMutation(signOut);
  const oauth = query.get("oauth");
  const oauthKey = oauth ? OAUTH[oauth] : undefined;
  return (
    <div className="relative isolate flex min-h-dvh flex-col overflow-x-clip">
      <div aria-hidden="true" className="backdrop-grid pointer-events-none absolute inset-x-0 top-0 -z-10 h-[40rem]" />
      <div aria-hidden="true" className="backdrop-beam pointer-events-none absolute inset-x-0 top-0 -z-10 h-[32rem]" />
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-ink-raised focus:px-3 focus:py-2">
        {t("app.skip")}
      </a>
      <header className="sticky top-0 z-30 border-b border-line bg-ink/85 backdrop-blur-md">
        <div aria-hidden="true" className="light-lamp pointer-events-none absolute inset-x-0 -bottom-px mx-auto h-px w-1/3" />
        <div className="mx-auto flex h-16 w-full max-w-6xl items-center gap-4 px-5 sm:px-8">
          <a
            href="/"
            aria-label={t("app.home")}
            className={`relative shrink-0 ${TOUCH}`}
            onClick={(event) => {
              event.preventDefault();
              navigate("/");
            }}
          >
            <Wordmark />
          </a>
          <nav aria-label={t("nav.label")} className="relative ml-2 hidden items-center gap-1 lg:flex">
            <NavLinks section={section} waiting={waiting} />
          </nav>
          <div className="ml-auto flex min-w-0 items-center gap-2">
            <SetupProgress />
            <Endpoint url={endpoint} />
            <LanguageSwitch />
            <button type="button" aria-label={t("shell.signOut")} title={t("shell.signOut")} className={ICON_BUTTON} disabled={logout.isPending} onClick={() => logout.mutate()}>
              <Icon name="logout" className="size-[1.15rem]" />
            </button>
          </div>
        </div>
        <nav aria-label={t("nav.label")} className="no-scrollbar relative mx-auto flex w-full max-w-6xl gap-1 overflow-x-auto px-4 py-1 lg:hidden">
          <NavLinks section={section} waiting={waiting} />
        </nav>
      </header>
      <main id="main" tabIndex={-1} className="mx-auto flex w-full max-w-6xl flex-1 flex-col px-5 pt-8 pb-20 sm:px-8 sm:pt-10">
        {oauth ? (
          <div className="mb-6">
            <Notice
              tone={oauth === "connected" ? "accent" : "amber"}
              action={
                <button type="button" className={button({ variant: "quiet", size: "xs" })} onClick={() => navigate(path, { replace: true })}>
                  {t("oauth.dismiss")}
                </button>
              }
            >
              {oauthKey ? t(oauthKey) : oauth}
            </Notice>
          </div>
        ) : null}
        <KeyNotice />
        <div key={section} className="swap-in flex flex-1 flex-col">
          {children}
        </div>
      </main>
      <Footer />
    </div>
  );
}

/** The links of the sections inside a `relative` nav: one pill glides under the current link and follows it when the links resize, and a soft iris opens where a link is pressed, from its centre on a key press. */
function NavLinks({ section, waiting }: { section: Section; waiting: number }) {
  const t = useT();
  const pill = useRef<HTMLSpanElement>(null);
  const [iris, setIris] = useState<Iris>();
  useLayoutEffect(
    function followTheCurrentLink() {
      const mark = pill.current;
      const nav = mark?.parentElement;
      if (!mark || !nav) return;
      let placed = "";
      const place = (glide: boolean) => {
        const link = nav.querySelector<HTMLElement>('[aria-current="page"]');
        if (!link) return;
        const width = `${link.offsetWidth}px`;
        const to = `translate(${link.offsetLeft}px, ${link.offsetTop}px)`;
        if (placed === width + to) return;
        placed = width + to;
        const was = mark.getBoundingClientRect();
        const box = nav.getBoundingClientRect();
        const from = glide && was.width > 0 && link.offsetWidth > 0 ? `translate(${was.left - box.left + nav.scrollLeft}px, ${was.top - box.top + nav.scrollTop}px) scaleX(${was.width / link.offsetWidth})` : to;
        mark.style.width = width;
        flip([{ element: mark, from, to }]);
      };
      place(true);
      const observer = new ResizeObserver(() => place(false));
      for (const link of nav.querySelectorAll("a")) observer.observe(link);
      return () => observer.disconnect();
    },
    [section],
  );
  return (
    <>
      <span ref={pill} aria-hidden="true" className="pointer-events-none absolute top-0 left-0 h-9 origin-left rounded-md bg-paper/8 transition-transform duration-250 ease-(--ease-glide)" />
      {NAV.map((item) => (
        <a
          key={item.section}
          href={item.href}
          aria-current={item.section === section ? "page" : undefined}
          className={NAV_LINK}
          onClick={(event) => {
            event.preventDefault();
            const link = event.currentTarget;
            const box = link.getBoundingClientRect();
            const point = event.detail === 0 ? undefined : { x: event.clientX - box.left, y: event.clientY - box.top };
            setIris({ section: item.section, at: event.timeStamp, ...irisAt(link.offsetWidth, link.offsetHeight, point) });
            navigate(item.href);
          }}
        >
          {iris?.section === item.section ? (
            <span aria-hidden="true" className="pointer-events-none absolute inset-0 -z-10 overflow-hidden rounded-[inherit]">
              <span
                key={iris.at}
                className="iris absolute -translate-1/2 rounded-full bg-[radial-gradient(closest-side,rgb(245_245_242/0.16),transparent)] opacity-0"
                style={{ left: iris.x, top: iris.y, width: iris.r * 2, height: iris.r * 2 }}
              />
            </span>
          ) : null}
          {t(item.label)}
          {item.section === "approvals" && waiting > 0 ? (
            <span className="arrive bubble-word bg-lime px-1.5 py-px font-mono text-[0.6875rem] leading-4 text-ink tabular-nums">
              <span aria-hidden="true">{waiting}</span>
              <span className="sr-only">{t("nav.waiting", { count: waiting })}</span>
            </span>
          ) : null}
        </a>
      ))}
    </>
  );
}
