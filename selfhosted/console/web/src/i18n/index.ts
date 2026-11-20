import { en, type MessageKey } from "./en";
import { es } from "./es";
import { ptBR } from "./pt-br";

export type { MessageKey } from "./en";

/** The languages the console speaks. */
export type Locale = "en" | "pt-BR" | "es";

/** Every locale with the short name its switch shows. */
export const LOCALES: readonly { value: Locale; short: string }[] = [
  { value: "en", short: "EN" },
  { value: "pt-BR", short: "PT" },
  { value: "es", short: "ES" },
];

const DICTIONARIES: Record<Locale, Record<MessageKey, string>> = { en, "pt-BR": ptBR, es };

/** Values that fill the `{placeholders}` of a message. */
export type Params = Record<string, string | number>;

/** Looks up a message and fills its placeholders. */
export type Translate = (key: MessageKey, params?: Params) => string;

/** The saved choice when there is one, else the browser's language when the console speaks it, else English. */
export function pickLocale(saved: string | null, browser: readonly string[]): Locale {
  if (saved === "en" || saved === "pt-BR" || saved === "es") return saved;
  for (const tag of browser) {
    const lower = tag.toLowerCase();
    if (lower.startsWith("pt")) return "pt-BR";
    if (lower.startsWith("es")) return "es";
    if (lower.startsWith("en")) return "en";
  }
  return "en";
}

/** Builds the translate function of a locale. */
export function createTranslate(locale: Locale): Translate {
  const dictionary = DICTIONARIES[locale];
  return (key, params) => dictionary[key].replace(/\{(\w+)\}/g, (whole, name: string) => String(params?.[name] ?? whole));
}
