import { hubURL } from "./hub";
import { useLocale, useT } from "./i18n/I18nProvider";
import { QUIET_LINK } from "./ui/styles";
import { WORDMARK } from "./wordmarkPaths";

const TOP = 0.79 * WORDMARK.unitsPerEm;
const CUT = 0.2 * WORDMARK.unitsPerEm;
const STEP = 0.26;
const INK_AFTER = 0.42;
const SITE = "https://versatile.sh";

function watchWordmark(node: HTMLDivElement | null) {
  if (!node) return;
  node.dataset.state = "armed";
  const observer = new IntersectionObserver(
    ([entry]) => {
      if (!entry?.isIntersecting) return;
      node.dataset.state = "drawn";
      observer.disconnect();
    },
    { threshold: 0.35 },
  );
  observer.observe(node);
  return () => observer.disconnect();
}

function FooterWordmark() {
  return (
    <div ref={watchWordmark} aria-hidden="true" className="wordmark-stage mt-10 overflow-hidden">
      <svg viewBox={`-40 ${-TOP} ${WORDMARK.width + 120} ${TOP - CUT}`} className="block w-full">
        <g transform="scale(1 -1)">
          {WORDMARK.letters.map((letter, index) => (
            <g key={letter.x} transform={`translate(${letter.x} 0)`}>
              <path d={letter.d} className="wordmark-ink" style={{ animationDelay: `${index * STEP + INK_AFTER}s` }} />
              <path d={letter.d} pathLength={1} className="wordmark-trace" style={{ animationDelay: `${index * STEP}s` }} />
            </g>
          ))}
        </g>
      </svg>
    </div>
  );
}

/** The foot of every signed-in page, in the site's style: the thesis in the sans with its lime answer and blinking cursor, the way to the hub and the site, and the wordmark writing itself. */
export function Footer() {
  const t = useT();
  const hub = hubURL(useLocale());
  return (
    <footer>
      <div className="vignette-top border-t border-line bg-ink-raised">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-end justify-between gap-x-12 gap-y-6 px-5 py-10 sm:px-8">
          <div>
            <p className="text-2xl leading-tight font-medium tracking-[-0.01em] text-balance sm:text-3xl">
              {t("footer.thesis")}
              <span className="block text-lime">
                {t("footer.answer")}
                <span aria-hidden="true" className="blink ml-[0.08em] inline-block h-[0.09em] w-[0.42em] rounded-[0.03em] bg-lime align-baseline" />
              </span>
            </p>
            <p className="mt-4 font-mono text-sm text-muted" translate="no">
              <span className="text-muted-light">$</span> versatile --anywhere --anything --your-way
            </p>
          </div>
          <nav aria-label="versatile.sh" className="flex flex-col items-start gap-2 text-sm text-muted pointer-coarse:gap-0">
            <a href={hub} target="_blank" rel="noopener" className={`${QUIET_LINK} pointer-coarse:py-3`}>
              {t("footer.hub")}
              <span className="sr-only"> {t("common.opensNewTab")}</span>
            </a>
            <a href={SITE} target="_blank" rel="noopener noreferrer" className={`${QUIET_LINK} pointer-coarse:py-3`} translate="no">
              versatile.sh
              <span className="sr-only"> {t("common.opensNewTab")}</span>
            </a>
          </nav>
        </div>
      </div>
      <div className="bg-ink-sunken">
        <div className="mx-auto w-full max-w-6xl px-5 pt-10 sm:px-8">
          <FooterWordmark />
        </div>
      </div>
    </footer>
  );
}
