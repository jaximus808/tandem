import type { Metadata } from "next";
import Link from "next/link";
import LandingNav from "@/components/LandingNav";
import SiteFooter from "@/components/SiteFooter";
import { B, Eyebrow, M } from "@/components/ui";
import { CommandLine } from "@/components/CopyCommand";
import ClientTabs, { CodeBlock, type Tab } from "@/components/mcp/ClientTabs";
import { MCP_ADD_CMD, MCP_SPEC_URL, NPM_URL, RELEASES_URL, REPO_URL, SETUP_CMD, UP_CMD } from "@/lib/site";

export const metadata: Metadata = {
  title: "Connect any MCP client to a shared task board",
  description:
    "Point Claude Code, Codex, Cursor, or any MCP-aware agent at a Tandem board. One stdio server, npx @jaximus/tandem-mcp, starts the local Tandem server and exposes the 18-tool facade.",
  alternates: { canonical: "/mcp" },
};

/* ─────────────────────────────────────────────────────────────────────────────
   /mcp — bring your own agent. One stdio MCP server, `@jaximus/tandem-mcp`,
   that (a) launches the local Tandem server and (b) proxies tool calls to it.
   There is no hosted endpoint: every client below spawns the same process.
   ───────────────────────────────────────────────────────────────────────────── */

const INSTALL_METHODS: Tab[] = [
  {
    id: "setup",
    label: "One command",
    badge: "Recommended",
    blurb:
      "Registers the MCP server with Claude Code at user scope (so it's available in every project), downloads this platform's Tandem binary into ~/.tandem/bin, starts it, and opens the board.",
    blocks: [{ code: SETUP_CMD }],
  },
  {
    id: "npx",
    label: "npx, by hand",
    blurb:
      "Nothing to install — your MCP client fetches and runs the package on demand. The server starts the local Tandem binary the first time an agent connects, so the whole setup is the client config in the wiring step below.",
    blocks: [{ text: "Claude Code, user scope:", code: MCP_ADD_CMD }, { text: "Start or open the board any time:", code: UP_CMD }],
  },
  {
    id: "global",
    label: "Global install",
    blurb: "Install once, get tandem-mcp on your PATH. Slightly faster startup than npx because there is no per-run fetch.",
    blocks: [
      { text: "Install:", code: "npm install -g @jaximus/tandem-mcp" },
      {
        text: "Then use `tandem-mcp` as the command (no args) in the wiring step, and:",
        code: "tandem-mcp up      # start + open the board\ntandem-mcp open    # open it (starts if needed)\ntandem-mcp down    # stop the server",
      },
    ],
  },
  {
    id: "dlx",
    label: "pnpm / yarn / bun",
    blurb: "If you don't use npm, your package manager's equivalent of npx works the same way.",
    blocks: [
      { text: "pnpm:", code: `"command": "pnpm",\n"args": ["dlx", "@jaximus/tandem-mcp"]` },
      { text: "yarn:", code: `"command": "yarn",\n"args": ["dlx", "@jaximus/tandem-mcp"]` },
      { text: "bun:", code: `"command": "bunx",\n"args": ["@jaximus/tandem-mcp"]` },
    ],
  },
  {
    id: "binary",
    label: "Standalone binary",
    blurb:
      "Skip the download step: grab tandem_<os>_<arch> from the GitHub release, put it on your PATH as `tandem`, and the MCP server uses it instead of fetching one. Also the way to run the server on a machine with no Node at all.",
    blocks: [
      {
        code: `# macOS (Apple silicon) — pick your file on the releases page
curl -L -o /usr/local/bin/tandem \\
  https://github.com/jaximus808/tandem/releases/latest/download/tandem_darwin_arm64
chmod +x /usr/local/bin/tandem
tandem            # serves http://127.0.0.1:7891, data in ~/.tandem`,
      },
    ],
  },
  {
    id: "source",
    label: "From source",
    blurb: "For contributors or a local fork. Requires Go 1.25, Node 22 and pnpm.",
    blocks: [
      {
        text: "Clone and build the single binary (web bundle embedded):",
        code: `git clone ${REPO_URL}.git
cd tandem
pnpm install
scripts/build.sh          # → dist/tandem`,
      },
      {
        text: "Point the MCP server at your build instead of the release download:",
        code: `"env": { "TANDEM_BIN": "/abs/path/to/tandem/dist/tandem" }`,
      },
    ],
  },
];

const NPX_JSON = `{
  "mcpServers": {
    "tandem": {
      "command": "npx",
      "args": ["-y", "@jaximus/tandem-mcp"]
    }
  }
}`;

const CLIENT_TABS: Tab[] = [
  {
    id: "claude-code",
    label: "Claude Code",
    blurb: (
      <>
        One command, user scope. <M>setup</M> runs exactly this for you; run it by hand if you&apos;d
        rather not.
      </>
    ),
    blocks: [{ code: MCP_ADD_CMD }, { text: "Or, per project, in .mcp.json:", code: NPX_JSON }],
  },
  {
    id: "cursor",
    label: "Cursor / Windsurf",
    blurb: "Same server works in any MCP-aware editor. Drop this into the client's MCP settings.",
    blocks: [{ code: NPX_JSON }],
  },
  {
    id: "codex",
    label: "Codex CLI",
    blurb: "Codex CLI supports MCP servers via its config file (typically ~/.codex/config.toml).",
    blocks: [{ code: `[mcp_servers.tandem]\ncommand = "npx"\nargs = ["-y", "@jaximus/tandem-mcp"]` }],
  },
  {
    id: "openai-agents",
    label: "OpenAI Agents SDK",
    blurb: "OpenAI's Agents SDK supports MCP servers natively. The Tandem tools show up as agent tools.",
    blocks: [
      {
        code: `# Python
from agents import Agent, Runner
from agents.mcp import MCPServerStdio

tandem = MCPServerStdio(
    params={
        "command": "npx",
        "args": ["-y", "@jaximus/tandem-mcp"],
    },
)

agent = Agent(
    name="executor",
    instructions=(
        "canvas_connect to the canvas code you are given, then loop: "
        "queue_next → task_claim → do the work → task_complete."
    ),
    mcp_servers=[tandem],
)

await Runner.run(agent, "Work the queue on canvas AB3XK9QZ")`,
      },
    ],
  },
  {
    id: "raw-stdio",
    label: "Custom orchestrator",
    blurb: "Any MCP client SDK (TypeScript, Python, Go, Rust) can spawn the server. Useful for bespoke multi-agent pipelines.",
    blocks: [
      {
        code: `// TypeScript — @modelcontextprotocol/sdk
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const transport = new StdioClientTransport({
  command: "npx",
  args: ["-y", "@jaximus/tandem-mcp"],
});

const client = new Client({ name: "my-orchestrator", version: "0.1.0" });
await client.connect(transport);

const { session } = await client.callTool({
  name: "canvas_connect",
  arguments: { code: "AB3XK9QZ", role: "planner", name: "orchestrator-1" },
});
const ready = await client.callTool({ name: "queue_next", arguments: { session } });
// → one subagent per task, each pasted the task's \`handoff\` block`,
      },
    ],
  },
];

const CLI = [
  ["up", "[--port N] [--data-dir P] [--code CODE] [--no-open] [--foreground]", "Find (or download) the binary, start it detached, wait for /healthz, open the board."],
  ["open", "[CODE]", "Open the board or one canvas in the browser; starts the server first if it isn't running."],
  ["down", "", "Stop the server `up` started."],
  ["setup", "", "up + register with Claude Code at user scope + append the loop snippet to ~/.claude/CLAUDE.md."],
  ["init", "[--name] [--code] [--write] [--force]", "In a repo: create a canvas, write its code into .mcp.json, print the CLAUDE.md snippet (--write appends it)."],
  ["listen", "--exec CMD [--port 8787]", "A loopback webhook receiver that runs CMD when tasks are approved — the approval becomes the go signal for an orchestrator."],
];

const ENV = [
  ["API_URL", "http://127.0.0.1:7891", "Where the Tandem server is. Only change it for a self-hosted instance."],
  ["TANDEM_DATA_DIR", "~/.tandem", "Binary cache, logs and pid file; the server keeps its SQLite file here too."],
  ["TANDEM_BIN", "(unset)", "Use this server binary instead of the downloaded one."],
  ["TANDEM_VERSION", "latest", "Which GitHub release `up` downloads when nothing is cached."],
  ["TANDEM_AUTOSTART", "1", "Set 0 to stop the MCP server from starting the backend on its own."],
  ["TANDEM_AUTOSTART_OPEN", "0", "Set 1 to also open the browser when an agent auto-starts the server."],
  ["TANDEM_CANVAS_CODE", "(unset)", "Written by `init`: this project's canvas, the default for canvas_connect."],
  ["TANDEM_FULL_TOOLS", "0", "Set 1 to also advertise the ~40-tool CRUD surface alongside the 18-tool facade."],
  ["MCP_TRACE", "off", "Per-tool-call timing on stderr (1 = readable, json = one object per line)."],
];

const TOOLS: { group: string; items: [string, string][] }[] = [
  {
    group: "Connect",
    items: [
      ["canvas_connect", "Bind the session to a canvas by code; registers you as planner or executor and returns the session handle every later call carries."],
      ["canvas_create", "Create a canvas and bind to it in one step. Returns its code and board URL."],
      ["agent_register", "Register (or re-register) an identity on the fleet view, optionally under a parent orchestrator."],
      ["context_get", "The one-call briefing: identity, document tabs, per-kind counts, queue state."],
    ],
  },
  {
    group: "Work the queue",
    items: [
      ["queue_next", "The approved, ready-to-work queue, each task with a paste-ready handoff block."],
      ["queue_wait", "The same read that parks server-side and returns the moment work is approved."],
      ["task_find", "Match a task by title without a board read."],
      ["task_get", "One task, hydrated with its linked notes, roadmap items and epic — plus any review verdict."],
      ["task_claim", "Atomic claim. `claimed: false` means another session won: take the next one."],
      ["task_progress", "One line per step. Doubles as the heartbeat that keeps the claim alive."],
      ["task_complete", "Hand back a result and links (commit, PR); the board resolves GitHub links live."],
    ],
  },
  {
    group: "Plan",
    items: [
      ["epic_propose", "A named batch and its tickets in one call, landing as proposed for one human approval."],
      ["task_propose", "Add a ticket to an existing batch."],
      ["task_amend", "Correct or withdraw a ticket you proposed; resubmit a rejected one with a note."],
      ["task_review", "On a peer-review canvas: pass a proposal, or send finished work back with a reason."],
    ],
  },
  {
    group: "Documents and board",
    items: [
      ["doc_write", "Write markdown to a named tab (creates it if new); pass a noteId to update in place."],
      ["doc_read", "Read one tab's notes back, scoped server-side."],
      ["board_status", "Counts by state, epics and their approval state, and every in-flight task with its holder."],
    ],
  },
];

export default function MCPPage() {
  return (
    <div className="min-h-screen bg-paper text-ink">
      <LandingNav />

      <div className="mx-auto max-w-4xl px-6 py-12">
        <section className="tandem-rise">
          <Eyebrow>Bring your own agent</Eyebrow>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight text-ink sm:text-5xl">
            Any MCP agent, one shared task board.
          </h1>
          <p className="mt-5 text-base leading-relaxed text-ink/65">
            Tandem&apos;s MCP server is a standard{" "}
            <a href={MCP_SPEC_URL} target="_blank" rel="noopener noreferrer" className="text-accent underline underline-offset-2 hover:text-ink">
              Model Context Protocol
            </a>{" "}
            stdio server. It isn&apos;t Claude-specific: any MCP-aware client — editor, agent framework,
            or an orchestrator you wrote yourself — spawns it, calls <M>canvas_connect</M> with a canvas
            code, and starts working the same board a human is looking at in the browser. It also
            starts the Tandem server on your machine if nothing is running yet.
          </p>
          <p className="mt-3 text-base leading-relaxed text-ink/65">
            Several agents can connect to the same canvas at once. Every claim, completion and note is
            broadcast over a WebSocket to every browser and every other agent on that code.
          </p>
        </section>

        {/* ── Install ───────────────────────────────────────────────────────── */}
        <section id="install" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">Install</h2>
          <p className="mt-1 text-sm text-ink/55">
            Pick whichever fits your setup — the one-liner is the easiest and works for almost everyone.
          </p>
          <div className="mt-6">
            <ClientTabs tabs={INSTALL_METHODS} />
          </div>
          <p className="mt-3 text-xs leading-relaxed text-ink/50">
            Binaries for macOS, Linux and Windows (amd64 + arm64) are on the{" "}
            <a href={RELEASES_URL} target="_blank" rel="noopener noreferrer" className="text-accent underline underline-offset-2 hover:text-ink">
              releases page
            </a>
            ; the package is{" "}
            <a href={NPM_URL} target="_blank" rel="noopener noreferrer" className="font-code text-[11px] text-accent underline underline-offset-2 hover:text-ink">
              @jaximus/tandem-mcp
            </a>{" "}
            on npm.
          </p>
        </section>

        {/* ── Wire ──────────────────────────────────────────────────────────── */}
        <section id="wire" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">Wire it into your client</h2>
          <p className="mt-1 text-sm text-ink/55">
            These snippets all use the npx form. Swap to <M>tandem-mcp</M> if you installed globally.
          </p>
          <div className="mt-6">
            <ClientTabs tabs={CLIENT_TABS} />
          </div>
        </section>

        {/* ── Connect ───────────────────────────────────────────────────────── */}
        <section id="connect" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">Connect to a canvas</h2>
          <p className="mt-3 text-sm leading-relaxed text-ink/65">
            Open the board (<M>tandem-mcp open</M>), click <B>New canvas</B>, and copy its 8-character
            code — or run <M>npx @jaximus/tandem-mcp init --write</M> inside a repo to create one and
            pin it to that project. Then tell your agent the code. It calls <M>canvas_connect</M> once;
            from then on every tool operates on that canvas, with no id to pass around.
          </p>
          <div className="mt-4">
            <CommandLine cmd="npx @jaximus/tandem-mcp init --write" label="init command" />
          </div>
        </section>

        {/* ── CLI ───────────────────────────────────────────────────────────── */}
        <section id="cli" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">The CLI</h2>
          <p className="mt-1 text-sm text-ink/55">
            The same package is the launcher. Run with no arguments it is the MCP server; with a
            subcommand it manages the local Tandem process.
          </p>
          <dl className="mt-6 divide-y divide-ink/10 overflow-hidden rounded-lg border border-ink/10">
            {CLI.map(([cmd, args, desc]) => (
              <div key={cmd} className="grid gap-1 bg-surface px-4 py-3 sm:grid-cols-[minmax(0,17rem)_1fr] sm:gap-6">
                <dt className="font-code text-[12.5px] text-ink">
                  tandem-mcp <span className="font-medium">{cmd}</span>{" "}
                  <span className="text-ink/45">{args}</span>
                </dt>
                <dd className="text-sm leading-relaxed text-ink/65">{desc}</dd>
              </div>
            ))}
          </dl>

          <h3 className="mt-10 text-xs font-medium uppercase tracking-wide text-ink/50">Environment</h3>
          <dl className="mt-3 divide-y divide-ink/10 overflow-hidden rounded-lg border border-ink/10">
            {ENV.map(([name, dflt, desc]) => (
              <div key={name} className="grid gap-1 bg-surface px-4 py-3 sm:grid-cols-[minmax(0,17rem)_1fr] sm:gap-6">
                <dt className="font-code text-[12.5px] text-ink">
                  {name} <span className="text-ink/45">= {dflt}</span>
                </dt>
                <dd className="text-sm leading-relaxed text-ink/65">{desc}</dd>
              </div>
            ))}
          </dl>
        </section>

        {/* ── Tool surface ──────────────────────────────────────────────────── */}
        <section id="tools" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">The tool surface</h2>
          <p className="mt-1 text-sm text-ink/55">
            Eighteen intent-level tools, advertised by default. Every one operates on whatever canvas
            the session connected to — the <M>session</M> handle from <M>canvas_connect</M> carries the
            binding, so there is never an id to pass around.
          </p>
          <div className="mt-6 grid gap-4 sm:grid-cols-2">
            {TOOLS.map((group) => (
              <div key={group.group} className="rounded-lg border border-ink/10 bg-surface p-4">
                <div className="mb-2 text-xs font-medium uppercase tracking-wide text-ink/50">{group.group}</div>
                <ul className="space-y-2.5">
                  {group.items.map(([name, desc]) => (
                    <li key={name} className="text-sm leading-relaxed">
                      <span className="font-code text-[12px] text-accent">{name}</span>
                      <span className="text-ink/60"> — {desc}</span>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
          <p className="mt-3 text-xs leading-relaxed text-ink/50">
            The older ~40-tool CRUD surface (documents, notes, roadmap items, raw actions, agents, each
            with a batch variant) is still callable and is advertised behind <M>TANDEM_FULL_TOOLS=1</M>.
          </p>
        </section>

        {/* ── Multi-agent example ───────────────────────────────────────────── */}
        <section id="multi-agent" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">Multi-agent flow: plan → approve → fan out</h2>
          <p className="mt-1 text-sm leading-relaxed text-ink/55">
            Because the board is the shared artifact, you can split work across sessions instead of
            stuffing everything into one prompt. Each session connects to the same canvas code; their
            claims and results land on the same board in real time, and you can watch (or steer) from
            the browser.
          </p>

          <div className="mt-6 rounded-lg border border-ink/10 bg-surface p-5">
            <ol className="space-y-4 text-sm text-ink/70">
              <li>
                <div className="font-semibold text-ink">1. Planner — proposes the batch</div>
                <p className="mt-1 leading-relaxed text-ink/60">
                  Connects as <M>role: &quot;planner&quot;</M>, reads <M>context_get</M>, and turns a
                  vague ask into an epic of named tickets with <M>epic_propose</M>. Nothing is claimable
                  yet.
                </p>
              </li>
              <li>
                <div className="font-semibold text-ink">2. You — approve once</div>
                <p className="mt-1 leading-relaxed text-ink/60">
                  One click on the board approves the epic and every ticket under it. The planner,
                  parked on <M>queue_wait</M>, wakes up with the ready queue.
                </p>
              </li>
              <li>
                <div className="font-semibold text-ink">3. Workers — one per ticket</div>
                <p className="mt-1 leading-relaxed text-ink/60">
                  The planner spawns a subagent per task and pastes it the task&apos;s <M>handoff</M>{" "}
                  block. Each worker connects as an executor, <M>task_claim</M>s its own ticket (the
                  loser of a race takes another), reports <M>task_progress</M>, and{" "}
                  <M>task_complete</M>s with the commit.
                </p>
              </li>
              <li>
                <div className="font-semibold text-ink">4. Human in the loop, throughout</div>
                <p className="mt-1 leading-relaxed text-ink/60">
                  Reject a ticket mid-batch and the reason lands on the proposer&apos;s next{" "}
                  <M>task_get</M>. Amend the plan and in-flight sessions pick it up on their next pull.
                  You don&apos;t stop the fleet to steer it.
                </p>
              </li>
            </ol>
          </div>

          <div className="mt-4 rounded-lg border border-accent/20 bg-accent/[0.05] p-4 text-sm leading-relaxed text-ink/70">
            <span className="font-semibold text-ink">The pattern:</span> the board is the blackboard.
            Each session only needs the code, an MCP client, and a narrow role. Hand-offs happen
            through board state, not through a shared prompt — which means you can mix vendors (Claude
            here, Codex there, a local open-weights model for the cheap step) without rewriting the
            orchestration.
          </div>
        </section>

        {/* ── Build your own ────────────────────────────────────────────────── */}
        <section id="build" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">Build your own integration</h2>
          <p className="mt-3 text-sm leading-relaxed text-ink/65">
            The MCP server is intentionally thin: it holds a canvas token and forwards tool calls to
            the Tandem HTTP API on <M>127.0.0.1:7891</M>. If you&apos;d rather skip it and talk to the
            API directly from your runtime, the same endpoints live under <M>/api/canvas/*</M>. The
            MCP layer exists so MCP-aware clients get a zero-config experience; it isn&apos;t required.
          </p>
          <p className="mt-3 text-sm leading-relaxed text-ink/65">
            Source lives in{" "}
            <a
              href={`${REPO_URL}/tree/main/apps/mcp-gateway`}
              target="_blank"
              rel="noopener noreferrer"
              className="font-code text-xs text-accent underline underline-offset-2 hover:text-ink"
            >
              apps/mcp-gateway/
            </a>{" "}
            — small enough to fork.
          </p>
          <div className="mt-6">
            <CodeBlock
              code={`# health + config of the local server
curl -s http://127.0.0.1:7891/healthz
curl -s http://127.0.0.1:7891/api/config     # {"auth":"none","local":true,"version":"0.1.1"}`}
            />
          </div>
        </section>

        <div className="mt-16 border-t border-ink/10 pt-6">
          <Link href="/" className="text-sm text-ink/50 hover:text-ink/70">
            ← Back to home
          </Link>
        </div>
      </div>

      <SiteFooter measure="max-w-4xl" />
    </div>
  );
}
