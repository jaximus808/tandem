/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.{js,ts,jsx,tsx}"],
  // Same theme system as the app (apps/web): a `.dark` class on <html>, set
  // before first paint by the boot script in app/layout.tsx.
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        // Inter (variable) is the ONE UI + display face (DESIGN.md v2) —
        // loaded through next/font in app/layout.tsx and exposed as a variable.
        sans: ["var(--font-inter)", "Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        // JetBrains Mono — strictly machine text: tickets, hashes, terminal
        // content, canvas codes, keyboard hints.
        code: ["var(--font-mono)", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      // Semantic tokens as RGB channels (see globals.css) so `.dark` flips them
      // and `ink/10`-style alpha utilities keep working.
      colors: {
        paper: "rgb(var(--color-paper) / <alpha-value>)",
        surface: "rgb(var(--color-surface) / <alpha-value>)",
        ink: "rgb(var(--color-ink) / <alpha-value>)",
        accent: "rgb(var(--color-accent) / <alpha-value>)",
        agent: "rgb(var(--color-agent) / <alpha-value>)",
      },
    },
  },
  plugins: [],
};
