/** Card surface shared by panels and tiles: a speech bubble with a hairline border. */
export const SURFACE = "bubble border border-line-strong bg-ink-raised";

/** Press feedback for anything clickable that is drawn as a button. */
export const PRESS = "transition-[background-color,border-color,color,opacity,transform,filter] duration-150 active:scale-[0.97]";

/** Quiet underlined text link. */
export const QUIET_LINK = "cursor-pointer underline decoration-line-strong underline-offset-4 transition-colors hover:text-paper hover:decoration-paper";

/** A loading placeholder that shimmers in the shape of what is coming. */
export const SKELETON = "shimmer bubble";

type ButtonOptions = { variant?: "primary" | "accent" | "secondary" | "quiet" | "danger" | "ink" | "onInk"; size?: "xs" | "sm" | "md" | "lg" };

const BUTTON_VARIANT = {
  primary: "bg-paper text-ink hover:opacity-90 focus-visible:opacity-90 disabled:opacity-60",
  accent: "bg-lime text-ink hover:brightness-110 focus-visible:brightness-110 disabled:opacity-60",
  secondary: "border border-line-strong text-paper hover:border-paper/60 focus-visible:border-paper/60 disabled:opacity-60",
  danger: "border border-amber/50 text-amber hover:border-amber focus-visible:border-amber disabled:opacity-60",
  ink: "bg-ink text-paper hover:opacity-85 focus-visible:opacity-85 disabled:opacity-60",
  onInk: "border border-paper/30 text-paper hover:border-paper/70 focus-visible:border-paper/70 disabled:opacity-60",
  quiet: "text-muted hover:bg-paper/8 hover:text-paper focus-visible:bg-paper/8 focus-visible:text-paper disabled:opacity-60",
} as const;

const BUTTON_SIZE = { xs: "h-8 px-3 text-xs", sm: "h-9 px-4 text-sm", md: "h-11 px-6 text-[0.9375rem]", lg: "h-12 px-5 text-base" } as const;

/** Class string of the one button language, for links and real buttons alike: a small bubble with its tail. */
export function button({ variant = "primary", size = "md" }: ButtonOptions = {}): string {
  return `bubble-word inline-flex shrink-0 items-center justify-center gap-2 font-semibold whitespace-nowrap ${PRESS} ${BUTTON_VARIANT[variant]} ${BUTTON_SIZE[size]}`;
}

/** Class string of a button that is only an icon: round, quiet until pointed at. */
export const ICON_BUTTON = `grid size-9 shrink-0 place-items-center rounded-full text-muted hover:bg-paper/10 hover:text-paper ${PRESS}`;

/** The icon button on ink or on lime: it takes the colour of the text around it. */
export const ICON_BUTTON_PLAIN = `grid size-9 shrink-0 place-items-center rounded-full hover:bg-current/15 ${PRESS}`;

/** Class string of a small choice that stays pressed: a bubble that turns lime when on (`aria-pressed` or `aria-checked`). */
export const CHIP = `bubble-word inline-flex h-8 shrink-0 items-center justify-center gap-2 border border-line-strong px-3 text-xs font-semibold whitespace-nowrap text-muted hover:border-paper/50 aria-checked:border-transparent aria-checked:bg-lime aria-checked:text-ink aria-pressed:border-transparent aria-pressed:bg-lime aria-pressed:text-ink ${PRESS}`;
