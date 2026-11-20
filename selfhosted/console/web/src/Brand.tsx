import { useT } from "./i18n/I18nProvider";

/** The versatile.sh terminal icon: a window bar with the lime light, a prompt chevron, the paper `v` and the lime cursor on ink. */
export function Mark({ className = "size-7" }: { className?: string }) {
  return (
    <svg className={`shrink-0 [clip-path:inset(0_round_22%)] ${className}`} viewBox="0 0 1024 1024" aria-hidden="true">
      <rect width="1024" height="1024" fill="#0E1116" />
      <rect width="1024" height="190" fill="#1A1F27" />
      <rect y="190" width="1024" height="4" fill="#2A313C" />
      <circle cx="320" cy="96" r="22" fill="#3A424E" />
      <circle cx="392" cy="96" r="22" fill="#3A424E" />
      <circle cx="464" cy="96" r="22" fill="#C6F432" />
      <path d="M158 467L310 541V581L158 655V617L287 561L158 505Z" fill="#3A424E" />
      <path d="M376 432H441L521 688H532L612 432H677L578 732H477Z" fill="#F5F5F2" />
      <rect x="716" y="694.5" width="149" height="37.5" rx="9" fill="#C6F432" />
    </svg>
  );
}

/** The site's wordmark: serif italic `versatile` followed by the mono lime `.sh`. */
export function Logotype({ className = "" }: { className?: string }) {
  return (
    <span className={`inline-flex items-baseline ${className}`} translate="no">
      <span className="font-serif text-[1.75em] leading-none italic">versatile</span>
      <span className="ml-[0.15em] font-mono text-[0.8em] text-lime">.sh</span>
    </span>
  );
}

/** The console's logotype: the terminal icon beside the wordmark, the product named under it in mono, which the header drops on a phone where the page eyebrow names it. On lime it sits on its own ink plate so the lime `.sh` survives. */
export function Wordmark({ onAccent = false, className = "" }: { onAccent?: boolean; className?: string }) {
  const t = useT();
  return (
    <span className={`inline-flex items-center gap-2.5 ${onAccent ? "bubble-word bg-ink py-2 pr-4 pl-2 text-paper" : ""} ${className}`}>
      <Mark className={onAccent ? "size-8" : "size-7"} />
      <span className="flex flex-col">
        <Logotype className="text-[0.9375rem]" />
        <span className={`mt-0.5 font-mono text-[0.6875rem] leading-none ${onAccent ? "text-paper/60" : "text-muted max-sm:hidden"}`}>{t("brand.label")}</span>
      </span>
    </span>
  );
}
