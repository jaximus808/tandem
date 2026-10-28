import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { SITE_URL } from "@/lib/site";

// Inter carries the prose; JetBrains Mono is for commands, tool names and
// ticket ids only. Both are self-hosted by next/font at build time, so nothing
// is fetched from Google at runtime. The page is dark-only (color-scheme: dark).
const sans = Inter({ subsets: ["latin"], weight: "variable", variable: "--font-sans", display: "swap" });
const mono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500"],
  variable: "--font-mono",
  display: "swap",
});

const TITLE = "Tandem · a task queue for parallel agent sessions";
const DESCRIPTION =
  "One task queue your parallel Claude Code, Codex and Cursor sessions claim work from. Agents propose, you approve once, claims are atomic. Runs locally: one Go binary and one SQLite file in ~/.tandem, no accounts.";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: TITLE,
  description: DESCRIPTION,
  applicationName: "Tandem",
  robots: { index: true, follow: true },
  alternates: { canonical: "/" },
  icons: { icon: "/favicon.svg", apple: "/apple-touch-icon.png" },
  openGraph: {
    type: "website",
    siteName: "Tandem",
    title: TITLE,
    description: DESCRIPTION,
    url: "/",
    images: [{ url: "/og-image.png", width: 1200, height: 630 }],
  },
  twitter: {
    card: "summary_large_image",
    title: TITLE,
    description: DESCRIPTION,
    images: ["/og-image.png"],
  },
};

// Structured data: only what the page itself says.
const JSON_LD = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebSite",
      name: "Tandem",
      alternateName: ["Tandem Canvas", "Tandem MCP"],
      url: `${SITE_URL}/`,
    },
    {
      "@type": "SoftwareApplication",
      name: "Tandem",
      applicationCategory: "DeveloperApplication",
      applicationSubCategory: "Agent task queue / MCP server",
      operatingSystem: "macOS, Linux, Windows",
      url: `${SITE_URL}/`,
      description: DESCRIPTION,
      featureList: [
        "One task queue every agent session on the machine claims from",
        "Atomic claims: when two sessions go for the same task exactly one wins",
        "Agents propose an epic; one human approval releases its tasks",
        "Expiring claims, so a dead session does not hold a task forever",
        "Every finished task carries a result and a commit link",
        "Model Context Protocol server: Claude Code, Codex, Cursor, or any stdio MCP client",
        "Runs locally: one Go binary and one SQLite file, no accounts",
      ],
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
    },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <head>
        <meta name="theme-color" content="#0f0f11" />
        <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(JSON_LD) }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
