import Link from "next/link";
import TandemLogo from "./TandemLogo";
import { NPM_URL, REPO_URL, WEBSITE_URL } from "@/lib/site";

/**
 * The footer every marketing page shares. `measure` matches the page's reading
 * width (6xl on the landing, 3xl/4xl on the essays) so the footer's edges line
 * up with the content above it. Pages hide the link to themselves via `hide`.
 */
export default function SiteFooter({
  measure = "max-w-6xl",
  hide = [],
}: {
  measure?: string;
  hide?: ("why" | "about" | "features")[];
}) {
  const link = "inline-flex items-center gap-1.5 transition-colors hover:text-ink";
  return (
    <footer className="border-t border-ink/10 bg-surface">
      <div
        className={`mx-auto flex ${measure} flex-col items-center justify-between gap-3 px-6 py-8 text-xs text-ink/45 sm:flex-row`}
      >
        <div className="flex items-center gap-2">
          <TandemLogo size={18} animate={false} />
          <span>Tandem — you and your agents, in tandem.</span>
        </div>
        <div className="flex flex-wrap items-center justify-center gap-4">
          {!hide.includes("features") && (
            <Link href="/features" className={link}>
              Features
            </Link>
          )}
          {!hide.includes("why") && (
            <Link href="/why-tandem" className={link}>
              Why Tandem
            </Link>
          )}
          {!hide.includes("about") && (
            <Link href="/about" className={link}>
              About
            </Link>
          )}
          <a href={REPO_URL} target="_blank" rel="noopener noreferrer" className={link}>
            GitHub
          </a>
          <a href={NPM_URL} target="_blank" rel="noopener noreferrer" className={link}>
            npm
          </a>
          <p>
            made by{" "}
            <a
              href={WEBSITE_URL}
              target="_blank"
              rel="noopener noreferrer"
              className="underline transition-colors hover:text-ink"
            >
              Jaxon
            </a>
          </p>
        </div>
      </div>
    </footer>
  );
}
