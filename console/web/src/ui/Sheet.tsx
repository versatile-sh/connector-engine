import { useEffect, useId, useRef, useState, type ReactNode, type RefObject } from "react";
import { Icon } from "./Icon";
import { morph } from "./motion";
import { ICON_BUTTON } from "./styles";

type SheetProps = { title: string; closeLabel: string; onClose(): void; children: ReactNode; wide?: boolean; origin?: RefObject<HTMLElement | null> };

/** The one modal window, in the shape of a message sent; on a phone it fills the screen. It closes on its button, Escape or a click outside, and gives focus back to what opened it; given an `origin`, it grows out of that element and shrinks back into it. */
export function Sheet({ title, closeLabel, onClose, children, wide = false, origin }: SheetProps) {
  const dialog = useRef<HTMLDialogElement>(null);
  const motion = useRef<Animation>(undefined);
  const heading = useId();
  const [opener] = useState(() => document.activeElement);
  const [closing, setClosing] = useState(false);
  useEffect(
    function showOverThePage() {
      const element = dialog.current;
      if (element && !element.open) {
        element.showModal();
        if (origin?.current) motion.current = morph(element, origin.current);
      }
      return () => {
        const back = origin?.current ?? opener;
        if (back instanceof HTMLElement) back.focus();
      };
    },
    [opener, origin],
  );
  const dismiss = () => {
    const element = dialog.current;
    const into = origin?.current;
    if (!element || !into) return onClose();
    if (closing) return;
    setClosing(true);
    const running = motion.current?.playState === "running" ? motion.current : undefined;
    running?.reverse();
    const close = () => element.close();
    void (running ?? morph(element, into, true)).finished.then(close, close);
  };
  return (
    <dialog
      ref={dialog}
      aria-labelledby={heading}
      onClose={onClose}
      onKeyDown={(event) => {
        if (event.key !== "Escape" || event.defaultPrevented) return;
        event.preventDefault();
        dismiss();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) dismiss();
      }}
      className={`sheet m-auto max-h-[calc(100dvh-2rem)] max-w-none flex-col overflow-hidden vignette-top bubble-sent border border-line-strong bg-ink-raised p-0 text-paper backdrop:bg-ink/70 backdrop:backdrop-blur-xs open:flex max-md:h-dvh max-md:max-h-none max-md:w-full max-md:border-0 ${wide ? "w-[min(52rem,calc(100vw-2rem))]" : "w-[min(30rem,calc(100vw-2rem))]"} ${origin ? "morph" : ""} ${closing ? "closing" : ""}`}
    >
      <div className="flex shrink-0 items-center justify-between gap-3 pt-3 pr-3 pb-1 pl-6">
        <h2 id={heading} className="min-w-0 font-serif text-2xl leading-tight font-normal tracking-[-0.01em] [overflow-wrap:anywhere]">{title}</h2>
        <button type="button" aria-label={closeLabel} className={ICON_BUTTON} onClick={dismiss}>
          <Icon name="close" className="size-4" />
        </button>
      </div>
      <div className="no-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 pt-2 pb-6">{children}</div>
    </dialog>
  );
}
