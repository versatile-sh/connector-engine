/** Card surface shared by panels and tiles: a speech bubble with a hairline border and the site's light falling on its top edge. */
export const SURFACE = "vignette-top bubble border border-line-strong bg-ink-raised";

/** Small mono caption for machine facts: paths, counts, states. */
export const CAPTION = "font-mono text-xs text-muted";

/** Small mono uppercase label over a title, as the site's eyebrows. */
export const EYEBROW = "font-mono text-[0.6875rem] tracking-[0.14em] text-muted uppercase";

/** The site's text field box: ink fill, a paper hairline that brightens on focus and turns amber when invalid, the lime caret. */
export const FIELD = "rounded-lg border border-paper/40 bg-ink text-paper caret-lime transition-colors duration-200 focus-within:border-paper/80 has-aria-invalid:border-amber/70";

/** Hover light of a card, from the site's directory: the top edge glows lime and the border warms. The element needs no `group`. */
export const LIT_EDGE =
  "relative isolate before:pointer-events-none before:absolute before:inset-0 before:-z-10 before:rounded-[inherit] before:bg-[radial-gradient(ellipse_75%_100%_at_50%_0%,rgb(198_244_50/0.1),transparent_70%)] before:opacity-0 before:transition-opacity before:duration-300 hover:border-lime/30 hover:before:opacity-100 focus-visible:border-lime/30 focus-visible:before:opacity-100";

/** The lime hairline that lights the top edge of the panel holding the next step; its parent is `relative`. */
export const HAIRLINE = "pointer-events-none absolute inset-x-0 top-0 h-px bg-linear-to-r from-transparent via-lime/70 to-transparent";

/** On a touch screen, the hit area of a small control grows to at least 44px square around its centre without changing how it looks; the control is `relative`. */
export const TOUCH =
  "pointer-coarse:after:absolute pointer-coarse:after:top-1/2 pointer-coarse:after:left-1/2 pointer-coarse:after:size-full pointer-coarse:after:min-h-11 pointer-coarse:after:min-w-11 pointer-coarse:after:-translate-x-1/2 pointer-coarse:after:-translate-y-1/2";

/** The touch hit area of one segment of a group: 44px tall, as wide as the segment so neighbours never overlap; the segment is `relative`. */
export const TOUCH_ROW = "pointer-coarse:after:absolute pointer-coarse:after:inset-x-0 pointer-coarse:after:top-1/2 pointer-coarse:after:h-full pointer-coarse:after:min-h-11 pointer-coarse:after:-translate-y-1/2";

/** The coloured bar on the left of a rounded card, inset clear of its corners; the card is `relative`. */
export const ACCENT_BAR = "pointer-events-none absolute inset-y-2.5 left-1.5 w-1 rounded-full";

/** Press feedback for anything clickable that is drawn as a button. */
export const PRESS = "transition-[background-color,border-color,color,opacity,transform,filter] duration-150 active:scale-[0.97]";

/** Quiet underlined text link. */
export const QUIET_LINK = "cursor-pointer underline decoration-line-strong underline-offset-4 transition-colors hover:text-paper hover:decoration-paper";

/** The small pill a compact control sits in: a select, a date, a search box. */
export const PILL = "h-8 min-w-0 rounded-full border border-line-strong bg-ink px-3 font-mono text-xs text-paper transition-colors duration-150 hover:border-paper/50 focus-visible:border-paper/80 focus-visible:outline-none";

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
  return `bubble-word relative inline-flex shrink-0 items-center justify-center gap-2 font-semibold whitespace-nowrap ${TOUCH} ${PRESS} ${BUTTON_VARIANT[variant]} ${BUTTON_SIZE[size]}`;
}

/** Class string of a button that is only an icon: round, quiet until pointed at. */
export const ICON_BUTTON = `relative grid size-9 shrink-0 place-items-center rounded-full text-muted hover:bg-paper/10 hover:text-paper ${TOUCH} ${PRESS}`;

/** The icon button on ink or on lime: it takes the colour of the text around it. */
export const ICON_BUTTON_PLAIN = `relative grid size-9 shrink-0 place-items-center rounded-full hover:bg-current/15 ${TOUCH} ${PRESS}`;

/** The site's chip, for a small choice that stays pressed: a pill that brightens to paper at 10% when on (`aria-pressed` or `aria-checked`), never lime. */
export const CHIP = `relative inline-flex shrink-0 items-center justify-center gap-2 rounded-full border border-line-strong whitespace-nowrap text-muted select-none hover:text-paper aria-checked:border-paper/60 aria-checked:bg-paper/10 aria-checked:text-paper aria-pressed:border-paper/60 aria-pressed:bg-paper/10 aria-pressed:text-paper ${TOUCH} ${PRESS}`;
