import { useT } from "./i18n/I18nProvider";

/** The versatile.sh terminal mark: a paper chevron and a lime caret on ink. */
export function Mark({ className = "size-7" }: { className?: string }) {
  return (
    <svg className={`shrink-0 ${className}`} viewBox="365.5 327 510 510" aria-hidden="true">
      <rect x="365.5" y="327" width="510" height="510" rx="96" fill="#0E1116" stroke="rgb(245 245 242 / 0.18)" strokeWidth="8" />
      <path d="M376 432H441L521 688H532L612 432H677L578 732H477Z" fill="#F5F5F2" />
      <rect x="716" y="694.5" width="149" height="37.5" rx="9" fill="#C6F432" />
    </svg>
  );
}

/** The console's logotype: the mark beside `~/versatile.sh`, the product named under it. On lime it sits on its own ink plate so the lime `~/` survives. */
export function Wordmark({ onAccent = false, className = "" }: { onAccent?: boolean; className?: string }) {
  const t = useT();
  return (
    <span className={`inline-flex items-center gap-2.5 ${onAccent ? "bubble-word-sent bg-ink py-2 pr-4 pl-2 text-paper" : ""} ${className}`} translate="no">
      <Mark className={onAccent ? "size-8" : "size-7"} />
      <span className="flex flex-col">
        <span className="font-mono text-[0.9375rem] leading-none font-bold tracking-[-0.02em]">
          <span className="text-lime">~/</span>versatile.sh
        </span>
        <span className={`mt-1 font-mono text-[0.6875rem] leading-none ${onAccent ? "text-paper/60" : "text-muted"}`}>{t("brand.label")}</span>
      </span>
    </span>
  );
}
