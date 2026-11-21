type Move = { element: HTMLElement; from: string; to: string };

/** Draws each element at its `from` transform at once, then, after one style flush, lets its own CSS transition carry it to `to`; with transitions off, as under reduced motion, it lands there at once. */
export function flip(moves: readonly Move[]) {
  for (const { element, from } of moves) {
    element.style.transition = "none";
    element.style.transform = from;
  }
  void moves[0]?.element.offsetWidth;
  for (const { element, to } of moves) {
    element.style.removeProperty("transition");
    element.style.transform = to;
  }
}

/** Where the iris of a pressed link opens inside its `width` by `height` box: at the pointer, or the centre for a key press, with the radius that reaches the farthest corner. */
export function irisAt(width: number, height: number, point?: { x: number; y: number }) {
  const x = point?.x ?? width / 2;
  const y = point?.y ?? height / 2;
  return { x, y, r: Math.hypot(Math.max(x, width - x), Math.max(y, height - y)) };
}

/** A box on the screen, as `getBoundingClientRect` gives it. */
export type Box = { left: number; top: number; width: number; height: number };

/** The transform that draws an element laid out at `to` over `from` instead, scaled about its centre: the first frame of a FLIP between the two boxes. */
export function boxTransform(from: Box, to: Box): string {
  const x = from.left + from.width / 2 - (to.left + to.width / 2);
  const y = from.top + from.height / 2 - (to.top + to.height / 2);
  return `translate(${x}px, ${y}px) scale(${from.width / to.width}, ${from.height / to.height})`;
}

/** Plays `element` growing out of `origin`'s box into its own, or shrinking back into it when `reverse`, on transform and opacity for 250ms on the site's glide; under reduced motion it only fades. */
export function morph(element: HTMLElement, origin: Element, reverse = false): Animation {
  const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const from = still ? "none" : boxTransform(origin.getBoundingClientRect(), element.getBoundingClientRect());
  return element.animate([{ transform: from, opacity: 0 }, { transform: "none", opacity: 1 }], {
    duration: 250,
    easing: getComputedStyle(element).getPropertyValue("--ease-glide"),
    fill: "both",
    direction: reverse ? "reverse" : "normal",
  });
}
