import { useEffect, useRef, useState, type ReactNode } from "react";
import { Icon } from "./Icon";
import { ICON_BUTTON } from "./styles";

type SheetProps = { title: string; closeLabel: string; onClose(): void; children: ReactNode; wide?: boolean };

/** The one modal window, in the shape of a message sent; on a phone it fills the screen. It closes on its button, Escape or a click outside, and gives focus back to what opened it. */
export function Sheet({ title, closeLabel, onClose, children, wide = false }: SheetProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [opener] = useState(() => document.activeElement);
  useEffect(() => {
    const element = dialog.current;
    if (element && !element.open) element.showModal();
    return () => {
      if (opener instanceof HTMLElement) opener.focus();
    };
  }, [opener]);
  return (
    <dialog
      ref={dialog}
      aria-label={title}
      onClose={onClose}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
      className={`sheet m-auto max-h-[calc(100dvh-2rem)] max-w-none flex-col overflow-hidden vignette-top bubble-sent border border-line-strong bg-ink-raised p-0 text-paper backdrop:bg-ink/70 open:flex max-md:size-full max-md:max-h-none max-md:border-0 ${wide ? "w-[min(52rem,calc(100vw-2rem))]" : "w-[min(30rem,calc(100vw-2rem))]"}`}
    >
      <div className="flex shrink-0 items-center justify-between gap-3 py-3 pr-3 pl-6">
        <h2 className="font-serif text-2xl leading-none font-normal tracking-[-0.01em]">{title}</h2>
        <button type="button" aria-label={closeLabel} className={ICON_BUTTON} onClick={onClose}>
          <Icon name="close" className="size-4" />
        </button>
      </div>
      <div className="no-scrollbar min-h-0 flex-1 overflow-y-auto px-6 pb-6">{children}</div>
    </dialog>
  );
}
