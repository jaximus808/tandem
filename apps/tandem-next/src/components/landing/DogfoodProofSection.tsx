import { ArrowIcon, CommitIcon, Eyebrow } from "@/components/ui";
import { COMMITS_URL } from "@/lib/site";

/**
 * DogfoodProofSection — the honest social proof: Tandem is planned and built
 * through its own task queue, and every commit carries the ticket it closed.
 * The CTA opens the repo's history, where the receipts are public even though
 * the board itself now runs on Jaxon's machine.
 */

const DONE_ROWS: { ticket: string; title: string; commit: string; by: string }[] = [
  { ticket: "TDM-61", title: "Bounded-parallel batch ops", commit: "8f3c21d", by: "claude-2" },
  { ticket: "TDM-57", title: "Dark-mode theme tokens", commit: "b04e7fa", by: "claude-1" },
  { ticket: "TDM-64", title: "Task board filter bar", commit: "4d19c8e", by: "codex-1" },
];

function DoneChip() {
  return (
    <span className="shrink-0 rounded bg-emerald-500/10 px-1.5 py-px text-[10px] font-semibold uppercase tracking-wide text-emerald-600 dark:text-emerald-400">
      Done
    </span>
  );
}

export default function DogfoodProofSection() {
  return (
    <section className="relative overflow-hidden border-y border-ink/10 bg-surface">
      <div className="relative mx-auto max-w-6xl px-6 py-24">
        <div className="grid items-center gap-12 lg:grid-cols-[1.05fr_1fr]">
          <div className="max-w-xl">
            <Eyebrow>
              Dogfooding · canvas <span className="font-code text-[11px] normal-case">TEGLQFXR</span>
            </Eyebrow>
            <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
              Built on its own queue.
            </h2>
            <p className="mt-4 leading-relaxed text-ink/65">
              Tandem is built through its own queue — every feature on this page was proposed,
              approved, claimed and completed as a ticket on Tandem&apos;s own board, by the agent
              sessions that build it. The board lives on the builder&apos;s machine now, but the
              receipts are public: more than two hundred commits carry the ticket that produced them.
            </p>
            <div className="mt-8">
              <a
                href={COMMITS_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex min-h-9 items-center gap-2 rounded-md bg-accent px-5 py-2 text-[13px] font-medium text-white transition-[filter] hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:ring-offset-2 focus-visible:ring-offset-surface"
              >
                Read the commit history — every TDM-n ticket, in order
                <ArrowIcon className="h-4 w-4 shrink-0" />
              </a>
            </div>
            <p className="mt-4 font-code text-xs text-ink/50">git log --oneline | grep TDM-</p>
          </div>

          <div className="min-w-0">
            <div className="overflow-hidden rounded-lg border border-ink/10 bg-surface shadow-sm">
              <div className="flex items-center gap-2.5 border-b border-ink/10 bg-paper px-3.5 py-2">
                <span className="truncate text-[13px] font-medium text-ink">tandem planning</span>
                <span className="rounded border border-ink/10 px-1.5 py-px font-code text-[9.5px] text-ink/45">
                  TEGLQFXR
                </span>
                <span className="ml-auto flex items-center gap-1.5 font-code text-[10px] text-ink/45">
                  <span className="h-2 w-2 rounded-full" style={{ backgroundColor: "#10B981" }} />
                  Done
                </span>
              </div>
              <div className="flex flex-col gap-2 p-3">
                {DONE_ROWS.map((row) => (
                  <div key={row.ticket} className="rounded-lg border border-ink/10 bg-surface p-2.5">
                    <div className="flex items-start justify-between gap-2">
                      <span className="min-w-0 text-[13px] font-semibold leading-snug text-ink/55">
                        <span className="mr-1.5 font-code text-[10px] font-medium tracking-tight text-ink/40">
                          {row.ticket}
                        </span>
                        {row.title}
                      </span>
                      <DoneChip />
                    </div>
                    <div className="mt-1.5 flex items-center gap-1.5">
                      <span className="inline-flex items-center gap-1 rounded-[4px] border border-emerald-600/20 bg-emerald-500/10 px-1.5 py-0.5 font-code text-[10.5px] text-emerald-700 dark:text-emerald-300">
                        <CommitIcon className="shrink-0" />
                        {row.commit}
                      </span>
                      <span className="ml-auto shrink-0 font-code text-[10px] text-ink/35">{row.by}</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <p className="mt-3 text-center text-[11px] text-ink/40">a vignette of the board&apos;s Done column</p>
          </div>
        </div>
      </div>
    </section>
  );
}
