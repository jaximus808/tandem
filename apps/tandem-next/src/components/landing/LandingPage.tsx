"use client";

import { useState, type ReactNode } from "react";
import Link from "next/link";
import LandingNav from "@/components/LandingNav";
import SiteFooter from "@/components/SiteFooter";
import TandemLogo from "@/components/TandemLogo";
import { ArrowIcon, BTN_SECONDARY, Eyebrow } from "@/components/ui";
import { CommandLine } from "@/components/CopyCommand";
import HeroBoardDemo from "./HeroBoardDemo";
import VillainSection from "./VillainSection";
import HowItWorksSection from "./HowItWorksSection";
import SeeItInActionSection from "./SeeItInActionSection";
import AudienceSection from "./AudienceSection";
import SolvesSection from "./SolvesSection";
import PlanReviewSection from "./PlanReviewSection";
import DogfoodProofSection from "./DogfoodProofSection";
import ReceiptsSection from "./ReceiptsSection";
import QuickstartSection from "./QuickstartSection";
import FullDemoModal from "./FullDemoModal";
import { HERO_CLIENTS, MCP_SPEC_URL, RELEASES_URL, SETUP_CMD } from "@/lib/site";

/* ─────────────────────────────────────────────────────────────────────────────
   The page is one story, top to bottom — why / how / who / what it solves:

     hero (the claim + HeroBoardDemo: two sessions drain one queue, a claim
       bounces — the product's whole argument, running)
     → WHY (VillainSection: two sessions, one TODO.md)
     → HOW (HowItWorksSection: the loop, step by step, in real tool names)
     → SEE IT (the recorded demo)
     → WHO (AudienceSection: the fleet shapes this is built for)
     → WHAT IT SOLVES (SolvesSection: five failure modes → five mechanisms)
     → THE GATE (PlanReviewSection: triage the plan before the code exists)
     → proof (DogfoodProofSection: Tandem is built on its own queue)
     → receipts (ReceiptsSection: every task traced to a commit)
     → try it (QuickstartSection: copy-paste setup)
     → a modest "the board is a full workspace" note
     → FAQ → closing CTA.
   ───────────────────────────────────────────────────────────────────────────── */

// The board's view modes, name-dropped once in the workspace note below.
const MODE_NAMES = ["Board", "Roadmap", "Docs"];

// KEEP IN SYNC with the FAQPage JSON-LD in app/layout.tsx: structured data that
// doesn't appear on the page is a manual-action risk, so the answers below and
// the ones in the <head> are the same sentences.
const FAQS: { q: string; a: ReactNode }[] = [
  {
    q: "What is a coordination plane for agents?",
    a: (
      <>
        A place outside any single agent where the coordination state lives: which tasks exist, which
        are approved, who claimed what, what&apos;s done, and the shared context everyone works from.
        In Tandem that&apos;s a local queue every agent reads and writes over the{" "}
        <a
          href={MCP_SPEC_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="text-accent underline underline-offset-2 hover:text-ink"
        >
          Model Context Protocol
        </a>
        , while you watch the same board live in a browser.
      </>
    ),
  },
  {
    q: "Which AI agents can connect to Tandem?",
    a: (
      <>
        Any MCP-aware client. Claude Code, Codex, Cursor, agent frameworks, CI jobs, and custom
        orchestrators all spawn the same stdio server,{" "}
        <span className="font-code text-[13px]">npx @jaximus/tandem-mcp</span>, which starts the local
        Tandem server if it isn&apos;t running. Sessions from different vendors can work the same
        queue at the same time.
      </>
    ),
  },
  {
    q: "Why not just keep a TODO.md in the repo?",
    a: (
      <>
        A file works for one session. It stops working the moment two sessions edit it at once, or a
        worktree has its own copy: a file has no notion of who holds what right now, and no atomic
        claim. Tandem splits it — intent stays in git, and the churn (claims, statuses, results) moves
        to a queue every session on the machine reads live.
      </>
    ),
  },
  {
    q: "How does Tandem stop agents from acting without approval?",
    a: (
      <>
        Tasks land as proposed. Only approved tasks are handed out when an agent pulls the queue, so
        unapproved work is invisible to it. You approve an epic once instead of answering a prompt per
        step, and every task records who proposed it, who approved it, and which session claimed it.
      </>
    ),
  },
  {
    q: "Why review agent plans instead of their code?",
    a: (
      <>
        Because a plan is a paragraph and the code it produces is a diff. Agent-proposed work arrives
        as tickets — the surface each one touches and a done condition you can check — so a wrong one
        is rejected or amended before any agent builds it, while the fix is still a sentence. The
        decision and the reason are recorded on the task, so weeks later it&apos;s clear who let the
        work in and why.
      </>
    ),
  },
  {
    q: "Do I need an account to use Tandem?",
    a: (
      <>
        No. Tandem runs on your machine with no sign-in at all: the browser is the owner, and any agent
        session joins a canvas with its short code. Google sign-in exists only for a self-hosted,
        shared instance (<span className="font-code text-[13px]">TANDEM_AUTH=google</span>).
      </>
    ),
  },
];

export default function LandingPage() {
  const [demoOpen, setDemoOpen] = useState(false);

  return (
    <div className="min-h-screen overflow-x-clip scroll-smooth bg-paper text-ink">
      <LandingNav samePageAnchors />

      {/* Hero — copy left, the two-terminals-one-queue demo right. */}
      <section className="relative overflow-hidden">
        <div className="tandem-rise relative mx-auto grid max-w-6xl items-center gap-14 px-6 pb-20 pt-16 lg:grid-cols-[1fr_1.05fr] lg:gap-10 lg:pb-24 lg:pt-20">
          <div className="min-w-0 max-w-xl lg:pt-4">
            <div>
              <Eyebrow>The task board for your agents</Eyebrow>
            </div>

            <div className="mt-5">
              <h1 className="text-[2.75rem] font-semibold leading-[1.08] tracking-tight text-ink sm:text-[3.5rem] sm:leading-[1.06]">
                One queue.
                <br />
                Every agent.
                <br />
                <span className="text-accent">Your approval.</span>
              </h1>
            </div>

            <p className="mt-6 text-base leading-relaxed text-ink/65">
              You run several agent sessions at once — terminals, worktrees, subagents — and they
              coordinate through a TODO.md three of them are editing. Tandem gives them one queue to
              claim work from atomically, one briefing they all read, and one gate you hold: you
              approve the plan before a line of code exists. One binary, a SQLite file, no account.
            </p>

            <div className="mt-8 max-w-md">
              <CommandLine cmd={SETUP_CMD} label="install command" />
              <p className="mt-2 text-[12px] leading-relaxed text-ink/50">
                Starts Tandem on your machine, registers it with Claude Code, opens the board.
                macOS, Linux, Windows.
              </p>
            </div>

            <div className="mt-6 flex flex-wrap items-center gap-3">
              <button onClick={() => setDemoOpen(true)} className={BTN_SECONDARY}>
                Watch the 4-minute demo
              </button>
              <a
                href={RELEASES_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-[13px] font-medium text-accent transition-[filter] hover:brightness-90"
              >
                or download the binary
                <ArrowIcon className="h-3.5 w-3.5" />
              </a>
            </div>

            <p className="mt-6 text-[13px] leading-relaxed text-ink/50">
              Any MCP client: {HERO_CLIENTS.join(", ")}, or your own script. No SDK, no adapter per
              vendor, nothing hosted.
            </p>
          </div>

          <div className="min-w-0">
            <HeroBoardDemo />
          </div>
        </div>
      </section>

      <VillainSection />

      <div id="how-it-works">
        <HowItWorksSection />
      </div>

      <SeeItInActionSection onWatchFull={() => setDemoOpen(true)} />

      <AudienceSection />

      <SolvesSection />

      <PlanReviewSection />

      <DogfoodProofSection />

      <ReceiptsSection />

      <div id="quickstart">
        <QuickstartSection />
      </div>

      {/* The board is a full workspace — one modest mention, not a section of demos. */}
      <section id="modes" className="mx-auto max-w-6xl px-6 py-24">
        <div className="max-w-2xl">
          <Eyebrow>Beyond the queue</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink">The board is a full workspace.</h2>
          <p className="mt-3 leading-relaxed text-ink/65">
            Everything your agents write lands as structured state you can actually read — the epic
            as a roadmap, results on the board, findings as a doc. Same data they see, rendered for
            the human who has to review it.
          </p>
          <div className="mt-5 flex flex-wrap gap-1.5">
            {MODE_NAMES.map((m) => (
              <span key={m} className="rounded border border-ink/10 bg-surface px-2 py-0.5 text-[11px] font-medium text-ink/60">
                {m.toLowerCase()}
              </span>
            ))}
          </div>
        </div>
      </section>

      {/* Where it runs — the local-first facts, plainly. */}
      <section className="border-y border-ink/10 bg-surface">
        <div className="mx-auto max-w-6xl px-6 py-24">
          <div className="max-w-2xl">
            <Eyebrow>Where it runs</Eyebrow>
            <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
              On your machine. Nowhere else.
            </h2>
            <p className="mt-3 leading-relaxed text-ink/65">
              Tandem is a single Go binary with the web app embedded and a SQLite file under{" "}
              <span className="font-code text-[13px] text-ink/70">~/.tandem</span>. Nothing phones home,
              nothing needs an account, and your board is yours to back up with{" "}
              <span className="font-code text-[13px] text-ink/70">cp</span>.
            </p>
          </div>
          <dl className="mt-12 grid gap-px overflow-hidden rounded-lg border border-ink/10 bg-ink/10 sm:grid-cols-3">
            {[
              ["one binary", "Go + embedded web bundle. The npm package downloads it once and starts it for you."],
              ["one file", "SQLite. Every canvas, ticket, claim and audit trail in ~/.tandem/tandem.db."],
              ["one flag to share", "Put it on a box for your team with TANDEM_AUTH=google — sign-in, tokens and sharing come back."],
            ].map(([k, v]) => (
              <div key={k} className="bg-paper px-5 py-5">
                <dt className="font-code text-[13px] font-medium text-ink">{k}</dt>
                <dd className="mt-1.5 text-sm leading-relaxed text-ink/60">{v}</dd>
              </div>
            ))}
          </dl>
        </div>
      </section>

      {/* FAQ — mirrored by the FAQPage JSON-LD in app/layout.tsx. */}
      <section id="faq" className="border-b border-ink/10">
        <div className="mx-auto max-w-3xl px-6 py-24">
          <Eyebrow>Questions</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
            Coordination for agents that don&apos;t share a memory
          </h2>
          <p className="mt-3 leading-relaxed text-ink/65">
            Tandem is a local queue your agents work over MCP while you watch and steer from the
            browser. The short version, in plain terms:
          </p>

          <dl className="mt-12 space-y-9">
            {FAQS.map((f) => (
              <div key={f.q} className="border-t border-ink/10 pt-6">
                <dt className="text-base font-semibold tracking-tight text-ink">{f.q}</dt>
                <dd className="mt-2 leading-relaxed text-ink/65">{f.a}</dd>
              </div>
            ))}
          </dl>
        </div>
      </section>

      {/* Closing CTA */}
      <section className="relative overflow-hidden">
        <div className="relative mx-auto max-w-3xl px-6 py-28 text-center">
          <TandemLogo size={44} />
          <h2 className="mt-8 text-2xl font-semibold leading-tight tracking-tight text-ink sm:text-[2rem]">
            Every agent on the same queue,{" "}
            <em className="not-italic text-accent">every task behind your approval.</em>
          </h2>
          <p className="mx-auto mt-6 max-w-xl leading-relaxed text-ink/65">
            Give your sessions one place to find the work, claim it without collisions, and hand it
            back with receipts — while you stay the one who says go, reviewing the plan instead of
            the diff.
          </p>
          <div className="mx-auto mt-9 max-w-md text-left">
            <CommandLine cmd={SETUP_CMD} label="install command" />
          </div>
          <div className="mt-5 flex flex-wrap items-center justify-center gap-3">
            <Link href="/mcp" className={BTN_SECONDARY}>
              Connect an agent
            </Link>
            <Link href="/features" className="text-[13px] font-medium text-ink/70 underline underline-offset-2 hover:text-ink">
              Everything it does
            </Link>
          </div>
        </div>
      </section>

      <SiteFooter />

      {demoOpen && <FullDemoModal onClose={() => setDemoOpen(false)} />}
    </div>
  );
}
