# Tandem — Design System v3 ("Tool")

> **v3 (TDM-10, 2026-10)** supersedes v2's indigo accent, pill shapes and soft
> shadows: the v2 app still read as generic AI-built UI (indigo/violet buttons,
> `rounded-full` on 97 elements, a violet "working" state). v3 keeps v2's type,
> neutral ground and token NAMES, and changes four things: primary actions are
> ink, not a hue; the accent is one desaturated blue for links/focus/selection
> only; the radius scale is capped at 6px; depth is hairlines, not shadows.
> Where a v2 rule below conflicts with a v3 rule, v3 wins.

The sections below are the current rules (v2 text kept where v3 did not change it).

Replaces the v1 world (cream paper / Fraunces display serif / terracotta-forward /
dotted grids / hard offset shadows). That world was consistently read as AI-generated
by testers and is **anti-reference**: evidence of what the product is, never how it
looks. v2 is the category canon played straight — the craft bar is Linear / Stripe /
Vercel. Nothing clever that costs clarity.

DIRECTION CONTRACT — THESIS: a coordination tool for people who ship; the surface
must feel engineered, not decorated. It refuses the warm-editorial arrangement the
category's AI-built competitors all share. IDENTITY: identity lives in the content
(terminals, receipts, live board); the skin stays invisible — cool neutral ground,
hairline borders, ink-filled primary actions and one desaturated blue for
links/focus/selection, mono strictly for machine text. The skin makes no
recognizability claim of its own; if a screen is memorable, it should be because of what it shows, not how it is dressed. STORY:
"these people sweat details, my agents' work is safe here" → create a canvas. FIRST
VIEWPORT (landing): headline + subhead left, live terminals+board demo right, one
filled primary button. FORM: canon, user-pinned; no seed roll.

## Tokens (the retheme lever — semantic names KEEP, values change)

Defined as CSS variable channels in `apps/web/src/index.css`, exposed via
`tailwind.config.js`. Every token takes Tailwind alpha (`ink/10`, `accent/40`).
Use tokens; never write a raw Tailwind hue (`violet-500`, `indigo-600`, `sky-*`)
in new markup.

| token | light | dark | used for |
|---|---|---|---|
| `paper` | `#FAFAFA` | `#0A0A0B` | page ground |
| `surface` | `#FFFFFF` | `#141416` | cards, panels, inputs, popovers |
| `ink` | `#18181B` | `#E4E4E7` | text; hairlines via `ink/10`, `ink/15` |
| `primary` | `#18181B` | `#EDEDF0` | primary ACTION fill (inverse of the page) |
| `primary-fg` | `#FAFAFA` | `#0A0A0B` | text/icon on `primary` |
| `accent` | `#3F5F99` | `#7E9AC6` | links, focus rings, `::selection`, active-nav marker |
| `agent` | `#C2571B` | `#E07A54` | agent presence only |
| `state-proposed` | `#A0670E` | `#D1A257` | awaiting a human (ochre) |
| `state-approved` | `#5B6B7F` | `#94A3B8` | ready, unclaimed (slate) |
| `state-executing` | `#2B7385` | `#6FB2C0` | an agent holds it (teal) |
| `state-done` | `#3D7A52` | `#74B88A` | done (green) |
| `state-failed` | `#B23B37` | `#E3807A` | failed / sent back (brick) |
| `state-rejected` | `#71717A` | `#8E8E97` | rejected (zinc) |

- **primary** — the one filled button on a view: `bg-primary text-primary-fg
  hover:bg-primary/90`. Near-black in light, off-white in dark. It is never a hue.
- **accent** — a desaturated blue, and ONLY: link text, focus rings
  (`focus-visible:ring-accent/40`), text selection, the active-nav indicator,
  a selected row's `accent/[0.08]` wash. **Never a button fill, never a chip
  ground, never a large area.** (`bg-accent text-white` buttons are v2 debt; see
  Migration.)
- **agent** — terracotta survives ONLY for presence: chips, cursors, the
  card-landed ring, attribution. Never a ground, heading or CTA.
  DECIDED (TDM-19): agent ACTIVITY is terracotta EVERYWHERE — the live
  cursor/halo, the header "editing {mode}" chip, and the reading chip all draw
  from this token. The per-mode hues (`lib/modeTheme.ts`) are content-level
  semantics only (e.g. docTypes icon tints) and never colour chrome or agent
  presence.
- **State hues** — a CLOSED set of six, one per lifecycle state, muted (no
  neon, no 500-level saturation). Text and dots use the token at full strength
  (`text-state-done`, `bg-state-done`); a chip ground is `/10`
  (`bg-state-done/10`). **Executing is teal, not violet.** No new hues; a
  seventh meaning reuses one of these or goes neutral ink. Every state and
  accent value clears 4.5:1 against both `paper` and `surface` in its theme, so
  they are safe as 11px text.

## Type

- **UI + display everywhere: Inter** (variable, via @fontsource-variable/inter —
  the one allowed new dep). Headings are Inter at tight tracking (-0.02em) and
  600/650 weight — the canon look; no display serif anywhere. `font-display`
  and `font-brand` remap to Inter so old markup degrades correctly; delete
  Fraunces + Hanken Grotesk imports.
- **JetBrains Mono** stays, strictly for machine text: tickets (TDM-n), commit
  hashes, terminal content, canvas codes, keyboard hints. Never for labels,
  eyebrows, or nav. (Tracked-mono eyebrow labels are a named slop tell — retire
  the pattern; section eyebrows become 12px Inter 500 uppercase tracking-wide
  in ink/50, used sparingly.)
- Scale: 12 / 13 / 14 (body) / 16 / 20 / 24 / 32 / 44-56 (landing hero only).
  Line-height 1.5 body, 1.2 headings.
- **Metadata contrast floor**: any text set BELOW 12px (ages, counts, tickets,
  provenance, captions, keyboard hints, micro-labels) renders at **ink/50
  minimum** — small + faint is unreadable, especially on dark paper. Sub-12px
  text may be quiet; it may not be both tiny and ghosted. (De-emphasis at these
  sizes comes from weight/size, not alpha below 50.) Placeholders, disabled
  states, and icon-only controls with hover/focus states are exempt. The 12px
  eyebrow spec above (ink/50) already sits on the floor.
- Size floor for labels: nothing below 10px.

## Radius (v3 — the scale is REPLACED in `tailwind.config.js`)

| class | value | use |
|---|---|---|
| `rounded-none` | 0 | table cells, segmented-control inner edges |
| `rounded-sm` | 3px | chips, badges, kbd hints, ticket refs |
| `rounded` | 4px | buttons, inputs, menu items, small cards |
| `rounded-md` | 6px | cards, panels, popovers, modals — the MAXIMUM |
| `rounded-lg` / `xl` / `2xl` / `3xl` | 6px | legacy aliases; do not write in new code |
| `rounded-full` | circle | **avatars and status dots only** |

Nothing in the UI is rounder than 6px. No pill buttons, no pill chips, no
pill tabs, no pill inputs. Arbitrary radii (`rounded-[10px]`) are banned.

## Surfaces & depth

- **Hairlines are the depth system.** 1px `ink/10` borders (or `ink/15` on
  inputs); a raised element is separated by its border, not by a shadow.
- **Shadows**: none on cards, rows, buttons, chips or panels. The ONE exception
  is floating layers that overlap content (popover, menu, modal, toast): a
  hairline border plus at most `shadow-lg`, and nothing coloured.
- **BAN**: gradients of any kind (backgrounds, text, borders), glows / bloom
  (`box-shadow` with a blur radius used as decoration), coloured shadows, hard
  offset "brutalist" shadows, glassmorphism / `backdrop-blur` panels,
  decorative blurred blobs, dotted/grid pattern backgrounds, emoji in UI
  chrome. A transient highlight is a 1–2px ring (`box-shadow: 0 0 0 2px …`),
  never a blur — see `tandem-card-land` / `tandem-card-bounce` in index.css.
- Density (Operate surfaces): 8px base grid; panel padding 12–16px; table/board
  rows compact. Landing sections: 96–128px vertical rhythm, max-w-6xl.

## Motion

150ms ease-out for hover/focus, 200ms for panels. One pulse allowed: live
"working" indicators. No entrance animations on Operate surfaces; landing keeps
the hero demo timeline + at most a single fade-up per section (no staggered
letter/word animations).

## Components (canon specs)

- **Primary button**: `bg-primary text-primary-fg`, `rounded` (4px), 13px/500,
  32px (app) / 36px (landing) height; hover `bg-primary/90`; focus ring
  `accent/40` 2px offset. One per view.
  Secondary: `bg-surface border border-ink/15 text-ink`, hover `bg-ink/[0.04]`.
  Ghost: `text-ink/70`, hover `bg-ink/[0.05] text-ink`.
  Destructive: secondary shape with `text-state-failed`; a filled
  `bg-state-failed text-white` only inside a confirm dialog.
- **Chips** (state/ticket/agent): 11px, `rounded-sm` (3px), `/10` token ground
  + full-strength token text, no border unless interactive. On dense surfaces
  (board cards, lists) prefer a **dot + word** (`rounded-full` 6px dot in the
  state token, then the label in `ink/70`) over a filled chip. Ticket refs:
  mono 10–11px `ink/50` (the metadata floor), no ground.
- **Cards** (board/task): `bg-surface border border-ink/10 rounded-md`, 10–12px
  padding, no shadow; hover raises the border to `ink/20`, not a shadow.
- **Inputs**: `bg-surface border border-ink/15 rounded`; focus `border-accent/60
  ring-2 ring-accent/20`; 13px; labels 12px/500 `ink/60`.
- **Nav/chrome**: paper ground, hairline dividers; active item = `text-ink` +
  `bg-ink/[0.06]` with an accent indicator (2px bar or accent text), never a
  filled accent pill.
- **Modals/popovers**: `bg-surface border border-ink/10 rounded-md shadow-lg`;
  scrim `bg-ink/30` (light) — no blur.

## Migration (what TDM-10 left for the component sweeps)

TDM-10 changed tokens and the radius scale only; no component file was
touched. The radius override already pulls every `rounded-lg/xl/2xl` down to
6px globally. The sweeps (TDM-11 primitives, TDM-12 board, TDM-13 chrome)
should, in their own files:

1. **`bg-accent text-white` → `bg-primary text-primary-fg`** on every filled
   button (57 `bg-accent`, 29 `hover:bg-accent/90` in v2 markup). Until
   migrated these render as desaturated-blue buttons — acceptable but off-spec,
   and in dark mode white-on-`#7E9AC6` is low contrast, so this is the first
   thing to fix.
2. **Raw state hues → `state-*` tokens.** `lib/stateChips.ts` (TDM-11) is the
   hub: `bg-violet-500/10 text-violet-600 dark:text-violet-400` becomes
   `bg-state-executing/10 text-state-executing` (no `dark:` variant needed —
   the token flips), and likewise amber→proposed, sky→approved,
   emerald→done, rose→failed, zinc→rejected. Then hunt the stragglers
   (`grep -rnE '(violet|indigo|purple)-[0-9]' apps/web/src`): FleetView,
   TaskBoard, AgentPresence, ActivityFeed, FollowControl, RoadmapMode.
3. **`rounded-full` (97 uses)** → keep only on avatars and status dots;
   pill buttons/chips/tabs/inputs become `rounded` or `rounded-sm`.
4. **Arbitrary radii** the scale override cannot reach: `rounded-[10px]` (15),
   `rounded-t-[10px]` (3), `rounded-[12px]`, `rounded-[7px]` → `rounded-md`;
   `rounded-[5px]`, `rounded-[4px]` → `rounded`; `rounded-[3px]`,
   `rounded-[2px]` → `rounded-sm`.
5. **Shadows**: drop `shadow-sm` / `shadow` from cards and buttons (≈21 uses);
   keep `shadow-lg` only on floating layers.
6. Note `rounded-md` was 6px before and is 6px now; `rounded` (4px) and
   `rounded-sm` (now 3px, was 2px) shift by at most 1px — no action needed.

## Dark mode

First-class, same token system via `.dark`. Landing UNLOCKS dark (remove
`theme-light` lock): terminals/board demo already read best on dark; verify both.

## Enforcement for agents

Every restyle task: (1) read this file fully; (2) change values/classes toward it,
never invent new hues/faces/radii; (3) grep your diff for banned patterns before
committing — raw hues (`violet-`, `indigo-`, `purple-`, `fuchsia-`), pills
(`rounded-full` on anything that is not an avatar or dot), arbitrary radii
(`rounded-[`), `bg-gradient`, `shadow-[`, `backdrop-blur`, Fraunces,
`surface-grid-faint`, tracked mono labels; (4) both themes verified; (5)
`pnpm build` green.
