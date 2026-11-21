import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { Footer } from "./Footer";
import type { MessageKey } from "./i18n";
import { useT } from "./i18n/I18nProvider";
import { signOut } from "./mutations";
import { approvalsQuery, sessionQuery } from "./queries";
import { navigate, useLocation } from "./router";
import { Sidebar } from "./shell/Sidebar";
import type { Section } from "./shell/shell";
import { TopStrip } from "./shell/TopStrip";
import { useShell } from "./shell/useShell";
import { Notice } from "./ui/Notice";
import { button } from "./ui/styles";

export type { Section } from "./shell/shell";

const OAUTH: Record<string, MessageKey> = {
  connected: "oauth.connected",
  refused: "oauth.refused",
  unknown_state: "oauth.unknown_state",
  issuer_mismatch: "oauth.issuer_mismatch",
  failed: "oauth.failed",
  wrong_user: "oauth.wrong_user",
};

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

/** The frame of every signed-in page: the site's grid and light, a left sidebar on the page's ground (a drawer on a narrow screen), the strip over the page with the endpoint, and the content on an inset raised surface that scrolls on its own and holds the notices, the page and the site's footer. The sidebar and the strip never remount between pages. */
export function Shell({ section, endpoint, children }: { section: Section; endpoint: string; children: ReactNode }) {
  const t = useT();
  const { path, query } = useLocation();
  const waiting = useQuery(approvalsQuery).data?.length ?? 0;
  const user = useQuery(sessionQuery).data?.user;
  const logout = useMutation(signOut);
  const shell = useShell(`${path}?${query}`);
  const sidebarId = useId();
  const sidebar = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLButtonElement>(null);
  const surface = useRef<HTMLElement>(null);
  const oauth = query.get("oauth");
  const oauthKey = oauth ? OAUTH[oauth] : undefined;
  useLayoutEffect(
    function startEachSectionAtTheTop() {
      surface.current?.scrollTo({ top: 0 });
    },
    [section],
  );
  useEffect(
    function keepFocusInTheDrawer() {
      if (!shell.drawer) return;
      const root = sidebar.current;
      const opener = menu.current;
      (root?.querySelector<HTMLElement>('[aria-current="page"]') ?? root)?.focus();
      return () => opener?.focus();
    },
    [shell.drawer],
  );
  return (
    <div data-shell="" className="relative isolate flex h-dvh overflow-hidden">
      <div aria-hidden="true" className="grid-base2 pointer-events-none absolute inset-x-0 top-0 -z-10 h-[40rem]" />
      <div aria-hidden="true" className="light-beam pointer-events-none absolute inset-x-0 top-0 -z-10 h-[32rem]" />
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-ink-raised focus:px-3 focus:py-2">
        {t("app.skip")}
      </a>
      <div
        aria-hidden="true"
        onClick={shell.closeDrawer}
        className={`fixed inset-0 z-30 bg-ink-sunken/80 transition-[opacity,visibility] duration-250 ease-(--ease-glide) lg:hidden ${shell.drawer ? "visible opacity-100" : "invisible opacity-0"}`}
      />
      <Sidebar
        id={sidebarId}
        ref={sidebar}
        section={section}
        waiting={waiting}
        user={user}
        rail={shell.rail}
        modal={shell.drawer}
        signingOut={logout.isPending}
        onToggleRail={shell.toggleRail}
        onNavigate={shell.closeDrawer}
        onClose={shell.closeDrawer}
        onSignOut={() => logout.mutate()}
      />
      <div inert={shell.drawer} className="flex min-w-0 flex-1 flex-col">
        <TopStrip section={section} endpoint={endpoint} sidebar={sidebarId} drawer={shell.drawer} menuRef={menu} onOpenMenu={shell.openDrawer} />
        <main
          id="main"
          ref={surface}
          tabIndex={-1}
          className="shell-surface vignette-top relative flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain border border-line bg-ink-raised [scrollbar-gutter:stable] max-lg:border-x-0 max-lg:border-b-0 lg:mr-3 lg:mb-3 lg:rounded-2xl"
        >
          <div className="mx-auto flex w-full max-w-6xl flex-1 flex-col px-5 pt-8 pb-16 sm:px-8 lg:px-10 lg:pt-10">
            {oauth ? (
              <div className="mb-6">
                <Notice
                  tone={oauth === "connected" ? "lime" : "amber"}
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
          </div>
          <Footer />
        </main>
      </div>
    </div>
  );
}
