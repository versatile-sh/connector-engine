import type { Locale } from "./i18n";

/** The connector hub's origin: change it here and every hub link follows. */
export const HUB_ORIGIN = "https://versatile.sh";

const HUB_PATH: Record<Locale, string> = { en: "/connectors", "pt-BR": "/pt/connectors", es: "/es/connectors" };

/** The hub's connector catalogue in the console's language. */
export const hubURL = (locale: Locale) => `${HUB_ORIGIN}${HUB_PATH[locale]}`;
