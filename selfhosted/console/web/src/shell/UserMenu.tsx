import { useEffect, useEffectEvent, useId, useRef, useState } from "react";
import type { SessionUser } from "../api";
import { useT } from "../i18n/I18nProvider";
import { LanguageSwitch } from "../LanguageSwitch";
import { navigate } from "../router";
import { Icon } from "../ui/Icon";
import { place, within } from "../ui/popup";
import { PRESS, TOUCH } from "../ui/styles";
import { initialOf } from "./shell";

type UserMenuProps = { user: SessionUser | undefined; rail: boolean; signingOut: boolean; onSignOut(): void; onNavigate(): void };

const ITEM = `relative flex min-h-9 w-full items-center gap-2.5 rounded-lg px-2.5 text-left text-sm text-muted hover:bg-paper/10 hover:text-paper focus-visible:bg-paper/10 focus-visible:text-paper disabled:opacity-50 ${TOUCH} ${PRESS}`;

/** The signed-in user at the foot of the sidebar: a round mark with their name and role, opening a small panel with Account and Sign out, and, in the rail, the language too. It closes on Escape, giving focus back to its button, and on a press or focus outside it. */
export function UserMenu({ user, rail, signingOut, onSignOut, onNavigate }: UserMenuProps) {
  const t = useT();
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const name = user?.username ?? t("nav.account");

  const show = () => {
    const list = popup.current;
    const button = trigger.current;
    if (!list || !button) return;
    list.showPopover();
    place(list, button);
    setOpen(true);
    list.querySelector<HTMLElement>("a")?.focus();
  };
  const hide = () => {
    popup.current?.hidePopover();
    setOpen(false);
  };
  const close = () => {
    hide();
    trigger.current?.focus();
  };

  const dismiss = useEffectEvent((event: Event) => {
    if (!within(event.target, popup.current, trigger.current)) hide();
  });
  const resize = useEffectEvent(() => hide());
  useEffect(
    function closeOnOutsideActivity() {
      if (!open) return;
      const controller = new AbortController();
      const signal = controller.signal;
      document.addEventListener("pointerdown", dismiss, { capture: true, signal });
      document.addEventListener("focusin", dismiss, { signal });
      window.addEventListener("resize", resize, { signal });
      return () => controller.abort();
    },
    [open],
  );

  return (
    <>
      <button
        ref={trigger}
        type="button"
        aria-label={t("shell.userMenu", { name })}
        aria-expanded={open}
        aria-controls={id}
        onClick={() => (open ? close() : show())}
        onKeyDown={(event) => {
          if (event.key !== "Escape" || !open) return;
          event.preventDefault();
          close();
        }}
        className={`group/user relative flex h-12 w-full items-center gap-3 rounded-xl border border-transparent px-2 text-left hover:bg-paper/10 aria-expanded:bg-paper/10 ${TOUCH} ${PRESS}`}
      >
        <span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-full border border-line-strong bg-ink-raised font-mono text-xs text-paper">
          {initialOf(name)}
        </span>
        <span className="flex min-w-0 flex-1 flex-col transition-opacity duration-250 group-data-rail/side:opacity-0">
          <span className="truncate text-sm leading-tight text-paper" translate="no">
            {name}
          </span>
          {user ? <span className="truncate font-mono text-xs leading-tight text-muted">{t(`role.${user.role}`)}</span> : null}
        </span>
        <Icon name="chevron" className="size-4 text-muted transition-[rotate,opacity] duration-250 ease-(--ease-glide) group-aria-expanded/user:rotate-180 group-data-rail/side:opacity-0" />
      </button>
      <div
        ref={popup}
        id={id}
        popover="manual"
        role="group"
        aria-label={t("shell.userMenu", { name })}
        onKeyDown={(event) => {
          if (event.key !== "Escape") return;
          event.preventDefault();
          close();
        }}
        className="select-list vignette-top inset-auto m-0 w-60 max-w-80 flex-col gap-0.5 overflow-hidden rounded-xl border border-line-strong bg-ink-raised p-1 text-paper shadow-xl shadow-black/40 open:flex max-sm:inset-x-2 max-sm:bottom-2 max-sm:w-auto max-sm:max-w-none max-sm:rounded-2xl max-sm:backdrop:bg-ink-sunken/60 sm:top-(--select-top) sm:bottom-(--select-bottom) sm:left-(--select-left) sm:max-h-[min(20rem,var(--select-room,20rem))] sm:w-(--select-width)"
      >
        {rail ? (
          <div className="flex flex-col gap-1.5 px-2.5 pt-2 pb-2">
            <p className="truncate text-sm text-paper" translate="no">
              {name}
            </p>
            {user ? <p className="font-mono text-xs text-muted">{t(`role.${user.role}`)}</p> : null}
            <LanguageSwitch className="mt-1.5 w-full" />
          </div>
        ) : null}
        <a
          href="/account"
          className={ITEM}
          onClick={(event) => {
            if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
            event.preventDefault();
            hide();
            navigate("/account");
            onNavigate();
          }}
        >
          <Icon name="user" className="size-4" />
          {t("nav.account")}
        </a>
        <button type="button" disabled={signingOut} className={ITEM} onClick={onSignOut}>
          <Icon name="logout" className="size-4" />
          {t("shell.signOut")}
        </button>
      </div>
    </>
  );
}
