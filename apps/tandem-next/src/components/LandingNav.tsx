"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import TandemLogo from "./TandemLogo";
import ThemeToggle from "./ThemeToggle";
import { REPO_URL } from "@/lib/site";

// The marketing top nav shared by every page. Solid paper chrome; the hairline
// only appears once the page scrolls under the nav (canon: no translucency or
// blur). Link visibility is tiered by width — same-page anchors are the most
// expendable (lg), Features / Why Tandem next (md), the CTA survives every
// width. whitespace-nowrap everywhere: a label that doesn't fit is hidden by
// its tier, never folded.
export default function LandingNav({ samePageAnchors = false }: { samePageAnchors?: boolean }) {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 4);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const howItWorksHref = samePageAnchors ? "#how-it-works" : "/#how-it-works";
  const quickstartHref = samePageAnchors ? "#quickstart" : "/#quickstart";
  const link =
    "whitespace-nowrap rounded-md px-3 py-1.5 text-ink/55 transition-colors hover:bg-ink/5 hover:text-ink";

  return (
    <header
      className={`sticky top-0 z-40 border-b bg-paper transition-colors duration-150 ${
        scrolled ? "border-ink/10" : "border-transparent"
      }`}
    >
      <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-6">
        <Link
          href="/"
          className="group flex items-center gap-2 text-[16px] font-semibold tracking-tight text-ink"
          title="Tandem — home"
        >
          <TandemLogo size={32} />
          <span>Tandem</span>
          <span className="ml-1 hidden items-center gap-1.5 whitespace-nowrap rounded border border-ink/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-ink/50 lg:inline-flex">
            <span className="inline-flex h-1 w-1 rounded-full bg-agent" />
            runs locally
          </span>
        </Link>
        <nav className="ml-auto flex items-center gap-1 text-sm sm:gap-1.5">
          <a href={howItWorksHref} className={`hidden lg:inline ${link}`}>
            How it works
          </a>
          <a href={quickstartHref} className={`hidden lg:inline ${link}`}>
            Quickstart
          </a>
          <Link href="/features" className={`hidden md:inline ${link}`}>
            Features
          </Link>
          <Link href="/why-tandem" className={`hidden md:inline ${link}`}>
            Why Tandem
          </Link>
          <Link href="/about" className={`hidden lg:inline ${link}`}>
            About
          </Link>
          <a
            href={REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            className={`hidden sm:inline ${link}`}
          >
            GitHub
          </a>
          <Link
            href="/mcp"
            className="whitespace-nowrap rounded-md px-3 py-1.5 font-medium text-ink/80 transition-colors hover:bg-ink/5"
          >
            Connect an agent
          </Link>
          <ThemeToggle />
        </nav>
      </div>
    </header>
  );
}
