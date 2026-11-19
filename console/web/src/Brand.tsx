/** The versatile.sh terminal mark: a paper chevron and a lime caret on ink. */
export function Mark({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="365.5 327 510 510" aria-hidden="true">
      <rect x="365.5" y="327" width="510" height="510" rx="96" fill="#0E1116" stroke="rgb(245 245 242 / 0.18)" strokeWidth="8" />
      <path d="M376 432H441L521 688H532L612 432H677L578 732H477Z" fill="#F5F5F2" />
      <rect x="716" y="694.5" width="149" height="37.5" rx="9" fill="#C6F432" />
    </svg>
  );
}

/** The console wordmark: the mark beside `~/versatile.sh`. */
export function Wordmark() {
  return (
    <span className="wordmark">
      <Mark />
      <span>
        <span className="lime">~/</span>versatile.sh
      </span>
    </span>
  );
}
