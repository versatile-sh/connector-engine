import { useLayoutEffect, useRef } from "react";
import { flip, type Move } from "../ui/motion";

type Pending = { keep: HTMLElement; was: { element: HTMLElement; box: DOMRect }[] };

/** Lets a list's items glide from where they stood to where a change puts them: call `before(keep)` just ahead of the change; once `key` changes, `keep` is scrolled into view and every moved item slides from its old place on its own CSS transform transition. */
export function useReflow<T extends HTMLElement>(key: unknown) {
  const list = useRef<T>(null);
  const pending = useRef<Pending>(undefined);
  useLayoutEffect(
    function glideItemsToTheirNewPlaces() {
      const change = pending.current;
      pending.current = undefined;
      if (!change) return;
      const moves: Move[] = [];
      for (const { element, box } of change.was) {
        const now = element.getBoundingClientRect();
        const x = box.left - now.left;
        const y = box.top - now.top;
        if (element.isConnected && (x || y)) moves.push({ element, from: `translate(${x}px, ${y}px)`, to: "" });
      }
      change.keep.scrollIntoView({ block: "nearest" });
      flip(moves);
    },
    [key],
  );
  const before = (keep: HTMLElement) => {
    const items = list.current?.querySelectorAll<HTMLElement>(":scope > li") ?? [];
    pending.current = { keep, was: Array.from(items, (element) => ({ element, box: element.getBoundingClientRect() })) };
  };
  return { list, before };
}
