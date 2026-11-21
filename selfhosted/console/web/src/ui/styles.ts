/** On a touch screen, the hit area of a small control grows to at least 44px square around its centre without changing how it looks; the control is `relative`. */
export const TOUCH =
  "pointer-coarse:after:absolute pointer-coarse:after:top-1/2 pointer-coarse:after:left-1/2 pointer-coarse:after:size-full pointer-coarse:after:min-h-11 pointer-coarse:after:min-w-11 pointer-coarse:after:-translate-x-1/2 pointer-coarse:after:-translate-y-1/2";

/** The touch hit area of one segment of a group: 44px tall, as wide as the segment so neighbours never overlap; the segment is `relative`. */
export const TOUCH_ROW = "pointer-coarse:after:absolute pointer-coarse:after:inset-x-0 pointer-coarse:after:top-1/2 pointer-coarse:after:h-full pointer-coarse:after:min-h-11 pointer-coarse:after:-translate-y-1/2";

/** Press feedback for anything clickable that is drawn as a button. */
export const PRESS = "transition-[background-color,border-color,color,opacity,transform] duration-150 active:scale-[0.96]";

/** The one card: raised ink, a strong hairline and the light falling on its top edge, 12px corners. */
export const SURFACE = "vignette-top rounded-xl border border-line-strong bg-ink-raised";

/** The one large panel of a page that stands on the page's own ink, outside the shell (the import landing): 16px corners, a soft hairline, raised ink. Inside the shell the content surface is the container, so a page never draws a panel in it. */
export const PANEL = "vignette-top rounded-2xl border border-line bg-ink-raised";

/** A sunken box inside a panel or a card: a list of rows, a block of text, the next step. */
export const WELL = "rounded-xl border border-line bg-ink";

const LIT = "relative isolate transition-colors duration-300 ease-(--ease-settle) before:pointer-events-none before:absolute before:inset-0 before:-z-10 before:rounded-[inherit] before:bg-[radial-gradient(ellipse_75%_100%_at_50%_0%,rgb(198_244_50/0.1),transparent_70%)] before:opacity-0 before:transition-opacity before:duration-300 hover:border-lime/30 hover:before:opacity-100 focus-visible:border-lime/30 focus-visible:before:opacity-100";

/** A card of a grid on the content surface, from the site's directory: a raised card whose border warms and whose top edge lights lime on hover or keyboard focus. */
export const LIT_CARD = `${SURFACE} ${LIT}`;

/** A tile inside a card, such as a link to the hub: an ink tile whose border warms and whose top edge lights lime on hover or keyboard focus. */
export const LIT_EDGE = `${WELL} ${LIT}`;

/** The lime hairline that lights the top edge of the box holding the next step; the box is `relative` and clips its overflow. */
export const HAIRLINE = "pointer-events-none absolute inset-x-0 top-0 h-px bg-linear-to-r from-transparent via-lime/70 to-transparent";

/** The text field box: ink fill, a strong hairline that brightens on hover and focus and turns amber when invalid, the lime caret. */
export const FIELD = "rounded-lg border border-line-strong bg-ink text-paper caret-lime transition-colors duration-200 hover:border-paper/30 focus-within:border-paper/50 has-aria-invalid:border-amber-400/70";

/** The field at control height, for the selects, dates and searches of a filter row. */
export const CONTROL = `${FIELD} h-9 min-w-0 px-3 text-sm focus-visible:border-paper/50`;

/** A small choice that stays pressed: a pill that turns to paper at 10% when on (`aria-pressed` or `aria-checked`), never lime. */
export const CHIP = `relative inline-flex shrink-0 items-center justify-center gap-2 rounded-full border border-line-strong whitespace-nowrap text-muted select-none hover:text-paper aria-checked:border-paper/60 aria-checked:bg-paper/10 aria-checked:text-paper aria-pressed:border-paper/60 aria-pressed:bg-paper/10 aria-pressed:text-paper ${TOUCH} ${PRESS}`;

/** A choice drawn as a card, for picking one of a few things that each need a line of explanation: chosen, it turns to paper at 10%, never lime. */
export const OPTION_CARD = `flex min-h-16 flex-col items-start justify-center gap-0.5 rounded-xl border border-line-strong bg-ink px-4 py-2.5 text-left hover:border-paper/40 aria-pressed:border-paper/60 aria-pressed:bg-paper/10 ${PRESS}`;

/** Quiet underlined text link, also worn by buttons that sit among such links. */
export const QUIET_LINK = "cursor-pointer underline decoration-line-strong underline-offset-4 transition-colors hover:text-paper hover:decoration-paper";

/** The vertical rhythm of a page: its header, toolbar, content and sections stand in one column, 1.5rem apart; the header adds 0.5rem under itself. */
export const PAGE = "flex flex-col gap-6";

/** The grid of cards a page lists: as many 18rem-or-wider columns as the width holds, 1.5rem apart, so the cards fill the surface on a wide screen and stack on a phone. */
export const CARD_GRID = "grid grid-cols-[repeat(auto-fill,minmax(min(100%,18rem),1fr))] gap-6";

/** The two-column body of a page whose side column is the shared facts or the secondary lists: the main column takes the rest, the side one is 20rem, 2.5rem apart from `lg`. */
export const SPLIT = "grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem] lg:gap-10";

/** Small mono uppercase label over a page title, as the site's directory eyebrow. */
export const EYEBROW = "font-mono text-[0.6875rem] tracking-[0.14em] text-muted uppercase";

/** Small mono caption for machine facts: paths, counts, states. */
export const CAPTION = "font-mono text-xs text-muted";

/** The heading of a section, a card, a dialog or a form, in the sans: the serif belongs to `Display` alone. */
export const TITLE = "text-lg leading-snug font-medium text-paper";

/** The heading one step under `TITLE`: a subsection, a row group, a step. */
export const SUBTITLE = "text-base leading-snug font-medium text-paper";

/** Running text under a title: muted, at a readable measure. */
export const LEAD = "max-w-2xl text-pretty text-muted";

/** A loading placeholder that shimmers in the shape of a box. */
export const SKELETON = "shimmer rounded-xl";

/** A loading placeholder that shimmers in the shape of a line of text. */
export const SKELETON_LINE = "shimmer rounded-full";

type ButtonOptions = { variant?: "primary" | "secondary" | "quiet" | "danger"; size?: "xs" | "sm" | "md" };

const BUTTON_VARIANT = {
  primary: "bg-paper text-ink hover:bg-white focus-visible:bg-white disabled:opacity-60",
  secondary: "border border-line-strong text-paper hover:border-paper/50 focus-visible:border-paper/60 disabled:opacity-50",
  quiet: "text-muted hover:bg-paper/10 hover:text-paper focus-visible:bg-paper/10 focus-visible:text-paper disabled:opacity-50",
  danger: "border border-amber-400/40 text-amber-400 hover:border-amber-400 focus-visible:border-amber-400 disabled:opacity-50",
} as const;

const BUTTON_SIZE = { xs: "h-8 px-3 text-xs", sm: "h-9 px-4 text-sm", md: "h-11 px-6 text-sm" } as const;

/** Class string of the one button language, for links and real buttons alike: a pill, paper for the main action, outlined, quiet, or amber when it destroys. */
export function button({ variant = "primary", size = "md" }: ButtonOptions = {}): string {
  return `relative inline-flex shrink-0 items-center justify-center gap-2 rounded-full font-medium whitespace-nowrap ${TOUCH} ${PRESS} ${BUTTON_VARIANT[variant]} ${BUTTON_SIZE[size]}`;
}

/** Class string of a button that is only an icon: round, quiet until pointed at. */
export const ICON_BUTTON = `relative grid size-9 shrink-0 place-items-center rounded-full text-muted hover:bg-paper/10 hover:text-paper ${TOUCH} ${PRESS}`;
