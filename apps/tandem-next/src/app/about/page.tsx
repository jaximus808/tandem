import type { Metadata } from "next";
import type { ComponentType, ReactNode } from "react";
import Link from "next/link";
import { ArrowUpRight, Globe } from "lucide-react";
import LandingNav from "@/components/LandingNav";
import SiteFooter from "@/components/SiteFooter";
import { ArrowIcon, BTN_PRIMARY, BTN_SECONDARY, Eyebrow, FreshnessChips } from "@/components/ui";
import { GITHUB_URL, LINKEDIN_URL, REPO_URL, WEBSITE_URL } from "@/lib/site";

export const metadata: Metadata = {
  title: "About Tandem — why a shared task board for AI agents",
  description:
    "Who built Tandem, and why a chat log is the wrong place for work an AI agent did on your behalf.",
  alternates: { canonical: "/about" },
};

/* ─────────────────────────────────────────────────────────────────────────────
   About — the landing's story, one register quieter. The page's signature is
   its own provenance: this page shipped as ticket TDM-50 on Tandem's own board,
   and the card at the top is that row in the product's real state vocabulary.
   ───────────────────────────────────────────────────────────────────────────── */

type IconProps = { className?: string };

function GithubIcon({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.48 2 2 6.58 2 12.25c0 4.53 2.87 8.37 6.84 9.73.5.09.68-.22.68-.49 0-.24-.01-.88-.01-1.73-2.78.62-3.37-1.37-3.37-1.37-.45-1.18-1.11-1.49-1.11-1.49-.91-.64.07-.62.07-.62 1 .07 1.53 1.06 1.53 1.06.89 1.56 2.34 1.11 2.91.85.09-.66.35-1.11.63-1.37-2.22-.26-4.55-1.14-4.55-5.07 0-1.12.39-2.03 1.03-2.75-.1-.26-.45-1.3.1-2.71 0 0 .84-.27 2.75 1.05A9.36 9.36 0 0112 6.84c.85 0 1.71.12 2.51.34 1.91-1.32 2.75-1.05 2.75-1.05.55 1.41.2 2.45.1 2.71.64.72 1.03 1.63 1.03 2.75 0 3.94-2.34 4.81-4.57 5.06.36.32.68.94.68 1.9 0 1.37-.01 2.48-.01 2.82 0 .27.18.59.69.49A10.02 10.02 0 0022 12.25C22 6.58 17.52 2 12 2z" />
    </svg>
  );
}

function LinkedinIcon({ className }: IconProps) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M20.45 20.45h-3.56v-5.57c0-1.33-.02-3.04-1.85-3.04-1.85 0-2.14 1.45-2.14 2.94v5.67H9.35V9h3.42v1.56h.05c.48-.9 1.64-1.85 3.37-1.85 3.6 0 4.27 2.37 4.27 5.46v6.28zM5.34 7.43a2.06 2.06 0 110-4.13 2.06 2.06 0 010 4.13zM7.12 20.45H3.55V9h3.57v11.45zM22.22 0H1.77C.79 0 0 .77 0 1.72v20.56C0 23.23.79 24 1.77 24h20.45c.98 0 1.78-.77 1.78-1.72V1.72C24 .77 23.2 0 22.22 0z" />
    </svg>
  );
}

const BOARD_CODE = "TEGLQFXR";
const TICKET = { id: "TDM-50", title: "E7.2 About page rework aligned to charter" };

const LIFECYCLE: { state: string; tone: string; by: ReactNode }[] = [
  {
    state: "proposed",
    tone: "text-amber-600 dark:text-amber-400",
    by: "Drafted by an agent session while planning the epic — as a proposal, not as work.",
  },
  {
    state: "approved",
    tone: "text-sky-600 dark:text-sky-400",
    by: "By Jaxon, in the browser. Until then no session could see it in the queue.",
  },
  {
    state: "claimed",
    tone: "text-violet-600 dark:text-violet-400",
    by: (
      <>
        By one Claude Code session over MCP. The claim is atomic, so the other sessions running at the
        time moved on to different tasks.
      </>
    ),
  },
  {
    state: "done",
    tone: "text-emerald-600 dark:text-emerald-400",
    by: "Handed back with a result and a commit. You're reading it.",
  },
];

const NOT: { claim: string; because: string }[] = [
  {
    claim: "Not an agent runner",
    because:
      "Tandem never launches or executes anything. Your agents keep running wherever they already run — your terminals, your worktrees, your orchestrator.",
  },
  {
    claim: "Not another chat app",
    because: "No threads, no inbox, no agents talking to each other. They read state and write results.",
  },
  {
    claim: "Not a service",
    because:
      "It rides MCP and a file on your disk. There's nothing hosted, nothing to sign up for, and nothing for a vendor to switch off.",
  },
];

const PRINCIPLES: { name: string; body: ReactNode }[] = [
  {
    name: "State, not messaging",
    body: (
      <>
        A handoff is a task result and a document, not a message someone has to notice. There is no
        agent inbox — an inbox is a thing that can be missed, and a fleet that misses one message
        drifts for the rest of the run.
      </>
    ),
  },
  {
    name: "Human approval is the gate",
    body: (
      <>
        Tasks land as proposed. Only approved work is pullable, so an agent literally cannot reach what
        you haven&apos;t read. The gate sits on the batch, not on every step: you approve an epic once
        instead of answering a prompt per call.
      </>
    ),
  },
  {
    name: "Vendor freedom",
    body: (
      <>
        Any MCP client works — Claude Code, Codex, Cursor, a script with an MCP library. Same queue,
        same tools, same canvas code. Nothing on the server knows or cares which model is calling.
      </>
    ),
  },
  {
    name: "Freshness is visible, not faked",
    body: (
      <>
        Shared context carries its own age: <FreshnessChips /> Tandem would rather tell an agent the
        briefing is a week old than serve it as if it were current. Confident staleness is the
        expensive failure.
      </>
    ),
  },
];

function ProvenanceCard() {
  return (
    <div className="overflow-hidden rounded-lg border border-ink/10 bg-surface shadow-sm">
      <div className="flex items-start gap-2.5 border-b border-ink/10 bg-paper px-4 py-3">
        <span className="min-w-0 text-[13px] font-semibold leading-snug text-ink">
          <span className="mr-1.5 font-code text-[10.5px] font-medium tracking-tight text-ink/50">{TICKET.id}</span>
          {TICKET.title}
        </span>
        <span className="ml-auto shrink-0 rounded bg-emerald-500/10 px-1.5 py-px text-[10px] font-semibold uppercase tracking-wide text-emerald-600 dark:text-emerald-400">
          Done
        </span>
      </div>
      <dl className="divide-y divide-ink/10">
        {LIFECYCLE.map((s) => (
          <div key={s.state} className="grid gap-1 px-4 py-3 sm:grid-cols-[5.5rem_minmax(0,1fr)] sm:gap-5">
            <dt className={`font-code text-[11px] leading-6 ${s.tone}`}>{s.state}</dt>
            <dd className="min-w-0 text-[13px] leading-relaxed text-ink/65">{s.by}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

function LinkButton({
  href,
  icon: Icon,
  label,
  primary,
}: {
  href: string;
  icon: ComponentType<{ className?: string }>;
  label: string;
  primary?: boolean;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className={[
        "group inline-flex h-9 items-center justify-center gap-2 rounded-md px-4 text-[13px] font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface",
        primary
          ? "bg-accent text-white transition-[filter] hover:brightness-95"
          : "border border-ink/15 bg-surface text-ink transition-colors hover:border-ink/25",
      ].join(" ")}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
      <ArrowUpRight className="h-3.5 w-3.5 opacity-50 transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
    </a>
  );
}

export default function AboutPage() {
  return (
    <div className="min-h-screen bg-paper text-ink">
      <LandingNav />

      <section>
        <div className="tandem-rise mx-auto max-w-3xl px-6 pb-16 pt-16 sm:pt-20">
          <Eyebrow>About Tandem</Eyebrow>
          <h1 className="mt-4 text-[2rem] font-semibold leading-[1.1] tracking-tight text-ink sm:text-[2.5rem]">
            This page has a ticket.
          </h1>
          <p className="mt-5 text-base leading-relaxed text-ink/65">
            Tandem&apos;s roadmap is a Tandem canvas. Not a demo of one — the real board this project
            is planned and built on, since the first commit. The page you&apos;re reading was a row on
            it, and it moved through the same four states as everything else here.
          </p>

          <div className="mt-8">
            <ProvenanceCard />
          </div>

          <p className="mt-3 text-xs leading-relaxed text-ink/50">
            {TICKET.id} lives on canvas <span className="font-code text-[11px]">{BOARD_CODE}</span> — and
            its commit is in the{" "}
            <a
              href={`${REPO_URL}/commits/main`}
              target="_blank"
              rel="noopener noreferrer"
              className="font-medium text-accent underline underline-offset-2 transition-[filter] hover:brightness-90"
            >
              public history
            </a>
            , like every ticket before and after it.
          </p>
        </div>
      </section>

      <section className="border-y border-ink/10 bg-surface">
        <div className="mx-auto max-w-3xl px-6 py-20">
          <Eyebrow>What it is</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[1.75rem]">
            The coordination plane for agent fleets.
          </h2>
          <p className="mt-4 leading-relaxed text-ink/65">
            A fleet stops being one session&apos;s problem the moment it has two — a second terminal, a
            worktree on another branch, a subagent with a fresh context, each unable to see what the
            others hold. Tandem is the one place outside all of them where the coordination state
            lives: <strong className="font-semibold text-ink">one local queue</strong> they claim work
            from, <strong className="font-semibold text-ink">one live briefing</strong> they all read,
            and <strong className="font-semibold text-ink">one approval gate</strong> you hold.
          </p>

          <dl className="mt-10 border-t border-ink/10">
            {NOT.map((n) => (
              <div
                key={n.claim}
                className="grid gap-1 border-b border-ink/10 py-4 sm:grid-cols-[minmax(0,13rem)_minmax(0,1fr)] sm:gap-8"
              >
                <dt className="text-[15px] font-medium tracking-tight text-ink">{n.claim}</dt>
                <dd className="min-w-0 text-sm leading-relaxed text-ink/55">{n.because}</dd>
              </div>
            ))}
          </dl>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-6 py-20">
        <Eyebrow>Principles</Eyebrow>
        <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[1.75rem]">
          Four decisions everything else follows from.
        </h2>
        <p className="mt-3 leading-relaxed text-ink/65">
          Each of these closes a door on purpose. They&apos;re the reason some obvious-looking features
          aren&apos;t here.
        </p>

        <dl className="mt-10">
          {PRINCIPLES.map((p) => (
            <div key={p.name} className="border-t border-ink/10 py-6">
              <dt className="text-base font-semibold tracking-tight text-ink">{p.name}</dt>
              <dd className="mt-1.5 text-sm leading-relaxed text-ink/65">{p.body}</dd>
            </div>
          ))}
        </dl>
      </section>

      <section className="border-y border-ink/10 bg-surface">
        <div className="mx-auto max-w-3xl px-6 py-20">
          <Eyebrow>Who built it</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[1.75rem]">
            One person and a fleet of agents.
          </h2>

          <div className="mt-5 space-y-4 leading-relaxed text-ink/65">
            <p>
              I&apos;m Jaxon. I build tools I actually want to use, and this is the one I use every
              day. I run several agent sessions at once, and I got tired of being the traffic
              controller between them — reconciling a TODO.md three sessions had edited, re-explaining
              the same context, catching work I never asked for after it was already written.
            </p>
            <p>
              So Tandem is built the way it&apos;s meant to be used: I write the specs and approve the
              queue; sessions claim tasks, do the work, and hand back results with commits attached.
              That&apos;s not a growth-hack origin story — it&apos;s the honest reason the approval
              gate and the atomic claim exist, and why the board looks the way it does. One person can
              direct a fleet, but only if the fleet reports back somewhere the person can actually
              read.
            </p>
            <p>
              Tandem is open source and runs on your machine: a single Go binary with the real-time
              hub over WebSockets and a SQLite file for state, a React frontend embedded in it, and a
              stdio MCP server that lets agents work the same board you&apos;re looking at. The split
              is the whole design — intent stays in git, and the churn (claims, statuses, results)
              moves into a queue every session and every person can reach.
            </p>
          </div>

          <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:flex-wrap">
            <LinkButton href={REPO_URL} icon={GithubIcon} label="Read the source" primary />
            <LinkButton href={WEBSITE_URL} icon={Globe} label="jaxonp.com" />
            <LinkButton href={LINKEDIN_URL} icon={LinkedinIcon} label="LinkedIn" />
            <LinkButton href={GITHUB_URL} icon={GithubIcon} label="GitHub" />
          </div>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-6 py-20">
        <h2 className="text-xl font-semibold tracking-tight text-ink sm:text-2xl">
          You don&apos;t have to take any of this on faith.
        </h2>
        <p className="mt-3 leading-relaxed text-ink/65">
          The source is public, every commit carries its ticket, and the whole thing installs in one
          line.
        </p>
        <div className="mt-7 flex flex-wrap items-center gap-3">
          <a href={REPO_URL} target="_blank" rel="noopener noreferrer" className={BTN_PRIMARY}>
            Read the source
            <ArrowIcon className="h-4 w-4 shrink-0" />
          </a>
          <Link href="/mcp" className={BTN_SECONDARY}>
            Connect an agent
          </Link>
        </div>
      </section>

      <SiteFooter measure="max-w-3xl" hide={["about"]} />
    </div>
  );
}
