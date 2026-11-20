# Design System: connector gateway console

The console speaks versatile.sh's own language, the one of the site's `/connectors` directory and connector pages: a dark, calm, editorial workbench. Serif headlines with one lime word, hairlines, lit card edges, generous spacing. It is an operating tool, so brand lives in precise details, not in decoration.

The identity source is the versatile.sh site's `DESIGN.md`, `app/globals.css` and `components/ui.ts`. Token and recipe names here match the site's. live.versatile.sh is a quality bar for polish and motion only, never a visual source: no lime panels, no speech-bubble shapes, no filled lime controls.

All recipes live in `src/ui/styles.ts`; the components in `src/ui/`.

## Colour

| Token | Use |
| --- | --- |
| `ink` | page background |
| `ink-raised` | cards, panels, dialogs |
| `ink-sunken` | footer band, code and copy boxes |
| `paper` | text and the primary button |
| `muted`, `muted-light` | secondary text, placeholders, counts |
| `line`, `line-strong` | hairlines and borders |
| `lime` | one word of a headline, the `.sh`, the caret, a live dot, the success check, the hover light of a card |
| `amber-400` | errors, cautions and the destructive button |
| `sky-400`, `red-400` | what a function does: reads, deletes |

**One live colour.** Inside one card or figure only one thing is lime. Lime is a word, a dot, a numeral or a hairline, never a fill. Selection is paper at 10%, never lime. The one exception, from the site: a filter list marks its chosen row with a short lime bar and a lime count.

## Typography

| Role | How | Where |
| --- | --- | --- |
| Display | `Display` (Instrument Serif, lit) | the one page title (`h1`) of a screen with its lime word, the setup and sign-in headline, a large empty-state headline |
| Title | `TITLE` (Geist 500, 18px) | section, card, dialog and form headings |
| Subtitle | `SUBTITLE` (Geist 500, 16px) | subsections, steps, row groups |
| Body | Geist 400, muted for running text, `max-w-2xl` | everything else |
| Caption, eyebrow | `CAPTION`, `EYEBROW` (JetBrains Mono) | machine facts: names, paths, counts, states |

**The serif rule.** `Display` is the only place that sets `font-serif`; the logotype in `Brand.tsx` is the other. Dialog titles, card titles, list items, numbers, labels and buttons are Geist. `src/ui/vocabulary.test.ts` fails the build when the serif, a speech-bubble class or an unknown font token appears anywhere else.

## Components

- **Button** (`button({ variant, size })`): a pill. `primary` paper on ink for the main action of a view, `secondary` outlined, `quiet` text only, `danger` amber outlined for removal. Sizes `xs` (in rows), `sm`, `md`. Every button presses in.
- **Surfaces**: `SURFACE` is the one card (raised, strong hairline, light on the top edge, 12px). `PANEL` is the one large panel a page holds its content in (16px, soft hairline). `WELL` is the sunken box inside a panel or card: a list of rows, a block of text. `LIT_EDGE` is a grid card inside a panel whose top edge lights lime on hover and focus. `HAIRLINE` lights the top edge of the box that holds the next step.
- **Field** (`Field`, `FIELD`, `CONTROL`): ink fill, strong hairline, paper on focus, amber when invalid; the note under it keeps its height. `CONTROL` is the field at 36px for filter rows, `SearchBox` and `Select`.
- **Choices**: `CHIP` for one pressed choice, `Segmented` for one of a few (the language switch, access off/on/ask, ranges), `Select` for one of many or where a `Segmented` has no room, `Choice` for one of a few that fits its container (a `Segmented` where the container has room, else a `Select` of the options that can be taken), `OPTION_CARD` for one of a few that each need a line, `Switch` for on/off (on is paper), `Checkbox` for picking rows of a list (paper, with a mixed state for a select-all).
- **Select** (`Select`): the one dropdown. The `CONTROL` field shows the chosen option and a chevron; its list opens in raised ink with a strong hairline, 12px corners and a soft shadow, under the field, or over it when it does not fit under. On a phone it rises from the bottom as a full-width sheet. The row under the keyboard or the pointer is paper at 10% and the chosen one carries a check; an option may lead with an icon; past eight options a filter tops the list. Keyboard: the arrows, Home, End and typing a name move; Enter or Space chooses; Escape closes and gives focus back to the field. With the filter shown, typing, Space, Home and End go to the filter, and the arrows and Enter still move and choose. It is controlled as a native select: `<Select label value options onChange disabled? placeholder? className? />` with `options` of `{ value, text, icon? }`, a `value` of `undefined` showing the `placeholder` (a dash unless given, so one `Select` can also be a page's overflow menu of actions), and `onChange` called only with a different value. An "All" choice is an ordinary option the caller adds (Activity maps `""` to no filter). The console draws no native select or datalist, whose list the system paints in its own colours: `src/ui/vocabulary.test.ts` fails the build on one.
- **Tag**: a machine fact as mono text in a tone, with an optional dot. It replaces every status pill.
- **Notice**: the one note box, amber when something needs care, a calm box with the lime check when it went well. `InlineConfirm` is the amber question that replaces a Remove or Revoke button.
- **Sheet**: the one dialog, centred, 16px, rising in; full screen on a phone.
- **EmptyState**: a serif headline with its lime word, a muted line, the way to fill the view.
- **Frame**: pages outside the sections (setup, sign-in, an install link, consent, one held call): the grid and light, the wordmark and language, one column.
- **PageTitle**: mono eyebrow, the `Display` title, the mono count of what the page lists, the lead.

## Layout

A section page is a `PageTitle`, then one `PANEL` (or a few `SURFACE`s for pages of settings), the way the site's directory is. Cards in a panel use container queries (`@container` with `@xl:`/`@4xl:` columns), so the same card works in a panel, a dialog or a narrow column.

## Responsiveness

- Every screen works at 360, 390, 768, 1024 and 1440px with no horizontal overflow.
- Layouts stack on small widths instead of forcing narrow columns. Components that live at different widths use container queries, not viewport breakpoints.
- `[overflow-wrap:anywhere]` is for machine strings only (ids, URLs, keys, function names). On prose it lets a flex or grid item shrink below a word and breaks words mid-word; `body` already wraps long words with `break-word`.
- Running text stays under about 65 characters (`max-w-xl` to `max-w-2xl`).

## Motion

Two curves, `--ease-glide` for things that travel and `--ease-settle` for things that arrive, about 250ms on transform and opacity. Things that open slide or rise; nothing snaps. Loops run only while loading. Under reduced motion every animation and transition is off.

## Focus

Clicking draws no outline; the keyboard draws a 1px neutral outline 4px out. Lime never draws focus.
