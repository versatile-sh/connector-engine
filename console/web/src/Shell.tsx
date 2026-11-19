import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api, setCsrf } from "./api";
import { Wordmark } from "./Brand";
import type { MessageKey } from "./i18n";
import { useT } from "./i18n/I18nProvider";
import { LanguageSwitch } from "./LanguageSwitch";
import { useSetup } from "./onboarding/useSetup";
import { approvalsQuery } from "./queries";
import { navigate, useLocation } from "./router";
import { Icon } from "./ui/Icon";
import { Notice } from "./ui/Notice";
import { ICON_BUTTON, PRESS, button } from "./ui/styles";
import { useCopy } from "./ui/useCopy";

/** The console's three sections and the paths each one owns. */
export type Section = "connectors" | "access" | "approvals";

const NAV: readonly { section: Section; href: string; label: MessageKey }[] = [
  { section: "connectors", href: "/", label: "nav.connectors" },
  { section: "access", href: "/access", label: "nav.access" },
  { section: "approvals", href: "/approvals", label: "nav.approvals" },
];

const OAUTH: Record<string, MessageKey> = {
  connected: "oauth.connected",
  refused: "oauth.refused",
  unknown_state: "oauth.unknown_state",
  issuer_mismatch: "oauth.issuer_mismatch",
  failed: "oauth.failed",
};

const NAV_LINK = `bubble-word inline-flex h-9 shrink-0 items-center gap-2 px-3.5 text-sm font-semibold text-muted hover:bg-fg/8 hover:text-fg aria-[current=page]:bg-ink-lift aria-[current=page]:text-fg ${PRESS}`;

function Endpoint({ url }: { url: string }) {
  const t = useT();
  const { result, copy } = useCopy();
  return (
    <button type="button" onClick={() => void copy(url)} aria-label={t("shell.copyEndpoint", { url })} className={`bubble-word hidden h-9 items-center gap-2 border border-line-strong px-3 font-mono text-xs text-muted hover:border-fg/50 hover:text-fg xl:inline-flex ${PRESS}`}>
      <span className={`size-1.5 rounded-full ${result === "copied" ? "bg-bubble" : "bg-bubble/60"}`} aria-hidden="true" />
      <span translate="no">{url.replace(/^https?:\/\//, "")}</span>
      <Icon name={result === "copied" ? "check" : "copy"} className="size-3.5" />
    </button>
  );
}

function SetupProgress() {
  const t = useT();
  const setup = useSetup();
  if (!setup.ready || setup.hidden || setup.done === setup.total) return null;
  return (
    <button type="button" onClick={() => navigate("/")} className={`${button({ variant: "secondary", size: "sm" })} arrive hidden font-mono text-xs sm:inline-flex`}>
      <span aria-hidden="true" className="flex gap-1">
        {setup.steps.map((step) => (
          <span key={step.id} className={`size-1.5 rounded-full transition-colors duration-500 ${step.done ? "bg-bubble" : "bg-line-strong"}`} />
        ))}
      </span>
      {t("shell.setup", { done: setup.done, total: setup.total })}
    </button>
  );
}

/** The frame of every signed-in page: the lit backdrop, the top bar with the sections, the setup progress, the endpoint, the language and sign-out, and the main landmark. */
export function Shell({ section, endpoint, children }: { section: Section; endpoint: string; children: ReactNode }) {
  const t = useT();
  const { path, query } = useLocation();
  const queryClient = useQueryClient();
  const waiting = useQuery(approvalsQuery).data?.length ?? 0;
  const logout = useMutation({
    mutationFn: () => api("POST", "/api/logout"),
    onSettled: () => {
      setCsrf(undefined);
      queryClient.clear();
    },
  });
  const oauth = query.get("oauth");
  const oauthKey = oauth ? OAUTH[oauth] : undefined;
  return (
    <div className="relative isolate flex min-h-dvh flex-col overflow-x-clip">
      <div aria-hidden="true" className="backdrop-grid pointer-events-none absolute inset-x-0 top-0 -z-10 h-[40rem]" />
      <div aria-hidden="true" className="backdrop-beam pointer-events-none absolute inset-x-0 top-0 -z-10 h-[32rem]" />
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-card focus:px-3 focus:py-2">
        {t("app.skip")}
      </a>
      <header className="sticky top-0 z-30 border-b border-line bg-canvas/85 backdrop-blur-md">
        <div className="mx-auto flex h-16 w-full max-w-6xl items-center gap-4 px-5 sm:px-8">
          <a
            href="/"
            aria-label={t("app.home")}
            className="shrink-0"
            onClick={(event) => {
              event.preventDefault();
              navigate("/");
            }}
          >
            <Wordmark />
          </a>
          <nav aria-label={t("nav.label")} className="ml-4 hidden items-center gap-1 md:flex">
            <NavLinks section={section} waiting={waiting} />
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <SetupProgress />
            <Endpoint url={endpoint} />
            <LanguageSwitch />
            <button type="button" aria-label={t("shell.signOut")} title={t("shell.signOut")} className={ICON_BUTTON} disabled={logout.isPending} onClick={() => logout.mutate()}>
              <Icon name="logout" className="size-[1.15rem]" />
            </button>
          </div>
        </div>
        <nav aria-label={t("nav.label")} className="no-scrollbar mx-auto flex w-full max-w-6xl gap-1 overflow-x-auto px-4 pb-2 md:hidden">
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
        <div key={section} className="swap-in flex flex-1 flex-col">
          {children}
        </div>
      </main>
    </div>
  );
}

function NavLinks({ section, waiting }: { section: Section; waiting: number }) {
  const t = useT();
  return NAV.map((item) => (
    <a
      key={item.section}
      href={item.href}
      aria-current={item.section === section ? "page" : undefined}
      className={NAV_LINK}
      onClick={(event) => {
        event.preventDefault();
        navigate(item.href);
      }}
    >
      {t(item.label)}
      {item.section === "approvals" && waiting > 0 ? (
        <span className="arrive bubble-word bg-bubble px-1.5 py-px font-mono text-[0.6875rem] leading-4 text-on-bubble tabular-nums">
          <span aria-hidden="true">{waiting}</span>
          <span className="sr-only">{t("nav.waiting", { count: waiting })}</span>
        </span>
      ) : null}
    </a>
  ));
}
