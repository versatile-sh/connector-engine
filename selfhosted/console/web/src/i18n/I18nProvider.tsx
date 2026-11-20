import { createContext, use, useMemo, useState, type ReactNode } from "react";
import { createTranslate, pickLocale, type Locale, type Translate } from "./index";

const KEY = "console.locale";

type I18n = { locale: Locale; t: Translate; choose(locale: Locale): void };

const I18nContext = createContext<I18n | undefined>(undefined);

const savedLocale = () => {
  try {
    return window.localStorage.getItem(KEY);
  } catch {
    return null;
  }
};

/** Makes `useT` available below it, in the saved language or the browser's; a choice is kept in localStorage. */
export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState(() => {
    const initial = pickLocale(savedLocale(), navigator.languages);
    document.documentElement.lang = initial;
    return initial;
  });
  const value = useMemo(
    () => ({
      locale,
      t: createTranslate(locale),
      choose(next: Locale) {
        setLocale(next);
        document.documentElement.lang = next;
        try {
          window.localStorage.setItem(KEY, next);
        } catch {
          return;
        }
      },
    }),
    [locale],
  );
  return <I18nContext value={value}>{children}</I18nContext>;
}

function useI18n(): I18n {
  const value = use(I18nContext);
  if (!value) throw new Error("useT needs an I18nProvider above it");
  return value;
}

/** The translate function of the current locale. */
export function useT(): Translate {
  return useI18n().t;
}

/** The current locale, for dates, numbers and localised links. */
export function useLocale(): Locale {
  return useI18n().locale;
}

/** Changes the language of the console. */
export function useChooseLocale(): (locale: Locale) => void {
  return useI18n().choose;
}
