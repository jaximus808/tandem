# tandem-next — tandemcanvas.com

The marketing site for Tandem: landing, `/features`, `/why-tandem`, `/about`,
`/mcp`. A static Next.js export — no server, no API routes, no data fetching.
The product itself runs on each user's machine (see the repo README); this site
only tells people how to install it.

## Deploy on Vercel

Import the `jaximus808/tandam` repo and set:

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

- `src/app/*` — one folder per route, plus `layout.tsx` (fonts, theme boot,
  `<head>` metadata + JSON-LD) and `globals.css` (design tokens + the few
  animations the landing uses).
- `src/components/landing/*` — the landing page sections, ported from
  `apps/web/src/components/landing`. Kept self-contained on purpose: this app
  does not import from `apps/web`.
- `public/demo/*` — the self-hosted demo clips (~27MB). They are served from
  the same origin; no external video host.

Design rules live in `/DESIGN.md` at the repo root and apply here too.
