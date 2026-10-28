import type { Metadata } from "next";
import SiteFooter from "@/components/SiteFooter";
import SiteNav from "@/components/SiteNav";
import CodeBlock from "@/components/docs/CodeBlock";
import DocsNav, { type DocsSection } from "@/components/docs/DocsNav";
import { REPO_URL, SETUP_CMD } from "@/lib/site";

export const metadata: Metadata = {
  title: "Docs · Tandem",
  description:
    "Install Tandem, run the first session, and understand the loop: agents propose, you approve once, parallel sessions claim atomically. Tools, CLI, configuration and troubleshooting.",
  alternates: { canonical: "/docs" },
  openGraph: { title: "Tandem docs", url: "/docs" },
};

const ext = { target: "_blank", rel: "noopener noreferrer" } as const;
const doc = (path: string) => `${REPO_URL}/blob/main/${path}`;

const SECTIONS: DocsSection[] = [
  {
    id: "install",
    title: "Install",
    children: [
      { id: "what-setup-does", title: "What setup does" },
      { id: "other-clients", title: "Other MCP clients" },
    ],
  },
  {
    id: "quick-start",
    title: "Quick start",
    children: [
      { id: "first-prompt", title: "The first prompt" },
      { id: "pin-a-repo", title: "Pin a repo to a canvas" },
    ],
  },
  {
    id: "how-it-works",
    title: "How it works",
    children: [
      { id: "task-lifecycle", title: "A task's life" },
      { id: "atomic-claims", title: "Atomic claims" },
      { id: "heartbeats", title: "Heartbeats and expiry" },
    ],
  },
  {
    id: "orchestrating",
    title: "Orchestrating subagents",
    children: [
      { id: "handoff", title: "The handoff block" },
      { id: "queue-wait", title: "Waiting for approval" },
    ],
  },
  { id: "approval-policies", title: "Approval policies" },
  { id: "tools", title: "Tools" },
  { id: "cli", title: "CLI" },
  { id: "configuration", title: "Configuration" },
  { id: "troubleshooting", title: "Troubleshooting" },
];

const MCP_JSON = `{
  "mcpServers": {
    "tandem": {
      "command": "npx",
      "args": ["-y", "@jaximus/tandem-mcp"]
    }
  }
}`;

const SNIPPET = `## Tandem task queue

The shared work queue for this repo is Tandem canvas \`AB3XK9QZ\`.

Every session:

1. \`canvas_connect\` with code \`AB3XK9QZ\` — once per session. Pass \`role\`
   ("executor" if you'll work tasks yourself, "planner" if you'll dispatch
   them to subagents) and a \`name\`. Keep the \`session\` handle it returns and
   pass it as \`session\` on every later Tandem call.
2. \`queue_next\` — the approved, ready-to-work queue. That is the entry point
   for work; don't go looking for it by reading the canvas.
3. Pick one task, \`task_get\` for its hydrated context, then \`task_claim\`.
   The claim is atomic: if it returns \`{ claimed: false, claimedBy }\`,
   another session won — do NOT work on that task; go back to \`queue_next\`.
4. Do the work. Every commit message starts with the ticket id, e.g.
   \`TDM-7: add rate limiter\`. On long work, \`task_progress\` with one line
   per meaningful step — it doubles as a heartbeat.
5. \`task_complete\` with a \`result\` saying what was done and where — always
   include the commit hash(es) — plus \`links\` to the commit or PR.`;

const TOOLS: [string, string][] = [
  [
    "canvas_connect",
    "Bind the session to a canvas by its 8-character code. Pass role (planner or executor), name and, if an orchestrator spawned you, parentAgentId: the one call connects and registers you. Returns the board url and the session handle.",
  ],
  [
    "canvas_create",
    "Create a canvas and bind to it in one step, when the user gave no code.",
  ],
  [
    "agent_register",
    "Re-register after connect: fix a rejected parentAgentId, record the model, switch role.",
  ],
  [
    "context_get",
    "A summary of the canvas in one call: your identity, document tabs, counts by kind, queue state. It doesn't return the full board.",
  ],
  [
    "queue_next",
    "The approved tasks that are ready to work. Start here. Each task includes a handoff block.",
  ],
  [
    "queue_wait",
    "Same as queue_next, but waits on the server until something is approved. Default 25 s, up to 60; optional epicId. A timeout means nothing was approved yet; call it again.",
  ],
  [
    "task_find",
    "Find a task by part of its title when you have no id. A ticket ref (TDM-21) resolves directly.",
  ],
  [
    "task_get",
    "One task with its linked notes, roadmap items and epic hydrated, plus any review block explaining why it came back.",
  ],
  [
    "task_claim",
    "Claim a task atomically (approved to executing). If another session got it first, you get { claimed: false, claimedBy }; take the next task.",
  ],
  [
    "task_progress",
    "Add one line of progress to a task you hold. It is stored on the task and also extends your claim.",
  ],
  [
    "task_complete",
    'Finish with a result (files, commit hashes) and links to the commit or PR, or status: "failed" with an error.',
  ],
  [
    "task_propose",
    "Propose one task, or many via tasks. Lands as proposed; the answer carries a tellHuman line to relay.",
  ],
  [
    "task_amend",
    "Correct, withdraw or resubmit a task you proposed while it is still proposed or rejected.",
  ],
  [
    "task_review",
    "The reviewer's verb on a peer canvas: pass approves another agent's proposal; changes_requested sends another agent's finished work back with a reason.",
  ],
  [
    "epic_propose",
    "Propose an epic and its tasks in one call. One human approval releases the batch.",
  ],
  [
    "doc_write",
    "Leave a markdown note on a document tab; a new tab name creates the tab.",
  ],
  [
    "doc_read",
    "Read one tab's notes back by name or id, scoped server-side. Keep the noteId of anything you will revise.",
  ],
  [
    "board_status",
    "Counts by state, epics and their approval state, every in-flight task with its holder and claim age. Pass epic to expand one batch.",
  ],
];

function Ref({ children }: { children: React.ReactNode }) {
  return <code>{children}</code>;
}

export default function DocsPage() {
  return (
    <>
      <SiteNav current="docs" />

      <div className="docs-shell">
        <div className="container docs">
          <DocsNav sections={SECTIONS} />

          <main className="docs-main">
            <header className="docs-head">
              <h1>Tandem docs</h1>
              <p className="docs-lede">
                Tandem is a task queue for agent sessions. Agents propose
                tasks, you approve them on a board, and Claude Code, Codex or
                Cursor sessions claim them from the queue. Each task goes to
                exactly one session. It runs locally: one Go binary and one
                SQLite file in <Ref>~/.tandem</Ref>.
              </p>
            </header>

            {/* ---- Install ------------------------------------------------- */}
            <section className="docs-section" id="install">
              <h2>Install</h2>
              <p>
                One command. It needs Node (for <Ref>npx</Ref>) and, to register
                itself, Claude Code on your <Ref>PATH</Ref>.
              </p>
              <CodeBlock
                code={`$ ${SETUP_CMD}`}
                label="Copy the setup command"
              />
              <p>
                Restart Claude Code afterwards so it picks up the new server,
                then check it is connected:
              </p>
              <CodeBlock
                code={`$ claude mcp list\ntandem: npx -y @jaximus/tandem-mcp - ✓ Connected`}
                copy="claude mcp list"
              />

              <h3 id="what-setup-does">What setup does</h3>
              <ol>
                <li>
                  Registers the MCP server with Claude Code at user scope, so
                  every project can reach it:{" "}
                  <Ref>
                    claude mcp add --scope user tandem -- npx -y
                    @jaximus/tandem-mcp
                  </Ref>
                  . If <Ref>claude</Ref> is not on your <Ref>PATH</Ref> it
                  prints that command for you to run later.
                </li>
                <li>
                  Appends a short Tandem section to{" "}
                  <Ref>~/.claude/CLAUDE.md</Ref> (idempotent), so phrases like
                  &ldquo;make me a sprint board&rdquo; or &ldquo;a task
                  board&rdquo; map to <Ref>canvas_create</Ref> and{" "}
                  <Ref>epic_propose</Ref> instead of a markdown file.
                </li>
                <li>
                  Downloads this platform&rsquo;s <Ref>tandem</Ref> binary from
                  the latest GitHub release into <Ref>~/.tandem/bin</Ref>,
                  starts it on <Ref>http://127.0.0.1:7891</Ref>, and opens the
                  board.
                </li>
              </ol>
              <table>
                <thead>
                  <tr>
                    <th>Flag</th>
                    <th>Effect</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <Ref>--project</Ref>
                    </td>
                    <td>
                      Register in the current repo&rsquo;s <Ref>.mcp.json</Ref>{" "}
                      (merged, never overwritten) and <Ref>./CLAUDE.md</Ref>{" "}
                      instead of user scope.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>--no-claude-md</Ref>
                    </td>
                    <td>Skip the CLAUDE.md snippet.</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>--no-open</Ref>
                    </td>
                    <td>Don&rsquo;t open the browser.</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>--port N</Ref>, <Ref>--data-dir P</Ref>
                    </td>
                    <td>
                      Server port (default <Ref>$PORT</Ref>, else 7891) and data
                      directory (default <Ref>$TANDEM_DATA_DIR</Ref>, else{" "}
                      <Ref>~/.tandem</Ref>).
                    </td>
                  </tr>
                </tbody>
              </table>
              <p>Safe to re-run.</p>

              <h3 id="other-clients">Cursor, Codex, other clients</h3>
              <p>
                Any stdio MCP client spawns the same command,{" "}
                <Ref>npx -y @jaximus/tandem-mcp</Ref>. The usual JSON form:
              </p>
              <CodeBlock code={MCP_JSON} />
              <p>
                The MCP server talks to the local Tandem server on{" "}
                <Ref>127.0.0.1:7891</Ref>, and starts it itself the first time a
                tool call finds nothing listening, so a client that never ran{" "}
                <Ref>setup</Ref> still works. Start it by hand with{" "}
                <Ref>npx -y @jaximus/tandem-mcp up</Ref>.
              </p>
            </section>

            {/* ---- Quick start --------------------------------------------- */}
            <section className="docs-section" id="quick-start">
              <h2>Quick start</h2>

              <h3 id="first-prompt">The first prompt</h3>
              <p>In any Claude Code session, ask for a board in plain words:</p>
              <CodeBlock
                code={`make me a sprint board for the auth refactor`}
              />
              <p>
                The agent creates a canvas, proposes an epic with its tasks, and
                hands you the board URL. Nothing runs yet: every proposal lands
                as <Ref>proposed</Ref>. Open the board, read the plan, and
                approve the epic once. That one approval releases every task
                under it, and each task gets a ticket id like <Ref>TDM-7</Ref>.
              </p>
              <p>
                Then start as many workers as you like against the same queue:
              </p>
              <CodeBlock
                code={`# terminal 1 and terminal 2, same repo:\n$ claude "work through the approved Tandem queue until it's empty"`}
              />
              <p>
                Both sessions take tasks from the same queue. On the board, a
                claimed task shows as <em>executing</em> with the
                session&rsquo;s name, and each finished task shows its result
                and commit hash. A session waiting on <Ref>queue_wait</Ref>{" "}
                continues as soon as you approve more work.
              </p>

              <h3 id="pin-a-repo">Pin a repo to a canvas</h3>
              <p>For a repo that should always use the same board:</p>
              <CodeBlock code={`$ npx -y @jaximus/tandem-mcp init --write`} />
              <p>
                That creates a canvas named after the folder, writes its code
                into the repo&rsquo;s <Ref>.mcp.json</Ref> as{" "}
                <Ref>TANDEM_CANVAS_CODE</Ref> (the default for{" "}
                <Ref>canvas_connect</Ref>), and appends this snippet to{" "}
                <Ref>AGENTS.md</Ref> or <Ref>CLAUDE.md</Ref>. Without{" "}
                <Ref>--write</Ref> it prints the snippet for you to paste;{" "}
                <Ref>--code</Ref> wires an existing canvas instead of creating
                one.
              </p>
              <CodeBlock code={SNIPPET} />
            </section>

            {/* ---- How it works -------------------------------------------- */}
            <section className="docs-section" id="how-it-works">
              <h2>How it works</h2>
              <p>
                Your MCP client starts <Ref>tandem-mcp</Ref>, a small stdio
                proxy that forwards tool calls over HTTP to the{" "}
                <Ref>tandem</Ref> binary on <Ref>127.0.0.1:7891</Ref>. The
                binary stores everything in one SQLite file,{" "}
                <Ref>~/.tandem/tandem.db</Ref>. The board in your browser
                connects to the same server over a WebSocket and updates live.
                Everything runs locally. There are no accounts.
              </p>
              <p>
                In local mode, the person using the browser owns every canvas.
                Anything that connects over MCP with a canvas code is an agent.
                Agents propose and claim tasks. Only the human approves,
                rejects or deletes them.
              </p>

              <h3 id="task-lifecycle">A task&rsquo;s life</h3>
              <table>
                <thead>
                  <tr>
                    <th>From</th>
                    <th>To</th>
                    <th>When</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <Ref>proposed</Ref>
                    </td>
                    <td>
                      <Ref>approved</Ref> / <Ref>rejected</Ref>
                    </td>
                    <td>
                      The human decides on the board. A rejection carries a
                      reason the agent reads back.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>approved</Ref>
                    </td>
                    <td>
                      <Ref>executing</Ref>
                    </td>
                    <td>
                      <Ref>task_claim</Ref>, exactly one winner.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>executing</Ref>
                    </td>
                    <td>
                      <Ref>done</Ref> / <Ref>failed</Ref>
                    </td>
                    <td>
                      <Ref>task_complete</Ref> with a result and links, or{" "}
                      <Ref>status: &quot;failed&quot;</Ref>.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>approved</Ref>
                    </td>
                    <td>
                      <Ref>proposed</Ref>
                    </td>
                    <td>
                      Its title or body was edited: it re-enters the gate.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>done</Ref>
                    </td>
                    <td>
                      <Ref>approved</Ref>
                    </td>
                    <td>
                      A reviewer requested changes (
                      <a href="#approval-policies">peer policy</a>). The claim
                      and stale result are cleared; the reason stays on the
                      audit trail.
                    </td>
                  </tr>
                </tbody>
              </table>
              <p>
                Every task has a ticket id, <Ref>TDM-n</Ref>. Commit messages
                start with it (<Ref>TDM-7: add rate limiter</Ref>) and each
                completion links to its commit, so the board shows the commit
                for every task.
              </p>

              <h3 id="atomic-claims">Atomic claims</h3>
              <p>
                A claim is one conditional{" "}
                <Ref>UPDATE … WHERE state = &apos;approved&apos;</Ref> inside a
                serialized transaction. When two sessions race for the same
                task, exactly one wins; the other gets{" "}
                <Ref>{"{ claimed: false, claimedBy }"}</Ref> naming the holder,
                plus <Ref>tapOut: true</Ref>, and goes back to{" "}
                <Ref>queue_next</Ref>. Losing is normal and is not an error.
                Asking again for a task you already lost is refused by the MCP
                server without reaching the API.
              </p>
              <p>
                Two tests in the repo check this:{" "}
                <Ref>TestClaimRaceHasOneWinner</Ref> races eight goroutines on
                one SQLite row, and{" "}
                <Ref>TestConcurrentWorkerClaimsProduceOneWinner</Ref> races two
                workers through the HTTP handler (one <Ref>200</Ref>, one{" "}
                <Ref>409</Ref> naming the winner).
              </p>

              <h3 id="heartbeats">Heartbeats and expiry</h3>
              <p>
                <Ref>task_progress</Ref> stores a line on the task and extends
                the claim. A claim nobody has heartbeated for{" "}
                <Ref>CLAIM_TTL_MINUTES</Ref> (default 15) counts as stuck: the
                next <Ref>task_claim</Ref> on that task may take it over
                atomically. Expiry is lazy, with no background job; <Ref>0</Ref>{" "}
                disables it. <Ref>board_status</Ref> flags such a task with{" "}
                <Ref>staleClaim: true</Ref>. If a task takes longer than 15
                minutes, call <Ref>task_progress</Ref> along the way.
              </p>
            </section>

            {/* ---- Orchestrating ------------------------------------------- */}
            <section className="docs-section" id="orchestrating">
              <h2>Orchestrating subagents</h2>
              <p>
                A session that works tasks itself connects with{" "}
                <Ref>role: &quot;executor&quot;</Ref> and runs the loop:{" "}
                <Ref>queue_next</Ref>, <Ref>task_get</Ref>,{" "}
                <Ref>task_claim</Ref>, work, <Ref>task_complete</Ref>. A session
                that dispatches subagents connects with{" "}
                <Ref>role: &quot;planner&quot;</Ref> and never claims.
              </p>
              <p>
                An agent only claims tasks it will do itself. An orchestrator
                dispatches subagents. It doesn&rsquo;t claim tasks, doesn&rsquo;t
                complete them for a worker, and doesn&rsquo;t give a subagent
                its <Ref>session</Ref> handle; subagents get the canvas code
                instead. If an orchestrator claims a task and never works on
                it, the board shows the task in progress with nobody working on
                it.
              </p>

              <h3 id="handoff">The handoff block</h3>
              <p>
                Every task <Ref>queue_next</Ref> (or <Ref>queue_wait</Ref>)
                returns carries a <Ref>handoff</Ref> block: the canvas code, the
                task&rsquo;s id, ticket and title, the planner&rsquo;s{" "}
                <Ref>agentId</Ref> as parent, and the literal steps the worker
                follows. The orchestrator spawns one subagent per ready task and
                pastes that block in verbatim. The worker connects as an
                executor with <Ref>parentAgentId</Ref> set, claims its own task,
                works, and completes under the identity it claimed with. On{" "}
                <Ref>claimed: false</Ref> it takes a different ready task. To
                report, the orchestrator reads <Ref>board_status</Ref>. It
                reports tasks that are still <Ref>executing</Ref>; it
                doesn&rsquo;t complete them.
              </p>

              <h3 id="queue-wait">Waiting for approval</h3>
              <p>
                After proposing, show the human the <Ref>tellHuman</Ref> line
                the propose call returns, so they know something is waiting for
                approval. Then call <Ref>queue_wait</Ref> instead of ending the
                turn. It waits on the server and returns as soon as work is
                approved. The <Ref>status</Ref> field says what happened:
              </p>
              <table>
                <thead>
                  <tr>
                    <th>status</th>
                    <th>Means</th>
                    <th>Do</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <Ref>ready</Ref>
                    </td>
                    <td>Approved tasks, each with its handoff block.</td>
                    <td>
                      Claim one (alone) or dispatch one subagent per task.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>rejected</Ref>
                    </td>
                    <td>
                      A ticket you proposed came back, with the reason verbatim.
                    </td>
                    <td>
                      Fix what the reason names and <Ref>task_amend</Ref> the
                      same ticket with a <Ref>note</Ref>. Don&rsquo;t file a
                      fresh copy.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>timeout</Ref>
                    </td>
                    <td>
                      Nothing was approved inside the window. Not an error.
                    </td>
                    <td>Call it again.</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>busy</Ref>, <Ref>unsupported</Ref>
                    </td>
                    <td>
                      Waiter cap reached, or the API predates the endpoint. The
                      queue was read for you.
                    </td>
                    <td>
                      Treat it like <Ref>queue_next</Ref>.
                    </td>
                  </tr>
                </tbody>
              </table>
              <p>
                Default wait is 25 seconds, clamped to 1–60 via{" "}
                <Ref>timeoutSeconds</Ref>; <Ref>epicId</Ref> narrows it to the
                batch you just proposed.
              </p>
              <p>
                When nothing is running, approval can launch the orchestrator
                instead: <Ref>tandem-mcp listen --exec &apos;…&apos;</Ref>{" "}
                receives a signed <Ref>task.approved</Ref> webhook on{" "}
                <Ref>127.0.0.1:8787</Ref> and runs the command. The full recipe
                is{" "}
                <a href={doc("docs/ORCHESTRATION.md")} {...ext}>
                  docs/ORCHESTRATION.md
                </a>
                .
              </p>
            </section>

            {/* ---- Approval policies --------------------------------------- */}
            <section className="docs-section" id="approval-policies">
              <h2>Approval policies</h2>
              <p>
                A canvas is on exactly one policy, set by its owner in the
                board&rsquo;s share dialog. It decides which state an
                agent-proposed task is born in and who may approve it.
              </p>
              <table>
                <thead>
                  <tr>
                    <th>Policy</th>
                    <th>An agent-proposed task is born</th>
                    <th>Epic approval cascades</th>
                    <th>Who approves a task</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <Ref>strict</Ref>
                    </td>
                    <td>
                      <Ref>proposed</Ref>
                    </td>
                    <td>No, each task keeps its own gate</td>
                    <td>Human</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>epic</Ref> (default)
                    </td>
                    <td>
                      <Ref>approved</Ref> if its epic is already approved, else{" "}
                      <Ref>proposed</Ref>
                    </td>
                    <td>Yes</td>
                    <td>Human</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>auto</Ref>
                    </td>
                    <td>
                      <Ref>approved</Ref>
                    </td>
                    <td>n/a</td>
                    <td>Nothing waits</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>peer</Ref>
                    </td>
                    <td>
                      <Ref>proposed</Ref>, always
                    </td>
                    <td>No, deliberately off</td>
                    <td>Human, or a different agent</td>
                  </tr>
                </tbody>
              </table>
              <p>
                A task proposed with <Ref>requiresApproval: true</Ref> lands{" "}
                <Ref>proposed</Ref> under every policy, <Ref>auto</Ref>{" "}
                included. Agents use it to flag work outside the approved plan.
              </p>
              <p>
                Under <Ref>peer</Ref>, a registered agent reviews another
                agent&rsquo;s work with <Ref>task_review</Ref>:{" "}
                <Ref>outcome: &quot;pass&quot;</Ref> approves a still-
                <Ref>proposed</Ref> task into the ready queue;{" "}
                <Ref>outcome: &quot;changes_requested&quot;</Ref> with a
                required <Ref>reason</Ref> sends a <Ref>done</Ref> task back to{" "}
                <Ref>approved</Ref>, unclaimed. The server derives both
                identities itself, so an agent cannot pass its own proposal or
                bounce its own completion; refusals come back as data (
                <Ref>reviewed: false</Ref> with a <Ref>refusal</Ref> code), not
                errors. Approving an epic, rejecting, and bulk approval stay
                human on every policy. On any policy other than <Ref>peer</Ref>,{" "}
                <Ref>task_review</Ref> answers <Ref>reviewed: false</Ref> and
                the human gate is unchanged.
              </p>
            </section>

            {/* ---- Tools --------------------------------------------------- */}
            <section className="docs-section" id="tools">
              <h2>Tools</h2>
              <p>
                The default MCP surface has 18 tools. Every
                tool except the two connectors takes the <Ref>session</Ref>{" "}
                handle <Ref>canvas_connect</Ref> returns; pass it on every call.
                Where a tool takes a task <Ref>id</Ref>, a ticket ref (
                <Ref>TDM-21</Ref>, <Ref>#21</Ref>, <Ref>21</Ref>) works as well
                as the uuid.
              </p>
              <table className="docs-tools">
                <thead>
                  <tr>
                    <th>Tool</th>
                    <th>What it does</th>
                  </tr>
                </thead>
                <tbody>
                  {TOOLS.map(([name, what]) => (
                    <tr key={name}>
                      <td>
                        <Ref>{name}</Ref>
                      </td>
                      <td>{what}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <p>
                The raw CRUD surface (documents, notes, roadmap items, raw
                actions, about 80 tools) is still callable but only advertised
                with <Ref>TANDEM_FULL_TOOLS=1</Ref> or{" "}
                <Ref>tandem-mcp --full-tools</Ref>. Per-tool arguments:{" "}
                <a href={doc("docs/TOOLS.md")} {...ext}>
                  docs/TOOLS.md
                </a>
                .
              </p>
            </section>

            {/* ---- CLI ----------------------------------------------------- */}
            <section className="docs-section" id="cli">
              <h2>CLI</h2>
              <p>
                All from <Ref>npx -y @jaximus/tandem-mcp &lt;command&gt;</Ref>,
                or <Ref>tandem-mcp &lt;command&gt;</Ref> after{" "}
                <Ref>npm i -g @jaximus/tandem-mcp</Ref>. With no command it runs
                as the stdio MCP server, which is what your client spawns.
              </p>
              <table className="docs-cli">
                <thead>
                  <tr>
                    <th>Command</th>
                    <th>What it does</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <Ref>setup</Ref>
                    </td>
                    <td>
                      First-run onboarding, as above. <Ref>--project</Ref>,{" "}
                      <Ref>--no-claude-md</Ref>, <Ref>--no-open</Ref>,{" "}
                      <Ref>--port N</Ref>, <Ref>--data-dir P</Ref>.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>up</Ref>
                    </td>
                    <td>
                      Start the local server, downloading the binary on first
                      run, and open the board. A server already answering on the
                      port is reused, never duplicated. <Ref>--port N</Ref>,{" "}
                      <Ref>--data-dir P</Ref>, <Ref>--code CODE</Ref> (open that
                      canvas), <Ref>--no-open</Ref>, <Ref>--foreground</Ref>{" "}
                      (attached, logs on stderr).
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>open [CODE]</Ref>
                    </td>
                    <td>
                      Open the board, or one canvas, starting the server first
                      if it is not running.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>down</Ref>
                    </td>
                    <td>
                      Stop the server <Ref>up</Ref> started.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>init</Ref>
                    </td>
                    <td>
                      Wire the current repo: create a canvas, write{" "}
                      <Ref>.mcp.json</Ref>, print the agent snippet.{" "}
                      <Ref>--write</Ref> appends it to <Ref>AGENTS.md</Ref> /{" "}
                      <Ref>CLAUDE.md</Ref>; <Ref>--code CODE</Ref> uses an
                      existing canvas; <Ref>--name</Ref>, <Ref>--dir</Ref>,{" "}
                      <Ref>--force</Ref>. Re-running on a wired repo reprints
                      the snippet and changes nothing.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>listen</Ref>
                    </td>
                    <td>
                      Local webhook listener that runs a command when work is
                      approved. <Ref>--exec</Ref> (required), <Ref>--port</Ref>{" "}
                      (default 8787), <Ref>--secret</Ref> (or{" "}
                      <Ref>TANDEM_WEBHOOK_SECRET</Ref>), <Ref>--events</Ref>{" "}
                      (default <Ref>task.approved</Ref>), <Ref>--debounce</Ref>.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>--full-tools</Ref>
                    </td>
                    <td>As the MCP server, also advertise the CRUD surface.</td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>--version</Ref>, <Ref>--help</Ref>
                    </td>
                    <td>
                      Each subcommand also takes <Ref>--help</Ref>.
                    </td>
                  </tr>
                </tbody>
              </table>
            </section>

            {/* ---- Configuration ------------------------------------------- */}
            <section className="docs-section" id="configuration">
              <h2>Configuration</h2>
              <p>
                No configuration is required. By default the server runs in
                local mode and keeps everything in one directory:
              </p>
              <table>
                <thead>
                  <tr>
                    <th>Path</th>
                    <th>What it is</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>
                      <Ref>~/.tandem/tandem.db</Ref>
                    </td>
                    <td>
                      Every canvas, task, note and webhook. Back it up by
                      copying it.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>~/.tandem/secret</Ref>
                    </td>
                    <td>
                      The auto-generated JWT secret, so tokens survive restarts.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>~/.tandem/bin/tandem</Ref>
                    </td>
                    <td>
                      The downloaded binary. <Ref>TANDEM_BIN</Ref>, or a{" "}
                      <Ref>tandem</Ref> on your <Ref>PATH</Ref>, wins over it.
                    </td>
                  </tr>
                  <tr>
                    <td>
                      <Ref>~/.tandem/images/</Ref>, <Ref>~/.tandem/logs/</Ref>
                    </td>
                    <td>Uploaded images; the detached server&rsquo;s log.</td>
                  </tr>
                </tbody>
              </table>
              <p>
                Server settings, read from the environment (or a <Ref>.env</Ref>{" "}
                next to the binary):
              </p>
              <div className="table-scroll">
                <table className="docs-env">
                  <thead>
                    <tr>
                      <th>Variable</th>
                      <th>Default</th>
                      <th>What it does</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>
                        <Ref>TANDEM_DATA_DIR</Ref>
                      </td>
                      <td>
                        <Ref>~/.tandem</Ref>
                      </td>
                      <td>Moves the whole directory.</td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>TANDEM_DB</Ref>
                      </td>
                      <td>
                        <Ref>&lt;data dir&gt;/tandem.db</Ref>
                      </td>
                      <td>
                        SQLite file. <Ref>:memory:</Ref> gives a throwaway
                        board.
                      </td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>PORT</Ref>
                      </td>
                      <td>
                        <Ref>7891</Ref>
                      </td>
                      <td>
                        HTTP port. <Ref>up --port</Ref> sets the same thing.
                      </td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>CLAIM_TTL_MINUTES</Ref>
                      </td>
                      <td>
                        <Ref>15</Ref>
                      </td>
                      <td>
                        How long an unheartbeated claim is honored before the
                        next <Ref>task_claim</Ref> may take it over.{" "}
                        <Ref>0</Ref> disables expiry.
                      </td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>JWT_TOKEN_TTL</Ref>
                      </td>
                      <td>
                        <Ref>24h</Ref>
                      </td>
                      <td>Canvas token lifetime.</td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>WEBHOOKS_ENABLED</Ref>
                      </td>
                      <td>
                        <Ref>true</Ref>
                      </td>
                      <td>
                        The outbound delivery worker <Ref>listen</Ref> receives
                        from.
                      </td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>TANDEM_WEBHOOKS_ALLOW_PRIVATE</Ref>
                      </td>
                      <td>on locally</td>
                      <td>
                        Whether webhooks may target loopback and private
                        addresses. Set <Ref>0</Ref> to turn the SSRF guard back
                        on.
                      </td>
                    </tr>
                    <tr>
                      <td>
                        <Ref>GH_STATUS_TOKEN</Ref>
                      </td>
                      <td>unset</td>
                      <td>
                        Optional public-read GitHub token for resolving commit
                        and PR links on the board (60 requests/hour
                        unauthenticated, 5,000 with one). A token that can read
                        private repos is refused.
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <p>
                The server answers <Ref>GET /healthz</Ref> and{" "}
                <Ref>GET /api/config</Ref> (
                <Ref>{"{auth, local, version}"}</Ref>
                ). The MCP server&rsquo;s own variables: <Ref>API_URL</Ref>{" "}
                (default the local server), <Ref>TANDEM_VERSION</Ref> (which
                release <Ref>up</Ref> downloads, default latest),{" "}
                <Ref>TANDEM_AUTOSTART=0</Ref> (don&rsquo;t start the backend
                from a tool call), <Ref>TANDEM_AUTOSTART_OPEN=1</Ref> (open the
                browser when it does), <Ref>TANDEM_CANVAS_CODE</Ref>,{" "}
                <Ref>TANDEM_FULL_TOOLS</Ref>, and <Ref>MCP_TRACE=1</Ref> for
                per-call timing on stderr. Every server setting is listed in{" "}
                <a href={doc("apps/api/.env.example")} {...ext}>
                  apps/api/.env.example
                </a>
                .
              </p>
              <p>
                To share one instance with a team, run the same binary with{" "}
                <Ref>TANDEM_AUTH=google</Ref> for Google sign-in, personal
                access tokens and per-account sharing; agents then set{" "}
                <Ref>API_URL</Ref> and <Ref>TANDEM_TOKEN</Ref>. See{" "}
                <a href={doc("docs/SELF-HOSTING.md")} {...ext}>
                  docs/SELF-HOSTING.md
                </a>
                .
              </p>
            </section>

            {/* ---- Troubleshooting ----------------------------------------- */}
            <section className="docs-section" id="troubleshooting">
              <h2>Troubleshooting</h2>
              <dl className="docs-faq">
                <dt>The tools don&rsquo;t show up after setup</dt>
                <dd>
                  Restart Claude Code; it reads its MCP servers at launch. Then{" "}
                  <Ref>claude mcp list</Ref> should show <Ref>tandem</Ref> as
                  connected.
                </dd>

                <dt>
                  setup said <Ref>claude</Ref> is not on your PATH
                </dt>
                <dd>
                  It printed the registration command instead of running it. Run{" "}
                  <Ref>
                    claude mcp add --scope user tandem -- npx -y
                    @jaximus/tandem-mcp
                  </Ref>{" "}
                  once Claude Code is installed, or configure your client by
                  hand as in <a href="#other-clients">other clients</a>.
                </dd>

                <dt>The server isn&rsquo;t running</dt>
                <dd>
                  The MCP server starts it on the first tool call that gets a
                  connection refused on the loopback address (unless{" "}
                  <Ref>TANDEM_AUTOSTART=0</Ref>). To start it yourself,{" "}
                  <Ref>npx -y @jaximus/tandem-mcp up</Ref>; it reuses a server
                  already on the port rather than starting a second one.{" "}
                  <Ref>down</Ref> stops the one <Ref>up</Ref> started, and says
                  so if none is running. <Ref>curl 127.0.0.1:7891/healthz</Ref>{" "}
                  tells you which case you are in.
                </dd>

                <dt>
                  <Ref>queue_next</Ref> is empty but work was proposed
                </dt>
                <dd>
                  Nothing is approved yet. Tasks under a still-
                  <Ref>proposed</Ref> epic are not in the ready queue; approve
                  the epic on the board. <Ref>board_status</Ref> shows what is
                  waiting and where.
                </dd>

                <dt>
                  <Ref>task_claim</Ref> returned <Ref>claimed: false</Ref>
                </dt>
                <dd>
                  Another session claimed it first. This is expected. The
                  answer names the session that holds it; take the next task
                  from <Ref>queue_next</Ref>.
                </dd>

                <dt>A task has sat in executing for a long time</dt>
                <dd>
                  Its session probably stopped. After{" "}
                  <Ref>CLAIM_TTL_MINUTES</Ref> (15 by default) without a{" "}
                  <Ref>task_progress</Ref> heartbeat, <Ref>board_status</Ref>{" "}
                  marks it <Ref>staleClaim</Ref> and the next{" "}
                  <Ref>task_claim</Ref> takes it over. Sessions doing long work
                  should report progress as they go.
                </dd>

                <dt>An agent proposed work and then went quiet</dt>
                <dd>
                  It is probably waiting on <Ref>queue_wait</Ref> for your
                  approval. Approve the work on the board and it continues.
                  Agents are supposed to show you the <Ref>tellHuman</Ref> line
                  from the propose answer before they wait.
                </dd>

                <dt>
                  <Ref>listen</Ref> answers 401 or 400
                </dt>
                <dd>
                  401 means the signature didn&rsquo;t verify, usually because
                  the secret is wrong (<Ref>--secret</Ref> takes priority over{" "}
                  <Ref>TANDEM_WEBHOOK_SECRET</Ref>). 400 means the timestamp is
                  more than 60 seconds off, usually because the clock drifted
                  while the laptop was asleep. The delivery list under the
                  board&rsquo;s webhook settings shows the response.
                </dd>
              </dl>
            </section>
          </main>
        </div>
      </div>

      <SiteFooter />
    </>
  );
}
