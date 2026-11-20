/** What a browser says about itself: the user-agent client hints where it gives them, and the user-agent string. */
export type BrowserFacts = { userAgent: string; hints?: { platform: string; brands: readonly { brand: string }[] } };

/** The longest name the console takes for a computer. */
export const DEVICE_NAME_MAX = 60;

const KEY = "console.device";

const isHints = (value: unknown): value is NonNullable<BrowserFacts["hints"]> =>
  typeof value === "object" &&
  value !== null &&
  "platform" in value &&
  typeof value.platform === "string" &&
  "brands" in value &&
  Array.isArray(value.brands) &&
  value.brands.every((entry: unknown) => typeof entry === "object" && entry !== null && "brand" in entry && typeof entry.brand === "string");

/** Reads the facts off a navigator; `userAgentData` is not in every browser, nor in the DOM types. */
export function browserFacts(navigator: { userAgent: string }): BrowserFacts {
  const hints = "userAgentData" in navigator ? navigator.userAgentData : undefined;
  return isHints(hints) ? { userAgent: navigator.userAgent, hints } : { userAgent: navigator.userAgent };
}

const PLATFORMS: Record<string, string> = { "Chrome OS": "ChromeOS", "Chromium OS": "ChromeOS" };

const BRANDS: Record<string, string> = { "Google Chrome": "Chrome", "Microsoft Edge": "Edge" };

const OS_PATTERNS: readonly [RegExp, string][] = [
  [/Windows/, "Windows"],
  [/iPhone|iPad|iPod/, "iOS"],
  [/Mac OS X|Macintosh/, "macOS"],
  [/Android/, "Android"],
  [/CrOS/, "ChromeOS"],
  [/Linux/, "Linux"],
];

const BROWSER_PATTERNS: readonly [RegExp, string][] = [
  [/Edg(e|A|iOS)?\//, "Edge"],
  [/OPR\/|Opera/, "Opera"],
  [/Firefox\/|FxiOS/, "Firefox"],
  [/Chrome\/|CriOS/, "Chrome"],
  [/Safari\//, "Safari"],
];

const matchOf = (text: string, patterns: readonly [RegExp, string][]) => patterns.find(([pattern]) => pattern.test(text))?.[1];

const brandOf = (brands: readonly { brand: string }[]) => {
  const real = brands.map((entry) => entry.brand).filter((brand) => !/^not.?a.?brand$/i.test(brand));
  const brand = real.find((name) => name !== "Chromium") ?? real[0];
  return brand && (BRANDS[brand] ?? brand);
};

/** A name for the computer this browser runs on, "<OS> · <browser>", from the client hints first, then the user-agent string; empty when neither is known, so the console names it. */
export function deviceName({ userAgent, hints }: BrowserFacts): string {
  const platform = hints?.platform && hints.platform !== "Unknown" ? (PLATFORMS[hints.platform] ?? hints.platform) : undefined;
  const os = platform ?? matchOf(userAgent, OS_PATTERNS);
  const browser = (hints && brandOf(hints.brands)) || matchOf(userAgent, BROWSER_PATTERNS);
  return [os, browser].filter(Boolean).join(" · ").slice(0, DEVICE_NAME_MAX);
}

/** The id of the computer this browser paired, when it paired one; the key never leaves the page that showed it. */
export function pairedHere(): string | null {
  try {
    return window.localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

/** Remembers that this browser paired the computer with this id, and answers the id. */
export function rememberPairedHere(id: string): string {
  try {
    window.localStorage.setItem(KEY, id);
  } catch {
    return id;
  }
  return id;
}
