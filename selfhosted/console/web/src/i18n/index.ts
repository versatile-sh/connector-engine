import { en, type MessageKey } from "./en";
import { es } from "./es";
import { ptBR } from "./pt-br";

/** The keys of the message catalogue. */
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

const PLURAL = /\{(\w+), plural,((?:\s*(?:=\d+|zero|one|two|few|many|other)\s*\{[^{}]*\})+)\s*\}/g;

const BRANCH = /(=\d+|zero|one|two|few|many|other)\s*\{([^{}]*)\}/g;

const PLACEHOLDER = /\{(\w+)\}/g;

/** Fills a message: each `{name, plural, =0 {…} one {…} other {…}}` takes the branch of its number, an exact `=n` first, then the locale's plural rule, then `other`, with `#` as the number; then each `{name}` takes its value. */
export function format(text: string, params: Params | undefined, rules: Intl.PluralRules): string {
  return text
    .replace(PLURAL, (whole, name: string, body: string) => {
      const given = params?.[name];
      const value = Number(given);
      if (given === undefined || given === "" || !Number.isFinite(value)) return whole;
      const branches = new Map([...body.matchAll(BRANCH)].map(([, selector, branch]) => [selector, branch]));
      const branch = branches.get(`=${value}`) ?? branches.get(rules.select(value)) ?? branches.get("other");
      return branch === undefined ? whole : branch.replaceAll("#", String(value));
    })
    .replace(PLACEHOLDER, (whole, name: string) => String(params?.[name] ?? whole));
}

/** The names a message fills, its plural numbers included, sorted. */
export function paramsOf(text: string): string[] {
  const plurals = [...text.matchAll(PLURAL)].map((match) => match[1] ?? "");
  const plain = [...text.replace(PLURAL, "").matchAll(PLACEHOLDER)].map((match) => match[1] ?? "");
  return [...new Set([...plurals, ...plain])].sort();
}

/** Builds the translate function of a locale. */
export function createTranslate(locale: Locale): Translate {
  const dictionary = DICTIONARIES[locale];
  const rules = new Intl.PluralRules(locale);
  return (key, params) => format(dictionary[key], params, rules);
}
