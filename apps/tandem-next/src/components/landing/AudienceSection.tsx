import { Eyebrow } from "@/components/ui";

/**
 * AudienceSection — the WHO beat, told as a fleet manifest rather than feature
 * cards: four places work actually happens, and the column that matters (can it
 * see the rest of the fleet?) reading "no" all the way down.
 */

const FLEET: { where: string; client: string; lifetime: string; note: string }[] = [
  {
    where: "terminal 1",
    client: "claude code",
    lifetime: "as long as the tab",
    note: "sees its own chat history, and nothing else",
  },
  {
    where: "two more worktrees",
    client: "claude code",
    lifetime: "hours",
    note: "same disk, different branch, no idea who took what",
  },
  {
    where: "subagents, fanned out",
    client: "orchestrator",
    lifetime: "one task each",
    note: "fresh context, no memory of the sibling that ran first",
  },
  {
    where: "a CI runner",
    client: "script + MCP client",
    lifetime: "one job",
    note: "runs while you sleep, merges nothing on its own",
  },
];

export default function AudienceSection() {
  return (
    <section className="relative overflow-hidden border-y border-ink/10 bg-surface">
      <div className="relative mx-auto max-w-6xl px-6 py-24">
        <div className="max-w-2xl">
          <Eyebrow>Who it&apos;s for</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
            If you&apos;re running more than one agent, you&apos;re running a fleet.
          </h2>
          <p className="mt-3 leading-relaxed text-ink/65">
            One person, several agents, no shared memory between them. A typical afternoon looks
            something like this:
          </p>
        </div>

        <div className="mt-12 overflow-x-auto">
          <table className="w-full min-w-[44rem] border-collapse text-left">
            <thead>
              <tr className="border-b border-ink/10">
                <th className="py-2 pr-6 text-xs font-medium uppercase tracking-wide text-ink/50">Where it runs</th>
                <th className="py-2 pr-6 text-xs font-medium uppercase tracking-wide text-ink/50">Client</th>
                <th className="py-2 pr-6 text-xs font-medium uppercase tracking-wide text-ink/50">Lives for</th>
                <th className="py-2 text-xs font-medium uppercase tracking-wide text-ink/50">
                  Can it see the rest of the fleet?
                </th>
              </tr>
            </thead>
            <tbody>
              {FLEET.map((row) => (
                <tr key={row.where} className="border-b border-ink/10 align-top">
                  <td className="py-4 pr-6 font-code text-[12.5px] text-ink">{row.where}</td>
                  <td className="py-4 pr-6 font-code text-[12.5px] text-ink/60">{row.client}</td>
                  <td className="py-4 pr-6 font-code text-[12.5px] text-ink/60">{row.lifetime}</td>
                  <td className="py-4 text-sm leading-relaxed text-ink/65">
                    <span className="mr-2 font-code text-[12.5px] font-medium text-ink/45">no</span>
                    {row.note}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <p className="mt-8 max-w-2xl text-base leading-relaxed text-ink">
          Add a queue every one of them can reach and the last column reads{" "}
          <span className="font-code text-[15px] font-medium text-accent">yes</span> for every row —
          including for you, watching the board in a browser tab.
        </p>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink/50">
          Running one agent at a time in one checkout? You don&apos;t need this — a file in the repo is
          genuinely fine. Tandem starts paying for itself at the second session. The CI runner needs
          a self-hosted instance it can reach; everything else is your laptop.
        </p>
      </div>
    </section>
  );
}
