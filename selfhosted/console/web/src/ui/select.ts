import { shownOf } from "./longList";
import type { Box } from "./motion";

/** How many options a `Select` holds before its list opens with a filter on top. */
export const SEARCH_FROM = 8;

/** How long typed letters keep adding up to one typeahead search, in milliseconds. */
export const TYPEAHEAD_MS = 500;

/** The options a list shows for its filter: those whose text holds it, ignoring case and outer spaces, or every one with no filter. */
export function matching<T extends { text: string }>(options: readonly T[], query: string): readonly T[] {
  return shownOf(options, (option) => option.text, { filter: query, all: true }).shown;
}

/** A list, open or closed: the row the keyboard is on, the filter typed over it, and the letters typed ahead with the time of the last one. */
export type ListState = { open: boolean; active: number; query: string; typed: string; at: number };

/** A key that moves the keyboard's row of an open list. */
export type Move = "ArrowDown" | "ArrowUp" | "Home" | "End";

/** What happens to a list: it opens on the chosen row (-1 for none), closes, moves, follows the pointer, filters, or jumps to the row whose text starts with the letters typed. */
export type ListAction =
  | { type: "open"; selected: number }
  | { type: "close" }
  | { type: "move"; key: Move; count: number }
  | { type: "point"; row: number }
  | { type: "filter"; query: string }
  | { type: "type"; char: string; at: number; texts: readonly string[] };

/** A closed list with nothing typed. */
export const CLOSED: ListState = { open: false, active: 0, query: "", typed: "", at: 0 };

const STEP: Record<Move, (active: number, last: number) => number> = {
  ArrowDown: (active, last) => Math.min(active + 1, last),
  ArrowUp: (active) => Math.max(active - 1, 0),
  Home: () => 0,
  End: (_, last) => last,
};

/** Whether a key moves the keyboard's row of an open list. */
export const isMove = (key: string): key is Move => Object.hasOwn(STEP, key);

function typedAhead(state: ListState, { char, at, texts }: Extract<ListAction, { type: "type" }>): ListState {
  const typed = (at - state.at < TYPEAHEAD_MS ? state.typed : "") + char.toLowerCase();
  const repeated = [...typed].every((letter) => letter === typed[0]);
  const needle = repeated ? char.toLowerCase() : typed;
  const from = repeated ? state.active + 1 : state.active;
  const row = texts.map((_, index) => (from + index) % texts.length).find((index) => texts[index]?.toLowerCase().startsWith(needle));
  return { ...state, typed, at, active: row ?? state.active };
}

/** The keyboard model of a list, as a native select's: opening lands on the chosen row, the arrows, Home and End move without wrapping, the pointer moves the row it is over, a filter starts again on its first match, and typing jumps to the next row whose text starts with the letters typed within half a second, one letter repeated cycling through the rows it starts. */
export function listbox(state: ListState, action: ListAction): ListState {
  switch (action.type) {
    case "open":
      return { ...CLOSED, open: true, active: Math.max(action.selected, 0) };
    case "close":
      return CLOSED;
    case "move":
      return { ...state, active: STEP[action.key](state.active, Math.max(action.count - 1, 0)) };
    case "point":
      return action.row === state.active ? state : { ...state, active: action.row };
    case "filter":
      return { ...state, query: action.query, active: 0 };
    case "type":
      return typedAhead(state, action);
  }
}

const GAP = 4;

const EDGE = 8;

/** A width and a height in pixels. */
type Size = { width: number; height: number };

/** Where an open list sits, in viewport pixels: its left edge, its width, the most height it may take, and its top under the trigger or its bottom over it. */
export type Placement = { left: number; width: number; room: number } & ({ up: false; top: number } | { up: true; bottom: number });

/** Where a list of the given size opens beside its trigger: under it, or over it when it does not fit under and there is more room above; at least as wide as the trigger, and inside the viewport, keeping 8px from its edges. */
export function placement(trigger: Box, list: Size, viewport: Size): Placement {
  const below = viewport.height - trigger.top - trigger.height - GAP - EDGE;
  const above = trigger.top - GAP - EDGE;
  const width = Math.min(Math.max(list.width, trigger.width), viewport.width - 2 * EDGE);
  const left = Math.max(EDGE, Math.min(trigger.left, viewport.width - EDGE - width));
  if (list.height > below && above > below) return { up: true, bottom: viewport.height - trigger.top + GAP, left, width, room: above };
  return { up: false, top: trigger.top + trigger.height + GAP, left, width, room: below };
}
