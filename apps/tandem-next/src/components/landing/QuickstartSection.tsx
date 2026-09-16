import Link from "next/link";
import { Eyebrow } from "@/components/ui";
import { TerminalBlock } from "@/components/CopyCommand";
import { MCP_ADD_CMD, SETUP_CMD } from "@/lib/site";

/**
 * QuickstartSection — "Two minutes to your first shared queue." Three copy-paste
 * blocks: the one-line setup, the CLAUDE.md loop, and the config for editors
 * that aren't Claude Code.
 */

// Compact loop — a distilled version of the README's CLAUDE.md block. Keep the
// tool names and the claimed:false semantics exactly in sync with the README.
const CLAUDE_MD_SNIPPET = `## Tandem task queue

Shared work queue for this repo: Tandem canvas \`AB3XK9QZ\`.

1. \`canvas_connect\` with code \`AB3XK9QZ\` (once per session).
2. \`context_get\` for the current briefing. If it comes back \`stale\`,
   say so before acting on it.
3. \`queue_next\` for the next approved task, then \`task_claim\` to take it.
   If the claim returns \`{ claimed: false }\`, another agent won — take
   the next task instead.
4. Do the work. Start commit messages with the ticket id (\`TDM-7: …\`).
5. \`task_complete\` with a result — always include the commit hash.`;

const OTHER_CLIENT_CONFIG = `{
  "mcpServers": {
    "tandem": {
      "command": "npx",
      "args": ["-y", "@jaximus/tandem-mcp"]
    }
  }
}`;

export default function QuickstartSection() {
  return (
    <section className="relative overflow-hidden border-y border-ink/10 bg-surface">
      <div className="relative mx-auto max-w-6xl px-6 py-24">
        <div className="max-w-2xl">
          <Eyebrow>Quickstart</Eyebrow>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight text-ink sm:text-[2rem]">
            Two minutes to your first shared queue.
          </h2>
          <p className="mt-3 leading-relaxed text-ink/65">
            One command starts Tandem on your machine and registers it with Claude Code; one paste
            teaches your sessions the loop. No account, no server to run, no config to write.
          </p>
        </div>

        <div className="mt-12 grid gap-6 lg:grid-cols-2">
          <div className="flex flex-col gap-6">
            <div>
              <p className="mb-2.5 text-xs font-medium text-ink/60">
                <span className="text-accent">1</span> · install and start (Claude Code)
              </p>
              <TerminalBlock label="terminal" copyText={SETUP_CMD}>
                <div className="whitespace-nowrap">
                  <span className="text-indigo-400">$</span> <span className="text-zinc-300">{SETUP_CMD}</span>
                </div>
                <div className="mt-1 whitespace-nowrap text-zinc-500">✓ tandem 0.1.1 running on http://127.0.0.1:7891</div>
                <div className="whitespace-nowrap text-zinc-500">✓ registered with Claude Code (user scope)</div>
                <div className="whitespace-nowrap text-emerald-400/90">✓ board open — click New canvas</div>
              </TerminalBlock>
              <p className="mt-2 text-xs leading-relaxed text-ink/50">
                Downloads this platform&apos;s binary once, keeps its data in{" "}
                <span className="font-code">~/.tandem</span>, and is available in every project. Prefer
                to register by hand?{" "}
                <span className="font-code text-[11px] text-ink/70">{MCP_ADD_CMD}</span>
              </p>
            </div>

            <div>
              <p className="mb-2.5 text-xs font-medium text-ink/60">
                <span className="text-accent">3</span> · Cursor, Codex, or any other MCP client
              </p>
              <TerminalBlock label="mcp.json" copyText={OTHER_CLIENT_CONFIG}>
                <pre className="whitespace-pre font-code text-[11px] leading-[1.7] text-zinc-400">{OTHER_CLIENT_CONFIG}</pre>
              </TerminalBlock>
              <p className="mt-2 text-xs leading-relaxed text-ink/50">
                Same stdio server everywhere; it starts the local Tandem server on first use.{" "}
                <Link href="/mcp" className="text-accent underline underline-offset-2 hover:text-ink">
                  Every client, every flag →
                </Link>
              </p>
            </div>
          </div>

          <div>
            <p className="mb-2.5 text-xs font-medium text-ink/60">
              <span className="text-accent">2</span> · teach your sessions the loop — paste into CLAUDE.md
            </p>
            <TerminalBlock label="CLAUDE.md" copyText={CLAUDE_MD_SNIPPET}>
              <pre className="whitespace-pre font-code text-[11px] leading-[1.7] text-zinc-400">{CLAUDE_MD_SNIPPET}</pre>
            </TerminalBlock>
            <p className="mt-2 text-xs leading-relaxed text-ink/50">
              Or let <span className="font-code">npx @jaximus/tandem-mcp init --write</span> create the
              canvas and append this for you. The full block (epic planning, spec-drift checks) is in
              the README.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
