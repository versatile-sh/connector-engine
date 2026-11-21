import { useSyncExternalStore } from "react";

/** The theme that is drawn. */
export type Theme = "light" | "dark";

/** A point of the viewport, in pixels. */
export type Point = { x: number; y: number };

const KEY = "console.theme";
const COLORS: Record<Theme, string> = { light: "#f4f4f0", dark: "#0e1116" };
const REVEAL_MS = 450;

/** The saved theme when it is a known one, else null: follow the system. */
export function pickTheme(saved: string | null): Theme | null {
  return saved === "light" || saved === "dark" ? saved : null;
}

/** The theme to draw: the saved one, else what the system prefers. */
export function resolveTheme(saved: Theme | null, systemLight: boolean): Theme {
  return saved ?? (systemLight ? "light" : "dark");
}

/** The other theme. */
export function otherTheme(theme: Theme): Theme {
  return theme === "light" ? "dark" : "light";
}

/** The radius of the circle from a point that covers a viewport: the distance to its farthest corner. */
export function revealRadius(from: Point, width: number, height: number): number {
  return Math.hypot(Math.max(from.x, width - from.x), Math.max(from.y, height - from.y));
}

const listeners = new Set<() => void>();
const systemLight = () => window.matchMedia("(prefers-color-scheme: light)");

function saved(): Theme | null {
  try {
    return pickTheme(window.localStorage.getItem(KEY));
  } catch {
    return null;
  }
}

function draw(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", COLORS[theme]);
  for (const listener of listeners) listener();
}

function drawSaved() {
  draw(resolveTheme(saved(), systemLight().matches));
}

/** Keeps the page in step with the system and with other tabs; call once at start. */
export function startTheme() {
  drawSaved();
  systemLight().addEventListener("change", drawSaved);
  window.addEventListener("storage", (event) => event.key === KEY && drawSaved());
}

/** Draws the other theme and keeps it; with a view-transition browser and motion allowed, the new theme grows as a circle from `from`. */
export function toggleTheme(from: Point) {
  const next = otherTheme(currentTheme());
  try {
    window.localStorage.setItem(KEY, next);
  } catch {
    draw(next);
    return;
  }
  if (!document.startViewTransition || window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    draw(next);
    return;
  }
  const radius = revealRadius(from, window.innerWidth, window.innerHeight);
  const transition = document.startViewTransition(() => draw(next));
  void transition.ready.then(() =>
    document.documentElement.animate(
      { clipPath: [`circle(0px at ${from.x}px ${from.y}px)`, `circle(${radius}px at ${from.x}px ${from.y}px)`] },
      { duration: REVEAL_MS, easing: "cubic-bezier(0.2, 0.8, 0.2, 1)", pseudoElement: "::view-transition-new(root)" },
    ),
  );
}

function currentTheme(): Theme {
  return document.documentElement.dataset.theme === "light" ? "light" : "dark";
}

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => void listeners.delete(listener);
};

/** The theme drawn now, shared by every switch on the page. */
export function useTheme(): Theme {
  return useSyncExternalStore(subscribe, currentTheme, () => "dark");
}
