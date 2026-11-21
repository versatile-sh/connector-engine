import { placement } from "./select";

const PLACED = ["--select-top", "--select-bottom", "--select-left", "--select-width", "--select-room"];

/** Sets the CSS variables of a `select-list` popover so that it opens under its trigger, or over it when it does not fit, inside the viewport. */
export function place(list: HTMLElement, trigger: HTMLElement) {
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

/** Whether an event's target lies inside one of the elements. */
export const within = (target: EventTarget | null, ...elements: (HTMLElement | null)[]) => target instanceof Node && elements.some((element) => element?.contains(target));
