# tandem-next — tandemcanvas.com

The marketing site for Tandem: the landing page and `/docs`. A static Next.js export — no server,
no API routes, no data fetching. The product itself runs on each user's machine
(see the repo README); this site only says what it is and how to install it.

## Deploy on Vercel

Import the `jaximus808/tandem` repo and set:

| Setting          | Value                       |
| ---------------- | --------------------------- |
| Framework Preset | Next.js                     |
| Root Directory   | `apps/tandem-next`          |
| Install Command  | _(default — pnpm install)_  |
| Build Command    | _(default — `next build`)_  |
| Output Directory | _(default)_                 |

Vercel detects the pnpm workspace from the root lockfile and installs only this
package's dependencies. Nothing here imports another workspace package, so the
root directory build needs nothing outside this folder. Point the domain at the
deployment and you're done — there are no environment variables.

## Local dev

```bash
pnpm install                       # once, at the repo root
pnpm --filter tandem-next dev      # http://localhost:3000
pnpm --filter tandem-next build    # static export into apps/tandem-next/out
```

## Where things are

- `src/app/page.tsx` — the landing page: hero (the live scheduler panel,
  headline, Install + Docs buttons), the install line, how it works. Copy is
  inline; links and the install command come from `src/lib/site.ts`.
- `src/app/docs/page.tsx` — `/docs`: one page with anchored sections
  (install, quick start, how it works, orchestration, approval policies,
  tools, CLI, configuration, troubleshooting). Every fact comes from the repo
  README, `docs/ORCHESTRATION.md`, `apps/mcp-gateway/src/` and
  `apps/api/.env.example`; update it when those change.
- `src/components/SiteNav.tsx`, `SiteFooter.tsx` — the nav and footer both
  pages share. `src/components/docs/` — the docs sidebar (`DocsNav`, a client
  component for the scroll-spy and the mobile `<details>`) and `CodeBlock`.
- `src/app/layout.tsx` — fonts (Inter + JetBrains Mono via `next/font`,
  self-hosted at build; `--font-sans` / `--font-mono`), `<head>` metadata and
  the JSON-LD (WebSite + SoftwareApplication).
- `src/app/globals.css` — all the styling. Plain CSS, no Tailwind: the grey
  ramp, the one blue accent, type, hairlines, the 4-across steps, breakpoints.
- `src/components/hero/` — `SchedulerHero`, the centerpiece: a live view of
  the scheduler (ticket queue + three agent lanes streaming tool calls, with a
  claim race). Owned separately from the page layout.
- `src/components/CopyButton.tsx` — the copy button in the install line.
- `public/tandem-mark.svg` is the nav mark (from `assets/svg/tandem-mark-white.svg`).

The look follows ghostty.org rather than `/DESIGN.md` (the app's design
system): near-black ground, Inter prose, JetBrains Mono only for commands and
tool names, 1px borders, outline buttons with a blue primary, dark only.
