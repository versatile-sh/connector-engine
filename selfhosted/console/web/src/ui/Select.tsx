import { useEffect, useEffectEvent, useId, useReducer, useRef, type KeyboardEvent, type ReactNode } from "react";
import { useT } from "../i18n/I18nProvider";
import { Icon } from "./Icon";
import { SearchBox } from "./SearchBox";
import { CLOSED, isMove, listbox, matching, placement, SEARCH_FROM } from "./select";
import { CONTROL } from "./styles";

/** One choice of a `Select`: the value it sets, the text it shows and an optional leading icon. */
export type SelectOption<T extends string> = { value: T; text: string; icon?: ReactNode };

type SelectProps<T extends string> = { label: string; value: T | undefined; options: readonly SelectOption<T>[]; onChange(value: T): void; disabled?: boolean; className?: string };

const PLACED = ["--select-top", "--select-bottom", "--select-left", "--select-width", "--select-room"];

function place(list: HTMLElement, trigger: HTMLElement) {
  for (const name of PLACED) list.style.removeProperty(name);
  const viewport = { width: document.documentElement.clientWidth, height: document.documentElement.clientHeight };
  const at = placement(trigger.getBoundingClientRect(), { width: list.offsetWidth, height: list.offsetHeight }, viewport);
  list.toggleAttribute("data-up", at.up);
  if (at.up) list.style.setProperty("--select-bottom", `${at.bottom}px`);
  else list.style.setProperty("--select-top", `${at.top}px`);
  list.style.setProperty("--select-left", `${at.left}px`);
  list.style.setProperty("--select-width", `${at.width}px`);
  list.style.setProperty("--select-room", `${at.room}px`);
}

const reveal = (row: HTMLElement | null) => row?.scrollIntoView({ block: "nearest" });

const within = (target: EventTarget | null, ...elements: (HTMLElement | null)[]) => target instanceof Node && elements.some((element) => element?.contains(target));

/** The one dropdown, controlled as a native select: the field at control height with the chosen option and a chevron, opening a list in raised ink under it, or over it near the bottom, or as a sheet from the bottom on a phone. The row under the keyboard or the pointer is paper at 10%, the chosen one has a check, and past eight options a filter tops the list, focused at once only with a fine pointer so a phone's keyboard does not cover the list. The keyboard is the select-only combobox's: the arrows, Home, End, typing a name, Enter or Space to choose, Escape to close; the list closes on a click or focus outside it and follows its field as the page scrolls. */
export function Select<T extends string>({ label, value, options, onChange, disabled = false, className = "" }: SelectProps<T>) {
  const t = useT();
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const popup = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const [state, dispatch] = useReducer(listbox, CLOSED);
  const searchable = options.length > SEARCH_FROM;
  const shown = matching(options, state.query);
  const chosen = options.find((option) => option.value === value);
  const active = state.open ? shown[state.active] : undefined;
  const optionId = (option: SelectOption<T>) => `${id}-${options.indexOf(option)}`;
  const activeId = active ? optionId(active) : undefined;

  const show = () => {
    const list = popup.current;
    const button = trigger.current;
    if (!list || !button) return;
    dispatch({ type: "open", selected: options.findIndex((option) => option.value === value) });
    list.showPopover();
    place(list, button);
    reveal(list.querySelector<HTMLElement>('[aria-selected="true"]'));
    const field = window.matchMedia("(pointer: fine)").matches ? search.current : null;
    (field ?? button).focus();
  };
  const hide = () => {
    popup.current?.hidePopover();
    dispatch({ type: "close" });
  };
  const close = () => {
    hide();
    trigger.current?.focus();
  };
  const pick = (option: SelectOption<T>) => {
    close();
    if (option.value !== value) onChange(option.value);
  };

  const onKeyDown = (event: KeyboardEvent) => {
    const { key } = event;
    if (!state.open) {
      if (key === "ArrowDown" || key === "ArrowUp" || key === "Enter" || key === " ") {
        event.preventDefault();
        show();
      }
      return;
    }
    if (key === "Escape") {
      event.preventDefault();
      close();
    } else if (key === "Enter" || (key === " " && !searchable)) {
      event.preventDefault();
      if (active) pick(active);
    } else if (isMove(key) && (!searchable || key.startsWith("Arrow"))) {
      event.preventDefault();
      dispatch({ type: "move", key, count: shown.length });
    } else if (!searchable && key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      dispatch({ type: "type", char: key, at: event.timeStamp, texts: options.map((option) => option.text) });
    }
  };

  const dismiss = useEffectEvent((event: Event) => {
    if (!within(event.target, popup.current, trigger.current)) hide();
  });
  const follow = useEffectEvent((event: Event) => {
    const list = popup.current;
    const button = trigger.current;
    if (!list || !button || within(event.target, list)) return;
    const box = button.getBoundingClientRect();
    if (box.bottom < 0 || box.top > document.documentElement.clientHeight) hide();
    else place(list, button);
  });
  useEffect(
    function closeOnOutsideActivity() {
      if (!state.open) return;
      const controller = new AbortController();
      const signal = controller.signal;
      document.addEventListener("pointerdown", dismiss, { capture: true, signal });
      document.addEventListener("focusin", dismiss, { signal });
      document.addEventListener("scroll", follow, { capture: true, signal });
      window.addEventListener("resize", follow, { signal });
      return () => controller.abort();
    },
    [state.open],
  );

  return (
    <>
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-label={label}
        aria-expanded={state.open}
        aria-controls={`${id}-list`}
        aria-activedescendant={searchable ? undefined : activeId}
        disabled={disabled}
        onClick={() => (state.open ? close() : show())}
        onKeyDown={onKeyDown}
        onKeyUp={(event) => event.key === " " && event.preventDefault()}
        className={`${CONTROL} group inline-flex items-center gap-2 text-left disabled:opacity-50 ${className}`}
      >
        {chosen?.icon}
        <span className={`min-w-0 flex-1 truncate ${chosen ? "" : "text-muted"}`}>{chosen?.text ?? "—"}</span>
        <Icon name="chevron" className="size-4 text-muted transition-transform duration-200 ease-(--ease-glide) group-aria-expanded:rotate-180" />
      </button>
      <div
        ref={popup}
        popover="manual"
        onClick={(event) => event.target === event.currentTarget && close()}
        className="select-list vignette-top inset-auto m-0 flex max-w-80 flex-col overflow-hidden rounded-xl border border-line-strong bg-ink-raised p-0 text-paper shadow-xl shadow-black/40 max-sm:inset-x-2 max-sm:bottom-2 max-sm:max-h-[min(24rem,70dvh)] max-sm:w-auto max-sm:max-w-none max-sm:rounded-2xl max-sm:backdrop:bg-ink-sunken/60 sm:top-(--select-top) sm:bottom-(--select-bottom) sm:left-(--select-left) sm:max-h-[min(20rem,var(--select-room,20rem))] sm:w-(--select-width)"
      >
        {searchable ? (
          <SearchBox
            ref={search}
            label={t("select.filter")}
            value={state.query}
            role="combobox"
            aria-expanded={state.open}
            aria-controls={`${id}-list`}
            aria-autocomplete="list"
            aria-activedescendant={activeId}
            onChange={(event) => dispatch({ type: "filter", query: event.target.value })}
            onKeyDown={onKeyDown}
            className="m-1 mb-0 shrink-0"
          />
        ) : null}
        <ul id={`${id}-list`} role="listbox" aria-label={label} onMouseDown={(event) => event.preventDefault()} className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-1">
          {shown.map((option, row) => (
            <li
              key={option.value}
              ref={option === active ? reveal : undefined}
              id={optionId(option)}
              role="option"
              aria-selected={option.value === value}
              data-active={option === active ? "" : undefined}
              onPointerMove={() => dispatch({ type: "point", row })}
              onClick={() => pick(option)}
              className="flex min-h-9 cursor-pointer items-center gap-2 rounded-lg px-2.5 text-sm text-muted transition-colors duration-150 select-none aria-selected:text-paper data-active:bg-paper/10 data-active:text-paper pointer-coarse:min-h-11"
            >
              {option.icon}
              <span className="min-w-0 flex-1 truncate">{option.text}</span>
              <Icon name="check" className={`size-4 ${option.value === value ? "" : "invisible"}`} />
            </li>
          ))}
        </ul>
        {shown.length === 0 ? <p className="px-3.5 pb-3 text-sm text-muted">{t("list.noMatch")}</p> : null}
      </div>
    </>
  );
}
