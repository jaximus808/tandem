import type { Metadata } from "next";
import type { ReactNode } from "react";
import LandingNav from "@/components/LandingNav";
import SiteFooter from "@/components/SiteFooter";
import { Eyebrow } from "@/components/ui";

export const metadata: Metadata = {
  title: "Why agent fleets need a coordination plane",
  description:
    "The fleet outgrew the machine: what breaks when one person runs many AI agents at once, and what a shared task queue has to do about it.",
  alternates: { canonical: "/why-tandem" },
};

/* ─────────────────────────────────────────────────────────────────────────────
   Why Tandem — the research essay ("The fleet outgrew the machine"). Fixed
   copy, transplanted verbatim from the app; one reading measure, one type
   scale, hairlines for every division.
   ───────────────────────────────────────────────────────────────────────────── */

const STATS: { value: string; label: string }[] = [
  { value: "64", label: "Claude agents in parallel" },
  { value: "11 days", label: "start to finished rewrite" },
  { value: "535,496 → 1M+", label: "lines of Zig to lines of Rust" },
  { value: "~$165k", label: "at API list pricing" },
];

function H2({ children }: { children: ReactNode }) {
  return (
    <h2 className="text-2xl font-semibold leading-tight tracking-tight text-ink sm:text-[2rem]">{children}</h2>
  );
}

function Code({ children }: { children: ReactNode }) {
  return <code className="rounded bg-ink/[0.06] px-1 py-0.5 font-code text-[0.85em] text-ink">{children}</code>;
}

function B({ children }: { children: ReactNode }) {
  return <strong className="font-semibold text-ink">{children}</strong>;
}

const P = "text-base leading-relaxed text-ink/65";
const UL = "mt-5 list-disc space-y-3 pl-5 text-[15px] leading-relaxed text-ink/65 marker:text-ink/30";
const OL = "mt-5 list-decimal space-y-4 pl-5 text-[15px] leading-relaxed text-ink/65 marker:font-medium marker:text-ink/40";

export default function WhyTandemPage() {
  return (
    <div className="min-h-screen bg-paper text-ink">
      <LandingNav />

      <header>
        <div className="tandem-rise mx-auto max-w-3xl px-6 pb-14 pt-16 sm:pt-20">
          <Eyebrow>Research</Eyebrow>
          <h1 className="mt-4 text-[2rem] font-semibold leading-[1.1] tracking-tight text-ink sm:text-[2.5rem]">
            The fleet outgrew the machine
          </h1>
          <p className="mt-5 text-base italic leading-relaxed text-ink/65">
            Why agent coordination breaks the moment agents leave one context window — and what
            we&apos;re building about it.
          </p>

          <dl className="mt-10 grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-ink/10 bg-ink/10 sm:grid-cols-4">
            {STATS.map((s) => (
              <div key={s.label} className="bg-paper px-4 py-3.5">
                <dt className="text-lg font-semibold tracking-tight text-ink tabular-nums">{s.value}</dt>
                <dd className="mt-1 text-xs leading-snug text-ink/50">{s.label}</dd>
              </div>
            ))}
          </dl>
        </div>
      </header>

      <section className="mx-auto max-w-3xl px-6 py-16">
        <H2>The largest agent fleet ever documented was held together by a filesystem</H2>

        <div className={`mt-6 space-y-5 ${P}`}>
          <p>
            In May 2026, Jarred Sumner rewrote Bun&apos;s runtime — 535,496 lines of Zig — into more
            than a million lines of Rust in eleven days. He did it with 64 Claude agents running in
            parallel: roughly 50 dynamic workflows looping over 1,448 files, 6,502 commits, a peak
            throughput near 1,300 lines of code per minute, about $165,000 at API list pricing. It
            worked: Claude Code itself now ships running on the Rust rewrite. It is the largest
            publicly documented multi-agent software project to date.
          </p>
          <p>
            Read the writeup closely, though, and a second story emerges. Almost every hard problem
            was a <em>coordination</em> problem, and almost every solution was a filesystem hack:
          </p>
        </div>

        <ul className={UL}>
          <li>
            He couldn&apos;t give each agent its own git worktree — Bun&apos;s repo is too big, and 64
            checkouts would blow the disk. So he compromised: 4 worktree shards, 16 Claudes each.
          </li>
          <li>
            Before the sharding, agents in a shared checkout kept stepping on each other — one ran{" "}
            <Code>git stash</Code> before committing, another ran <Code>git stash pop</Code>, a third
            ran <Code>git reset --hard</Code>. The fix was instructing every agent never to run{" "}
            <Code>git stash</Code> or <Code>git reset</Code>. Coordination by pleading.
          </li>
          <li>The machine ran out of disk space and crashed. Several times.</li>
        </ul>

        <div className={`mt-5 space-y-5 ${P}`}>
          <p>
            Here&apos;s the thing the writeup doesn&apos;t say outright:{" "}
            <B>a coordination plane existed the whole time. Jarred built it by hand, out of the wrong materials.</B>
          </p>
          <p>
            The 4×16 worktree shards are a task allocator, implemented as filesystem geometry. The
            git-command bans are access control, implemented as pleading — probabilistic, per-prompt,
            enforced by nothing. His manual reading of agent output to catch issues is the live status
            board, implemented as one man&apos;s attention. And the bill line-items the cost of doing
            it this way: alongside 5.9 billion uncached input tokens sit{" "}
            <B>72 billion cached-read tokens</B>. Some of that is the mechanical cost of agent loops —
            but it is also what coordination-by-re-reading looks like on an invoice: a fleet with no
            queue to ask <em>what&apos;s claimed, what&apos;s done, what changed while I was working?</em>
          </p>
          <p>
            Every agent fleet already pays for a coordination plane. The only question is whether
            it&apos;s made of disk layout, prompt discipline, and human vigilance — or made of actual
            primitives.
          </p>
        </div>
      </section>

      <section className="border-y border-ink/10 bg-surface">
        <div className="mx-auto max-w-3xl px-6 py-16 sm:py-20">
          <H2>One file is the ceiling</H2>

          <p className={`mt-6 ${P}`}>
            The hand-built version has a hard boundary: it only works while everything shares one
            checkout and one person&apos;s attention. Look at what today&apos;s coordination mechanisms
            actually are:
          </p>

          <ul className={UL}>
            <li>
              <B>The markdown plan file.</B> <Code>PLAN.md</Code>, <Code>TODO.md</Code>, the task list
              your orchestrator maintains — a file on a disk. A session in another worktree reads its
              own copy, can&apos;t lock it, can&apos;t check off an item the others will see.
            </li>
            <li>
              <B>Git worktrees.</B> They isolate agents from each other. Isolation is the easy half —
              integration, and knowing who took what, is what breaks.
            </li>
            <li>
              <B>The orchestrator&apos;s context window.</B> A parent agent that spawns subagents holds
              the plan in its head. That works until the fleet spans processes or days — context
              windows survive neither.
            </li>
            <li>
              <B>The human watching terminals.</B> One screen, one pair of eyes. It doesn&apos;t extend
              to a fleet running while you sleep.
            </li>
          </ul>

          <div className={`mt-5 space-y-5 ${P}`}>
            <p>
              Every one of these primitives has the same hidden dependency: state that lives in one
              place nobody else can query. The moment a second session starts, all of them go soft at
              once. The plan file becomes a rumor. The task list is stale the second it&apos;s copied
              into a prompt. Prompt pleading doesn&apos;t cross a process boundary.
            </p>
            <p>
              And more sessions is exactly where fleets are headed — the Bun rewrite maxed out one
              engineer&apos;s attention, and the next increment of throughput is more parallelism, not
              less.
            </p>
          </div>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-6 py-16">
        <H2>The fleet won&apos;t be one vendor, either</H2>

        <div className={`mt-6 space-y-5 ${P}`}>
          <p>
            The single-session assumption is fragile. The single-vendor assumption is already false.
            Nobody prices a fleet on one model: you spend frontier tokens where judgment is scarce and
            route mechanical transforms to whatever is cheap — an open-weight Qwen or DeepSeek on
            hardware you control, when cost or data locality demands it. And a growing share of the
            fleet was never a coding agent at all: the agent building your frontend, the one drafting
            your launch post, the one filling in your research doc.
          </p>
          <p>
            These agents share no context window, and their vendors&apos; orchestration stories each
            stop at their own walls — not by oversight but by incentive. An orchestrator that drives
            competitors&apos; agents is a commodity pipe, and no lab intends to be the pipe. Each will
            build excellent coordination <em>inside</em> its walls. The layer that spans them can&apos;t
            be a file, and it can&apos;t be owned by a model vendor. It has to be neutral, reachable
            by every session, and speak a protocol every agent already speaks.
          </p>
          <p>That protocol exists: MCP. What&apos;s missing is the plane.</p>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-6 py-16">
        <H2>&ldquo;Isn&apos;t this just an issue tracker?&rdquo;</H2>

        <p className={`mt-6 border-l border-ink/10 pl-5 ${P}`}>
          Fair question — GitHub Issues, Linear, and Notion are persistent, human-legible, and all have
          MCP servers. But they were designed for creatures who check in twice a day, not agents that
          check in twice a second, and it shows in the semantics:
        </p>

        <ul className={UL}>
          <li>
            <B>An assignee field is not an atomic claim.</B> Two agents polling the same tracker race;
            nothing in the contract stops both from starting the same task. A fleet needs claim-or-skip
            as a primitive, not a convention.
          </li>
          <li>
            <B>There is no proposed → approved state machine.</B> In a tracker, an agent filing an idea
            and an agent authorized to burn fifty dollars of tokens on it perform the same write.
            Approval exists only as social convention in the comments. When agents generate the work
            items, the gate has to be structural.
          </li>
          <li>
            <B>Results are comment threads, not completions.</B> An agent&apos;s &ldquo;done&rdquo;
            needs to be a structured record — what was done, where, against what — that a human can
            audit and a downstream agent can consume, surviving long after the session that produced
            it dies.
          </li>
          <li>
            <B>Nothing distinguishes human writes from agent writes.</B> That distinction is the entire
            authorization boundary when your collaborators are machines.
          </li>
        </ul>

        <p className={`mt-5 ${P}`}>
          Human teams built trackers when they stopped sharing an office. Agent fleets are hitting the
          same wall at a thousand times the pace, and they need the machine-native version.
        </p>
      </section>

      <section className="border-y border-ink/10 bg-surface">
        <div className="mx-auto max-w-3xl px-6 py-16 sm:py-20">
          <H2>What the plane requires</H2>

          <p className={`mt-6 ${P}`}>
            Strip the Bun rewrite down to what Jarred was doing by hand, and you get the spec:
          </p>

          <ol className={OL}>
            <li>
              <B>A shared task queue with claim semantics.</B> Work lives in one place. An agent claims
              a task atomically; every other agent sees it&apos;s taken and skips it. Completions post
              structured results that outlive the agent&apos;s context.
            </li>
            <li>
              <B>Approval gates at task granularity.</B> Agents propose; humans promote. The gate sits
              on <em>tasks</em>, not commits — you approve &ldquo;port the WebSocket module,&rdquo; not
              each of forty commits inside it. Structured completions make sampling-based review
              possible; 6,502 raw commits are reviewable by no process at all. Andrew Kelley&apos;s
              verdict on the Bun rewrite — &ldquo;unreviewed slop&rdquo; — is the cost of having no
              place where that trade could even be expressed.
            </li>
            <li>
              <B>A live, human-legible board.</B> Not logs. A glance tells you what&apos;s waiting,
              what&apos;s claimed, what landed, what wants your sign-off — whether the worker is a
              terminal beside you or a subagent you&apos;ll never see the transcript of.
            </li>
            <li>
              <B>Vendor neutrality via MCP.</B> Any agent that speaks the protocol connects. Same queue,
              same board, same rules.
            </li>
            <li>
              <B>Persistence beyond any session.</B> Contexts die, machines reboot, sessions time out.
              The plan is durable state on disk, not resident state in anyone&apos;s RAM.
            </li>
          </ol>

          <p className={`mt-6 ${P}`}>
            One scoping note: Tandem allocates and gates <em>work</em>. Git remains the integration
            plane for <em>code</em> — merging what parallel agents produce is git&apos;s problem, and
            solved there. The coordination plane&apos;s job is allocation — so two agents rarely
            collide on the same work in the first place.
          </p>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-6 py-16">
        <H2>Tandem is that plane</H2>

        <div className={`mt-6 space-y-5 ${P}`}>
          <p>
            Tandem is a task queue and a live board that humans and agents share over MCP. It runs on
            your machine — one binary, one SQLite file — and self-hosts when a fleet outgrows it.
          </p>
          <p>
            An agent connects with one tool call and asks the queue for approved work. Claims are
            atomic — the moment one agent takes a task, every other agent that asks sees it&apos;s
            taken. Two agents can&apos;t port the same file, and no prompt has to beg them not to. When
            the agent finishes — or its context dies trying — it posts a result that outlives the
            session: what was done, where. Work an agent proposes along the way enters as exactly
            that, <em>proposed</em>, and waits behind a human gate until someone promotes it to the
            queue. And you watch all of it on a board rather than a wall of terminals: legible at a
            glance whether the worker is Claude Code in the terminal next to you or a subagent three
            layers down.
          </p>
          <p>
            We hold ourselves to this. Tandem&apos;s roadmap is a Tandem canvas, and the agents that
            build Tandem pull their tasks from it. The queue you&apos;d be adopting is the queue this
            essay was planned in.
          </p>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-6 pb-20 pt-16">
        <H2>The honest version</H2>

        <div className={`mt-6 space-y-5 ${P}`}>
          <p>
            Tandem is early. It&apos;s dogfooded daily by its own builder, and it has not yet
            coordinated a Bun-scale fleet — we&apos;re building the plane before the fleet is fully
            airborne. You also don&apos;t need to believe in cross-vendor swarms to want it today: even
            a three-terminal, one-laptop fleet loses its coordination state every time a context window
            dies, and a queue that remembers is worth having at n=3.
          </p>
          <p>
            The next fleet at Bun&apos;s scale will span more sessions, because that&apos;s where the
            throughput is, and more vendors, because that&apos;s where the specialized agents are. But
            you don&apos;t have to believe in that future to see the problem — the coordination that
            carried the last fleet was one disk, one repo, and one very good engineer reading output
            instead of sleeping, and it was already crashing at 64 agents. Past that line, coordination
            has to live outside any one session, speak a protocol every agent speaks, keep its state
            when contexts die, and keep a human&apos;s hand on the gate.
          </p>
        </div>

        <p className="mt-6 text-lg leading-relaxed tracking-tight text-ink">
          That is what Tandem is: the coordination plane for fleets that span sessions, vendors, and
          days.
        </p>

        <p className="mt-12 border-t border-ink/10 pt-6 text-xs italic leading-relaxed text-ink/50">
          Sources: Jarred Sumner&apos;s Bun-in-Rust writeup and coverage by Simon Willison, The
          Pragmatic Engineer, and The Register (July 2026).
        </p>
      </section>

      <SiteFooter measure="max-w-3xl" hide={["why"]} />
    </div>
  );
}
