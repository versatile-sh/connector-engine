import type { ReactNode } from "react";
import { Wordmark } from "./Brand";
import { useT } from "./i18n/I18nProvider";
import { LanguageSwitch } from "./LanguageSwitch";
import { navigate } from "./router";

type FrameProps = { width: "narrow" | "wide"; home?: boolean; children: ReactNode };

const WIDTH = { narrow: "max-w-xl", wide: "max-w-6xl" } as const;

/** The frame of a page that stands outside the console's sections, as setup, sign-in, an install link, an app's consent and one held call: the site's grid and light, the wordmark and the language on top, and one centred column, narrow for a decision, wide for an editorial page. */
export function Frame({ width, home = false, children }: FrameProps) {
  const t = useT();
  return (
    <div className="relative isolate flex min-h-dvh flex-col overflow-x-clip">
      <div aria-hidden="true" className="grid-base2 pointer-events-none absolute inset-x-0 top-0 -z-10 h-[40rem]" />
      <div aria-hidden="true" className="light-beam pointer-events-none absolute inset-x-0 top-0 -z-10 h-[32rem]" />
      <header className={`mx-auto flex w-full items-center justify-between gap-4 px-5 pt-5 sm:px-8 sm:pt-6 ${WIDTH[width]}`}>
        {home ? (
          <a
            href="/"
            aria-label={t("app.home")}
            onClick={(event) => {
              event.preventDefault();
              navigate("/");
            }}
          >
            <Wordmark />
          </a>
        ) : (
          <Wordmark />
        )}
        <LanguageSwitch />
      </header>
      <main id="main" tabIndex={-1} className={`mx-auto flex w-full min-w-0 flex-1 flex-col justify-center px-5 py-10 sm:px-8 sm:py-14 ${WIDTH[width]}`}>
        {children}
      </main>
    </div>
  );
}
