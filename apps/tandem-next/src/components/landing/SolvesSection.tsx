import type { ReactNode } from "react";
import { Eyebrow, FreshnessChips, ToolChip } from "@/components/ui";

/**
 * SolvesSection — five failure modes of a multi-session fleet, each paired with
 * the mechanism that handles it (and the MCP tool that mechanism lives in).
 */

const SOLVES: { problem: string; mechanism: string; body: ReactNode; tools?: string[] }[] = [
  {
    problem: "Two agents do the same task",
    mechanism: "Atomic claims with a TTL",
    body: (
      <>
        One claim per task, decided by the server, not by whoever writes last. The loser is told{" "}
        <span className="font-code text-[13px] text-ink/70">claimed: false</span> in the same call and
        moves to the next task. A claim that goes quiet expires and the task returns to the queue — a
        session that died mid-task can&apos;t hold work hostage.
      </>
    ),
    tools: ["task_claim"],
  },
  {
    problem: "Agents act on stale context",
    mechanism: "One briefing, with its age on it",
    body: (
      <>
        Same goal, same constraints, same decisions for every session — and the briefing says how old
        it is: <FreshnessChips /> so an agent can tell the difference between the current picture and
        last week&apos;s, and ask instead of guessing.
      </>
    ),
    tools: ["context_get"],
  },
  {
    problem: "Agents act without supervision",
    mechanism: "An approval gate on the work",
    body: "Agents propose; only what a human approved is pullable. Approve an epic once and the batch clears — the gate is on what runs, not on every step. Each task keeps who proposed it, who approved it, and which session claimed it.",
  },
  {
    problem: "One vague prompt becomes eleven vague tickets",
    mechanism: "The plan is the thing you review",
    body: (
      <>
        The batch arrives as text before any of it is worked — every ticket with its title, the surface
        it touches and the condition that says it&apos;s done. Amend one there, or send a finished one
        back, and the reason is stored on the task: the next session reads{" "}
        <span className="font-code text-[13px] text-ink/70">why</span> off the ticket instead of
        asking. You find out a ticket was wrong while it&apos;s still a sentence.
      </>
    ),
    tools: ["epic_propose", "task_review"],
  },
  {
    problem: "The tooling picks your vendor",
    mechanism: "Plain MCP, no SDK",
    body: "Claude Code, Codex, Cursor, a script with an MCP client — same queue, same tools, same canvas code. Nothing on the server knows or cares which model is calling.",
    tools: ["canvas_connect"],
  },
];

export default function SolvesSection() {
  return (
    <section className="mx-auto max-w-6xl px-6 py-24">
      <div className="max-w-2xl">
        <Eyebrow>What it solves</Eyebrow>
        <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
          Five failure modes, five mechanisms.
        </h2>
        <p className="mt-3 leading-relaxed text-ink/65">
          Every one of these is a thing the server does, not a promise about behaviour.
        </p>
      </div>

      <dl className="mt-12 max-w-4xl">
        {SOLVES.map((s) => (
          <div
            key={s.problem}
            className="grid gap-2 border-t border-ink/10 py-7 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)] sm:gap-10"
          >
            <dt className="text-[15px] leading-relaxed text-ink/45">{s.problem}</dt>
            <dd className="min-w-0">
              <h3 className="text-base font-semibold tracking-tight text-ink">{s.mechanism}</h3>
              <p className="mt-1.5 text-sm leading-relaxed text-ink/65">{s.body}</p>
              {s.tools && (
                <div className="mt-2.5 flex flex-wrap gap-1.5">
                  {s.tools.map((t) => (
                    <ToolChip key={t} name={t} />
                  ))}
                </div>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
