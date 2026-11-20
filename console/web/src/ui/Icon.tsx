const PATHS = {
  arrow: "M5 12h14M13 6l6 6-6 6",
  close: "M6 6l12 12M18 6L6 18",
  plus: "M12 5v14M5 12h14",
  copy: "M9 9h10.5v10.5H9zM15 9V4.5H4.5V15H9",
  check: "M5 12.5l4.5 4.5L19 7.5",
  key: "M14.5 9.5a5 5 0 1 1-3.6 4.8L4 21v-3.5h2.5V15H9l2.4-2.4M15.5 7.5h.01",
  plug: "M9 3v5M15 3v5M6.5 8h11v3.5a5.5 5.5 0 0 1-11 0zM12 17v4",
  shield: "M12 3l7.5 3v5.5c0 4.5-3.200 8-7.500 9.500C7.700 19.500 4.500 16 4.500 11.500V6z",
  hand: "M8 12.5V6.2a1.2 1.2 0 0 1 2.4 0V11M10.4 10.5V4.8a1.2 1.2 0 0 1 2.4 0v5.7M12.8 10.5V5.6a1.2 1.2 0 0 1 2.4 0v6M15.2 11.5V8a1.2 1.2 0 0 1 2.4 0v6.4c0 3.4-2.3 5.8-5.6 5.8-2.3 0-3.9-1-5-2.9L4.6 13.4a1.3 1.3 0 0 1 2.2-1.3L8 14",
  search: "M10.500 17a6.500 6.500 0 1 0 0-13 6.500 6.500 0 0 0 0 13zM15.500 15.500L20 20",
  external: "M14 4.5h5.5V10M19.5 4.5L11 13M17 14v5.5H4.500V7H10",
  trash: "M4.5 7h15M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13",
  link: "M10 14a4 4 0 0 0 5.700.3l3-3a4 4 0 0 0-5.700-5.700l-1.500 1.500M14 10a4 4 0 0 0-5.700-.3l-3 3a4 4 0 0 0 5.700 5.700l1.500-1.500",
  terminal: "M4 5.5h16v13H4zM7.5 10l2.5 2-2.5 2M12.5 14.5h4",
  code: "M9 7l-5 5 5 5M15 7l5 5-5 5",
  chat: "M5 5.5h14a1.5 1.5 0 0 1 1.5 1.5v8A1.5 1.5 0 0 1 19 16.5h-7L7.5 20v-3.5H5A1.5 1.5 0 0 1 3.5 15V7A1.5 1.5 0 0 1 5 5.5z",
  globe: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM3.5 12h17M12 3c2.500 2.700 3.500 5.700 3.500 9s-1 6.300-3.500 9c-2.500-2.700-3.500-5.700-3.500-9s1-6.300 3.500-9z",
  logout: "M14 4.5h5.5v15H14M10 8l-4 4 4 4M6 12h10",
  activity: "M3 12h4l2.5-6.5 5 13L17 12h4",
  expand: "M14 4.5h5.5V10M19.5 4.5l-6 6M10 19.5H4.5V14M4.5 19.5l6-6",
  chevron: "M6.5 9.5l5.5 5.5 5.5-5.5",
} as const;

/** The name of one icon of the console's set. */
export type IconName = keyof typeof PATHS;

/** One icon of the console's set: a 24-unit outline in the text colour, one stroke weight throughout. */
export function Icon({ name, className = "size-5" }: { name: IconName; className?: string }) {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className={`shrink-0 ${className}`} fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
      <path d={PATHS[name]} />
    </svg>
  );
}

/** The small arrow of links that lead somewhere, or back. */
export function Arrow({ back = false }: { back?: boolean }) {
  return (
    <svg aria-hidden="true" viewBox="0 0 16 16" className={`inline-block size-4 shrink-0 ${back ? "rotate-180" : ""}`} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 8h10M9 4l4 4-4 4" />
    </svg>
  );
}
