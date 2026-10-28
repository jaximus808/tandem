/** @type {import('tailwindcss').Config} */

// A CSS-variable colour channel, so `<alpha-value>` keeps `ink/10`, `accent/40`
// etc. working: `border-ink/10` resolves to `rgb(var(--color-ink) / 0.1)`.
const v = (name) => `rgb(var(--color-${name}) / <alpha-value>)`;

module.exports = {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  // Class strategy: a `.dark` class on <html> drives the theme. The class is set
  // (before first paint) by the boot script in index.html + lib/theme.ts, which
  // default to the OS `prefers-color-scheme` and honor a saved override.
  darkMode: "class",
  theme: {
    // Radius scale — REPLACED, not extended (TDM-10, DESIGN.md "Radius"). Capped
    // at 6px: a dense tool, not a pill-shaped consumer app. Every stock step
    // above `md` collapses onto 6px so legacy `rounded-lg/xl/2xl/3xl` markup
    // renders square-ish today, before the component sweeps reach it. `full`
    // stays a true circle — reserved for avatars and status dots ONLY; pills
    // that use it are migration debt (see DESIGN.md).
    borderRadius: {
      none: "0px",
      sm: "3px",
      DEFAULT: "4px",
      md: "6px",
      lg: "6px",
      xl: "6px",
      "2xl": "6px",
      "3xl": "6px",
      full: "9999px",
    },
    extend: {
      fontFamily: {
        // Inter (variable) is the ONE UI + display face. Headings are Inter at
        // tight tracking and 600 weight — no display serif.
        sans: ['"Inter Variable"', "Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        // JetBrains Mono — strictly machine text: tickets (TDM-n), commit
        // hashes, terminal content, canvas codes, keyboard hints. Never for
        // labels, eyebrows, or nav.
        code: ['"JetBrains Mono"', "ui-monospace", "SFMono-Regular", "monospace"],
      },
      // Semantic tokens are driven by CSS variables (channels in index.css) so
      // `.dark` flips them without touching markup.
      colors: {
        // Page background.
        paper: v("paper"),
        // Card / panel surface — white in light, one step above paper in dark.
        surface: v("surface"),
        // Near-black text in light; off-white in dark. Borders are ink at low
        // alpha (ink/10, ink/15) so hairlines flip with the theme.
        ink: v("ink"),
        // Primary ACTION fill: near-black in light, off-white in dark — the
        // inverse of the page, never a hue. Pair with `text-primary-fg`.
        //   bg-primary text-primary-fg hover:bg-primary/90
        primary: { DEFAULT: v("primary"), fg: v("primary-fg") },
        // The one interactive hue, a desaturated blue: links, focus rings,
        // selection, the active-nav marker. NOT a button fill (that's primary).
        accent: v("accent"),
        // Agent-presence signature (muted terracotta): presence chips, cursors,
        // attribution — never a ground, heading, or CTA.
        agent: v("agent"),
        // Task-state hues — a CLOSED set, one per lifecycle state. Text/dot at
        // full strength, chip ground at /10:  bg-state-done/10 text-state-done
        state: {
          proposed: v("state-proposed"),
          approved: v("state-approved"),
          executing: v("state-executing"),
          done: v("state-done"),
          failed: v("state-failed"),
          rejected: v("state-rejected"),
        },
      },
    },
  },
  plugins: [require("@tailwindcss/typography")],
};
