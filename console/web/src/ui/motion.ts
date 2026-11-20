/** One element's move in a FLIP: the transform it is drawn at first and the one its CSS transition carries it to. */
export type Move = { element: HTMLElement; from: string; to: string };

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
