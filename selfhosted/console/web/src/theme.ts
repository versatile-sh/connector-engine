import { useSyncExternalStore } from "react";

/** What the person picked: follow the system, or force one theme. */
export type ThemeChoice = "system" | "light" | "dark";

/** The theme that is drawn. */
export type Theme = "light" | "dark";

const KEY = "console.theme";
const COLORS: Record<Theme, string> = { light: "#f4f4f0", dark: "#0e1116" };

/** Every choice, in the order the switch shows them. */
export const THEME_CHOICES: readonly ThemeChoice[] = ["system", "light", "dark"];

/** The saved choice when it is a known one, else follow the system. */
export function pickTheme(saved: string | null): ThemeChoice {
  return saved === "light" || saved === "dark" ? saved : "system";
}

/** The theme a choice draws, given whether the system prefers light. */
export function resolveTheme(choice: ThemeChoice, systemLight: boolean): Theme {
  if (choice === "system") return systemLight ? "light" : "dark";
  return choice;
}

const listeners = new Set<() => void>();
const systemLight = () => window.matchMedia("(prefers-color-scheme: light)");

function read(): ThemeChoice {
  try {
    return pickTheme(window.localStorage.getItem(KEY));
  } catch {
    return "system";
  }
}

function draw(choice: ThemeChoice) {
  const theme = resolveTheme(choice, systemLight().matches);
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", COLORS[theme]);
  for (const listener of listeners) listener();
}

/** Keeps the page in step with the system and with other tabs; call once at start. */
export function startTheme() {
  draw(read());
  systemLight().addEventListener("change", () => draw(read()));
  window.addEventListener("storage", (event) => event.key === KEY && draw(read()));
}

/** Saves a choice in localStorage and draws it at once. */
export function chooseTheme(choice: ThemeChoice) {
  try {
    window.localStorage.setItem(KEY, choice);
  } catch {
    return draw(choice);
  }
  draw(choice);
}

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => void listeners.delete(listener);
};

/** The saved theme choice, shared by every switch on the page. */
export function useThemeChoice(): ThemeChoice {
  return useSyncExternalStore(subscribe, read, () => "system");
}
