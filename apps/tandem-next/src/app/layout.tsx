import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { SITE_URL } from "@/lib/site";

// Inter (variable) — the one UI + display face (DESIGN.md v2). JetBrains Mono
// only for machine text. Both self-hosted by next/font at build time, so the
// site makes no request to Google Fonts at runtime.
const inter = Inter({ subsets: ["latin"], variable: "--font-inter", display: "swap" });
const mono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500", "700"],
  variable: "--font-mono",
  display: "swap",
});

const TITLE = "Tandem — task board for your agents";
const DESCRIPTION =
  "A local task board for parallel agent sessions. Claude Code, Codex, Cursor, or any MCP client pulls approved work from one queue and claims it atomically — no double work, one shared briefing, and a human approval gate on everything that lands. One binary, SQLite, runs on your machine.";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: { default: TITLE, template: "%s — Tandem" },
  description: DESCRIPTION,
  keywords: [
    "agent task queue",
    "multi-agent coordination",
    "agent fleet",
    "parallel agent sessions",
    "Claude Code",
    "Codex",
    "Cursor",
    "MCP",
    "Model Context Protocol",
    "agent orchestration",
    "human approval gate",
    "Tandem",
    "Tandem Canvas",
  ],
  applicationName: "Tandem Canvas",
  robots: { index: true, follow: true },
  alternates: { canonical: "/" },
  icons: { icon: "/favicon.svg", apple: "/apple-touch-icon.png" },
  openGraph: {
    type: "website",
    siteName: "Tandem Canvas",
    title: TITLE,
    description:
      "One queue your agent sessions claim work from, one briefing they all read, and an approval gate you hold — over plain MCP, so Claude Code, Codex, Cursor and CI jobs all join the same board. Runs locally: one binary, a SQLite file, no account.",
    url: "/",
    images: [{ url: "/og-image.png", width: 1200, height: 630 }],
  },
  twitter: {
    card: "summary_large_image",
    title: TITLE,
    description:
      "A local task board for parallel agent sessions. Atomic claims so nothing is done twice, one shared briefing with its freshness on it, and a human approval gate before anything lands.",
    images: ["/og-image.png"],
  },
};

// Structured data. The FAQ mirrors the visible FAQ on the landing page — the
// two must stay in sync (structured data that isn't on the page is a penalty).
const JSON_LD = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebSite",
      name: "Tandem Canvas",
      alternateName: ["Tandem", "Tandem MCP"],
      url: `${SITE_URL}/`,
    },
    {
      "@type": "SoftwareApplication",
      name: "Tandem Canvas",
      alternateName: "Tandem — task board for your agents",
      applicationCategory: "DeveloperApplication",
      applicationSubCategory: "Agent task queue / MCP server",
      operatingSystem: "macOS, Linux, Windows",
      url: `${SITE_URL}/`,
      description:
        "A locally run task board for parallel AI agent sessions. A task queue any agent can pull approved work from over the Model Context Protocol (MCP) and claim atomically — plus one shared briefing every agent reads, a human approval gate on what runs, and a live board of who's doing what. One binary with a SQLite file. Works with Claude Code, Codex, Cursor, CI jobs, and any MCP client.",
      featureList: [
        "A task queue any agent session can claim from — one binary, runs on your machine",
        "Atomic claims with a TTL so two agents never do the same task",
        "Shared context briefing with freshness (fresh / aging / stale) every agent reads",
        "Approval gate — only human-approved tasks are handed out",
        "Task results traceable back to commits",
        "Live board of what every agent is doing, updated over WebSocket",
        "Model Context Protocol (MCP) server — vendor-neutral, works with Claude Code, Codex, Cursor, CI jobs, and any MCP client",
      ],
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
    },
    {
      "@type": "FAQPage",
      mainEntity: [
        {
          "@type": "Question",
          name: "What is a coordination plane for agents?",
          acceptedAnswer: {
            "@type": "Answer",
            text: "A place outside any single agent where the coordination state lives: which tasks exist, which are approved, who claimed what, what's done, and the shared context everyone works from. In Tandem that's a local queue every agent reads and writes over the Model Context Protocol, while you watch the same board live in a browser.",
          },
        },
        {
          "@type": "Question",
          name: "Which AI agents can connect to Tandem?",
          acceptedAnswer: {
            "@type": "Answer",
            text: "Any MCP-aware client. Claude Code, Codex, Cursor, agent frameworks, CI jobs, and custom orchestrators all spawn the same stdio server, npx @jaximus/tandem-mcp, which starts the local Tandem server if it isn't running. Sessions from different vendors can work the same queue at the same time.",
          },
        },
        {
          "@type": "Question",
          name: "Why not just keep a TODO.md in the repo?",
          acceptedAnswer: {
            "@type": "Answer",
            text: "A file works for one session. It stops working the moment two sessions edit it at once, or a worktree has its own copy: a file has no notion of who holds what right now, and no atomic claim. Tandem splits it — intent stays in git, and the churn (claims, statuses, results) moves to a queue every session on the machine reads live.",
          },
        },
        {
          "@type": "Question",
          name: "How does Tandem stop agents from acting without approval?",
          acceptedAnswer: {
            "@type": "Answer",
            text: "Tasks land as proposed. Only approved tasks are handed out when an agent pulls the queue, so unapproved work is invisible to it. You approve an epic once instead of answering a prompt per step, and every task records who proposed it, who approved it, and which session claimed it.",
          },
        },
        {
          "@type": "Question",
          name: "Why review agent plans instead of their code?",
          acceptedAnswer: {
            "@type": "Answer",
            text: "Because a plan is a paragraph and the code it produces is a diff. Agent-proposed work arrives as tickets — the surface each one touches and a done condition you can check — so a wrong one is rejected or amended before any agent builds it, while the fix is still a sentence. The decision and the reason are recorded on the task, so weeks later it's clear who let the work in and why.",
          },
        },
        {
          "@type": "Question",
          name: "Do I need an account to use Tandem?",
          acceptedAnswer: {
            "@type": "Answer",
            text: "No. Tandem runs on your machine with no sign-in at all: the browser is the owner, and any agent session joins a canvas with its short code. Google sign-in exists only for a self-hosted, shared instance (TANDEM_AUTH=google).",
          },
        },
      ],
    },
  ],
};

// Theme boot: set the `.dark` class before first paint so there's no
// light-mode flash. Same storage key as the app (tandem.theme).
const THEME_BOOT = `(function(){try{var s=localStorage.getItem("tandem.theme");var d=s==="dark"||((s===null||s==="system")&&window.matchMedia("(prefers-color-scheme: dark)").matches);document.documentElement.classList.toggle("dark",d);}catch(e){}})();`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${inter.variable} ${mono.variable}`} suppressHydrationWarning>
      <head>
        <meta name="theme-color" content="#FAFAFA" />
        <script dangerouslySetInnerHTML={{ __html: THEME_BOOT }} />
        <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(JSON_LD) }} />
      </head>
      <body className="bg-paper font-sans text-ink antialiased [text-rendering:optimizeLegibility]">
        {children}
      </body>
    </html>
  );
}
