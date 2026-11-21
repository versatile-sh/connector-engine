# Design System: connector gateway console

The console speaks versatile.sh's own language, the one of the site's `/connectors` directory and connector pages: a dark, calm, editorial workbench. Serif headlines with one lime word, hairlines, lit card edges, generous spacing. It is an operating tool, so brand lives in precise details, not in decoration.

The identity source is the versatile.sh site's `DESIGN.md`, `app/globals.css` and `components/ui.ts`. Token and recipe names here match the site's. live.versatile.sh is a quality bar for polish and motion only, never a visual source: no lime panels, no speech-bubble shapes, no filled lime controls.

All recipes live in `src/ui/styles.ts`; the components in `src/ui/`.

## Colour

| Token | Use |
| --- | --- |
| `ink` | page background; the sidebar's ground |
| `ink-raised` | the shell's content surface, the current-link pill, cards, panels on a page, dialogs |
| `ink-sunken` | footer band, code and copy boxes, a panel's tray on the shell's surface |
| `paper` | text and the primary button |
| `muted`, `muted-light` | secondary text, placeholders, counts |
| `line`, `line-strong` | hairlines and borders |
| `lime` | one word of a headline, the `.sh`, the caret, a live dot, the success check, the hover light of a card |
| `amber-400` | errors, cautions and the destructive button |
| `sky-400`, `red-400` | what a function does: reads, deletes |

**One live colour.** Inside one card or figure only one thing is lime. Lime is a word, a dot, a numeral or a hairline, never a fill. Selection is paper at 10%, never lime. The one exception, from the site: a filter list marks its chosen row with a short lime bar and a lime count. A bar or a chart draws its parts in paper tints, amber and red, never lime. `src/ui/vocabulary.test.ts` fails the build on a lime fill that is not a dot, the filter mark or the caret.

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
- **Surfaces**: `SURFACE` is the one card (strong hairline, light on the top edge, 12px). `PANEL` is the one large panel a page holds its content in (16px, soft hairline). A panel takes its fill from the surface it sits on (`--tray-fill`): raised ink on a page of `ink`, and a sunken `ink-sunken` tray inside the shell's raised content surface. A card stays raised ink with its strong hairline there, as it does in a dialog, so the fields and wells inside it keep their step. `WELL` is the sunken box inside a panel or card: a list of rows, a block of text. `LIT_EDGE` is a grid card inside a panel whose top edge lights lime on hover and focus. `HAIRLINE` lights the top edge of the box that holds the next step.
- **Field** (`Field`, `FIELD`, `CONTROL`): ink fill, strong hairline, paper on focus, amber when invalid; the note under it keeps its height. `CONTROL` is the field at 36px for filter rows, `SearchBox` and `Select`.
- **Choices**: `CHIP` for one pressed choice, `Segmented` for one of a few (the language switch, access off/on/ask, ranges), `Select` for one of many or where a `Segmented` has no room, `Choice` for one of a few that fits its container (a `Segmented` where the container has room, else a `Select` of the options that can be taken), `OPTION_CARD` for one of a few that each need a line, `Switch` for on/off (on is paper), `Checkbox` for picking rows of a list (paper, with a mixed state for a select-all).
- **Tabs** (`Tabs`): a row of text tabs over a hairline that switch the view of one panel. The chosen tab is paper with a 2px paper underline that glides to it on transform alone; the others are muted. The arrows wrap, Home and End go to the ends, and the tab that gets focus is the one shown (automatic activation), so a tab only holds a view that shows at once. Past six tabs the row scrolls sideways with faded edges and a `Select` of every tab sits at its end. A machine name (an access key) is set in mono. `<Tabs label value options onChange panel />` with `options` of `{ value, text, hint?, mono? }`; the panel is `<div id={panel} role="tabpanel" aria-labelledby={tabId(panel, value)}>`.
- **Tabs as paths**: when each tab is a page of its own (Account: Profile, People, Backups), each tab is a path, choosing one moves to it in the history, and a reload opens the same tab. The tabs shown are the ones the session's capabilities open; one tab alone draws no tab bar.
- **Tabs or Segmented**: tabs switch what a region shows, a segmented control picks a value that is saved or sent. A connector's profiles and an app's config formats are tabs: each shows other content and nothing is written by choosing one. A function's access, the language, a range or a link kind are segmented: the choice is the data. When a choice both switches a view and is the data, it is a value, so it is segmented.
- **Select** (`Select`): the one dropdown. The `CONTROL` field shows the chosen option and a chevron; its list opens in raised ink with a strong hairline, 12px corners and a soft shadow, under the field, or over it when it does not fit under. On a phone it rises from the bottom as a full-width sheet. The row under the keyboard or the pointer is paper at 10% and the chosen one carries a check; an option may lead with an icon; past eight options a filter tops the list. Keyboard: the arrows, Home, End and typing a name move; Enter or Space chooses; Escape closes and gives focus back to the field. With the filter shown, typing, Space, Home and End go to the filter, and the arrows and Enter still move and choose. It is controlled as a native select: `<Select label value options onChange disabled? placeholder? className? />` with `options` of `{ value, text, icon? }`, a `value` of `undefined` showing the `placeholder` (a dash unless given, so one `Select` can also be a page's overflow menu of actions), and `onChange` called only with a different value. An "All" choice is an ordinary option the caller adds (Activity maps `""` to no filter). A custom period is two `Select`s, from and to, of the days the console keeps, each limited by the other. The console draws no native select, datalist or date field, whose list or calendar the system paints in its own colours: `src/ui/vocabulary.test.ts` fails the build on one.
- **AccessBar**: the share of a connector's or skill's functions that are on and ask, under its description on the card. A 2px rounded hairline on the `line` track: on is paper at 60%, ask is `amber-400` at 50%, off is the bare track. It is `aria-hidden`; the counts beside it carry the meaning.
- **Tag**: a machine fact as mono text in a tone, with an optional dot. It replaces every status pill.
- **Notice**: the one note box, amber when something needs care, a calm box with the lime check when it went well. `InlineConfirm` is the amber question that replaces a Remove or Revoke button.
- **Sheet**: the one dialog, centred, 16px, rising in; full screen on a phone.
- **EmptyState**: a serif headline with its lime word, a muted line, the way to fill the view.
- **Frame**: pages outside the sections (setup, sign-in, an install link, consent, one held call): the grid and light, the wordmark and language, one column.
- **PageTitle**: the `Display` title, the mono count of what the page lists, the lead. It has no eyebrow: the sidebar and the top strip name the section.

## Several people

The console draws what the session's `can` list allows and never works out a role's rights itself. Multi-user chrome appears only once `session.people` is above one, so a one-user install looks as it did (`src/oneUser.test.ts` holds its pages to a snapshot).

- **Role badge**: the viewer's role as a mono `Tag` under their name in the sidebar's user button, and in the rail in the head of the user menu. The name shows always; the role only once a second user exists.
- **Mine | Everyone**: a small `Segmented` for a viewer with the list's `*.any` capability, in the list's own card (the Access keys list, the Activity filter row) or under the page title (Approvals). It is a value sent to the server, so it is segmented, kept in the address as `?scope=everyone`. With Everyone, the count reads "everyone · 12" and each row carries its owner as a `Tag`; Activity adds a person `Select`.
- **Someone else's held call** reads "waiting for bob" and offers only the answers the server lists in `choices`, with **Cancel this call** when `cancel` is among them.
- **Read-only for Members**: what only managers change shows disabled with "Set by your admins", never hidden when the Member still needs to see it (function access, the shared credential). Import, Remove and the first-run guide are hidden.
- **People rows** keep one visible action: an overflow `Select` for an Owner, the Reset password button for an Admin.

## Layout

A section page is a `PageTitle`, then one `PANEL` (or a few `SURFACE`s for pages of settings), the way the site's directory is, on the shell's content surface. Cards in a panel use container queries (`@container` with `@xl:`/`@4xl:` columns), so the same card works in a panel, a dialog or a narrow column.

A page of settings (Account, Notifications) is a column of `SURFACE` cards, one concern each: an icon, a title, one line of what it does and at most one action beside the title, then its body. A card whose list grows (paired computers, channels, apps) takes the full width, and forms side by side in one card end with their buttons on one line.

When a panel shows one of several views, its `Tabs` are the panel's first row, over all of its columns, and the views swap under them. A connector's page is the model: the profile tabs, then the profile's column of cards on the left and, on the right, the cards every profile shares (the credential and the connector's facts), each saying so in its line. Switching a tab changes the data in the cards and never mounts them again, so a filter, a list's length and the scroll stay; only what belongs to one view (its bar, its pending write, its picked rows) is held per view.

## Layout principles

- **Containers align with containers.** Card edge meets card edge and top meets top, never a card beside a line of text. Put a section's heading, counts and filter inside its card, so two columns start with two cards.
- **One grid per page**, with the same gutters everywhere: `gap-6` between cards, `gap-10` between columns.
- **Proximity and common region** (Gestalt): what belongs together sits in one card; what does not, in another.
- **Hick**: few choices at once; a view shows at most three primary actions, and the rest go into one overflow menu (`Select` with a placeholder). A picker of many shows the few most people take first and the others behind one More card that opens them in place (the app picker: Claude Code, Codex, Cursor, VS Code and Claude Desktop, then More apps).
- **Miller**: chunk long sets (a filter past six rows, counts in the heading, a select-all over what the filter shows).
- **Fitts**: the primary action is large and close to what it acts on.
- **Doherty**: feedback in under 400 ms; a pending control shows it at once.
- **Tesler**: the complexity lives in the code, not on the screen; defaults and one-request batches do the work.

## Shell

Every signed-in page lives in one shell (`src/Shell.tsx`, its parts in `src/shell/`): a sidebar, a top strip and the content surface. The shell mounts once; between pages only the content under `main` swaps (`key` on the section), so the sidebar, its marks and the strip never remount and nothing around the content moves. The window never scrolls: `main#main` does, and `navigate()` and a change of section scroll it back to the top. A page draws no `min-h-dvh` and no scroll of its own, and a sticky offset is measured from the surface's top edge.

- **Sidebar**: 16rem at the left on the page's own `ink`, with no card and no border. The logotype and, on its edge, the collapse handle; then the links in two groups, Gateway (Connectors, Skills, Access, Approvals, Activity) and Settings (Notifications, Account), each under a mono eyebrow; at the foot the signed-in user (round initial, name, role) and the language. Every link has an icon from `ui/Icon`.
- **Current link**: one raised-ink pill (`ink-raised`, strong hairline, light on its top edge) with paper text and a short lime bar on its left edge, a mark like the filter list's chosen row. The pill and its bar are one element that glides between links on transform alone, placed once without motion. The lime bar is the sidebar's one live colour, so the count of held calls on Approvals is a paper mono count on paper at 10%, never lime. Never a lime fill, never a lime focus ring.
- **Rail**: the handle shrinks the sidebar to a 4.5rem rail of icons and back, in 250ms on the glide curve, and the choice is kept in `localStorage` under `console.sidebar` (only the exact value `collapsed` reads as a rail). In the rail the names show as tooltips on hover and on keyboard focus, the group eyebrows become hairlines, the count of held calls sits on its icon, and the language moves into the user menu. Only a wide screen shows the rail; the drawer is always full width.
- **User menu**: the user's mark opens a small raised panel with Account and Sign out, placed as a `Select`'s list is, over the button near the bottom and as a sheet from the bottom on a phone. It is a disclosure, not an application menu: Tab moves through it, Escape closes it and gives focus back to its button, and a press or focus outside closes it. Escape inside it never closes the drawer too.
- **Top strip**: 4rem, over the content only and on the page's ground. It names where the page is (group and section, in mono), and shows the first-run progress. The agent endpoint is not shown here; it lives on Access, where an app is connected. Its first item on a narrow screen is the menu button.
- **Content surface**: `main`, one `ink-raised` surface with 16px corners and a soft hairline, starting under the top strip and 12px from the window's right and bottom edges, holding the notices, the page and the site's footer, scrolling on its own while the sidebar and the strip stay put.
- **Below `lg`**: the sidebar is a drawer, the same element slid in from the left (never a second copy of the links). Open, it is a modal dialog (`role="dialog"`, `aria-modal`), the rest of the shell is `inert`, Tab wraps inside it, focus goes to the current link and, when it closes, back to the menu button. Escape, the scrim, the close button and any link close it, and so does leaving the page or a wide screen. The content surface is full-bleed: no corners, no side hairlines.
- **Keyboard and landmarks**: the skip link comes first and lands on `main`; the links sit in a `nav` labelled Console, the current one with `aria-current="page"`; the handle, the menu button and the user menu are real buttons. Focus is the 1px neutral outline.
- **Logic that tests**: the links, the active one, the saved width, the drawer and its keys are plain functions in `src/shell/shell.ts`, held by `shell.test.ts`.

## Responsiveness

- Every screen works at 360, 390, 768, 1024 and 1440px with no horizontal overflow.
- Layouts stack on small widths instead of forcing narrow columns. Components that live at different widths use container queries, not viewport breakpoints.
- `[overflow-wrap:anywhere]` is for machine strings only (ids, URLs, keys, function names). On prose it lets a flex or grid item shrink below a word and breaks words mid-word; `body` already wraps long words with `break-word`.
- Running text stays under about 65 characters (`max-w-xl` to `max-w-2xl`).

## Motion

Two curves, `--ease-glide` for things that travel and `--ease-settle` for things that arrive, about 250ms on transform and opacity. Things that open slide or rise; nothing snaps. A marker that moves between choices glides to the new one: the `Segmented` mark and the `Tabs` underline, which is placed once without motion and then only travels. A row that comes into a list (a filter, Show all, a new call, a new key) grows from no height and fades in, and one that goes shrinks and fades out in its place before it is removed (`useKept` and `rowMotion` from `src/ui/`; `LongList` does it for its rows); the first draw of a list does not move. Rows grow by height, so a list must key them by a stable id, never by position. A view that changes in place updates its cards without mounting them again; only what is new slides in. Loops run only while loading. Under reduced motion every animation and transition is off.

## Focus

Clicking draws no outline; the keyboard draws a 1px neutral outline 4px out. Lime never draws focus.
