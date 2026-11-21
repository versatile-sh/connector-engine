import type { Ref } from "react";
import type { Role, SessionUser } from "../api";
import { Logotype, Mark } from "../Brand";
import { useT } from "../i18n/I18nProvider";
import { LanguageSwitch } from "../LanguageSwitch";
import { ThemeSwitch } from "../ThemeSwitch";
import { navigate } from "../router";
import { Icon } from "../ui/Icon";
import { ICON_BUTTON, PRESS, TOUCH } from "../ui/styles";
import { SideNav } from "./SideNav";
import { closesDrawer, wrapFocus, type Section } from "./shell";
import { UserMenu } from "./UserMenu";

type SidebarProps = {
  id: string;
  ref?: Ref<HTMLDivElement>;
  section: Section;
  waiting: number;
  user: SessionUser | undefined;
  role: Role | undefined;
  rail: boolean;
  modal: boolean;
  signingOut: boolean;
  onToggleRail(): void;
  onNavigate(): void;
  onClose(): void;
  onSignOut(): void;
};

const FRAME =
  "group/side fixed inset-y-0 left-0 z-40 invisible flex w-72 max-w-[85vw] -translate-x-full flex-col border-r border-line bg-ink shadow-2xl shadow-black/60 transition-[translate,visibility,width] duration-250 ease-(--ease-glide) data-drawer:visible data-drawer:translate-x-0 lg:relative lg:z-30 lg:shrink-0 lg:visible lg:w-64 lg:translate-x-0 lg:border-r-0 lg:bg-transparent lg:shadow-none lg:data-rail:w-[4.5rem]";

const HANDLE = `absolute top-5 -right-3 z-10 hidden size-6 place-items-center rounded-full border border-line-strong bg-ink-raised text-muted hover:text-paper lg:grid ${TOUCH} ${PRESS}`;

/** The console's left sidebar, on the page's own ground: the logotype with its collapse handle, the links, and at the foot the user's menu and the language. On a wide screen it stays beside the page and can shrink to an icon rail; on a narrow one it is a drawer, a modal dialog that traps Tab, closes on Escape and gives focus back to the button that opened it. */
export function Sidebar({ id, ref, section, waiting, user, role, rail, modal, signingOut, onToggleRail, onNavigate, onClose, onSignOut }: SidebarProps) {
  const t = useT();
  return (
    <div
      id={id}
      ref={ref}
      role={modal ? "dialog" : undefined}
      aria-modal={modal ? true : undefined}
      aria-label={modal ? t("nav.label") : undefined}
      tabIndex={modal ? -1 : undefined}
      data-rail={rail ? "" : undefined}
      data-drawer={modal ? "" : undefined}
      className={FRAME}
      onKeyDown={(event) => {
        if (!modal) return;
        if (closesDrawer(event.key, modal, event.defaultPrevented)) {
          event.preventDefault();
          onClose();
          return;
        }
        if (event.key !== "Tab") return;
        const controls = [...event.currentTarget.querySelectorAll<HTMLElement>("a[href], button:not(:disabled)")].filter((control) => control.checkVisibility());
        const next = wrapFocus(controls.length, controls.findIndex((control) => control === document.activeElement), event.shiftKey);
        if (next === undefined) return;
        event.preventDefault();
        controls[next]?.focus();
      }}
    >
      <div className="flex h-16 shrink-0 items-center justify-between pr-3 pl-[1.375rem]">
        <a
          href="/"
          aria-label={t("app.home")}
          className={`relative flex min-w-0 items-center gap-2.5 overflow-hidden ${TOUCH}`}
          onClick={(event) => {
            event.preventDefault();
            navigate("/");
            onNavigate();
          }}
        >
          <Mark className="size-7" />
          <span className="flex min-w-0 flex-col transition-opacity duration-250 group-data-rail/side:opacity-0">
            <Logotype className="text-[0.9375rem]" />
            <span className="mt-0.5 truncate font-mono text-[0.6875rem] leading-none text-muted">{t("brand.label")}</span>
          </span>
        </a>
        <button type="button" aria-label={t("shell.closeMenu")} className={`${ICON_BUTTON} lg:hidden`} onClick={onClose}>
          <Icon name="close" className="size-4" />
        </button>
      </div>
      <button type="button" aria-controls={id} aria-label={rail ? t("shell.expand") : t("shell.collapse")} title={rail ? t("shell.expand") : t("shell.collapse")} className={HANDLE} onClick={onToggleRail}>
        <Icon name="back" className={`size-3.5 transition-transform duration-250 ease-(--ease-glide) ${rail ? "rotate-180" : ""}`} />
      </button>
      <SideNav section={section} waiting={waiting} rail={rail} onNavigate={onNavigate} />
      <div className="shrink-0 border-t border-line px-3 py-3">
        <UserMenu user={user} role={role} rail={rail} signingOut={signingOut} onSignOut={onSignOut} onNavigate={onNavigate} />
        <div className="mt-2 flex flex-col gap-1.5 group-data-rail/side:hidden">
          <ThemeSwitch className="w-full" />
          <LanguageSwitch className="w-full" />
        </div>
      </div>
    </div>
  );
}
