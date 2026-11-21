const PATHS = {
  close: "M6 6l12 12M18 6L6 18",
  plus: "M12 5v14M5 12h14",
  copy: "M9 9h10.5v10.5H9zM15 9V4.5H4.5V15H9",
  check: "M5 12.5l4.5 4.5L19 7.5",
  key: "M14.5 9.5a5 5 0 1 1-3.6 4.8L4 21v-3.5h2.5V15H9l2.4-2.4M15.5 7.5h.01",
  shield: "M12 3l7.5 3v5.5c0 4.5-3.200 8-7.500 9.500C7.700 19.500 4.500 16 4.500 11.500V6z",
  search: "M10.500 17a6.500 6.500 0 1 0 0-13 6.500 6.500 0 0 0 0 13zM15.500 15.500L20 20",
  external: "M14 4.5h5.5V10M19.5 4.5L11 13M17 14v5.5H4.500V7H10",
  trash: "M4.5 7h15M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13",
  terminal: "M4 5.5h16v13H4zM7.5 10l2.5 2-2.5 2M12.5 14.5h4",
  chat: "M5 5.5h14a1.5 1.5 0 0 1 1.5 1.5v8A1.5 1.5 0 0 1 19 16.5h-7L7.5 20v-3.5H5A1.5 1.5 0 0 1 3.5 15V7A1.5 1.5 0 0 1 5 5.5z",
  logout: "M14 4.5h5.5v15H14M10 8l-4 4 4 4M6 12h10",
  activity: "M3 12h4l2.5-6.5 5 13L17 12h4",
  expand: "M14 4.5h5.5V10M19.5 4.5l-6 6M10 19.5H4.5V14M4.5 19.5l6-6",
  chevron: "M6.5 9.5l5.5 5.5 5.5-5.5",
  back: "M14.5 6.5L9 12l5.5 5.5",
  menu: "M4 7h16M4 12h16M4 17h16",
  plug: "M9 3.5V8M15 3.5V8M6.5 8h11v3.5a5.5 5.5 0 0 1-11 0zM12 17v3.5",
  spark: "M10.5 4l1.7 4.8L17 10.5l-4.8 1.7L10.5 17l-1.7-4.8L4 10.5l4.8-1.7zM18 15v5M15.5 17.5h5",
  bell: "M6.5 16.5V11a5.5 5.5 0 0 1 11 0v5.5l1.5 2H5zM10 21h4",
  user: "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4.5 20a7.5 7.5 0 0 1 15 0",
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
