import CopyButton from "@/components/CopyButton";
import SchedulerHero from "@/components/hero/SchedulerHero";
import SiteFooter from "@/components/SiteFooter";
import SiteNav from "@/components/SiteNav";
import { SETUP_CMD } from "@/lib/site";

export default function Page() {
  return (
    <>
      <SiteNav />

      <main>
        <section className="hero">
          <div className="container">
            <div className="hero-window">
              <SchedulerHero />
            </div>
            <h1>One task queue your parallel agent sessions claim work from.</h1>
            <p className="sub">
              Agents propose the work, you approve it once, and every session on the machine pulls from the same
              queue. It all runs locally: one binary, one SQLite file, no accounts.
            </p>
            <div className="actions">
              <a className="btn btn-primary" href="#install">
                Install
              </a>
              <a className="btn-link" href="/docs">
                Read the docs <span aria-hidden="true">→</span>
              </a>
            </div>
          </div>
        </section>

        <section className="install" id="install" aria-label="Install">
          <div className="container">
            <div className="cmd">
              <code>
                <span className="prompt" aria-hidden="true">
                  $
                </span>
                {SETUP_CMD}
              </code>
              <CopyButton text={SETUP_CMD} />
            </div>
            <p className="note">
              Adds the MCP server to Claude Code, downloads the binary into <code>~/.tandem</code>, starts it on{" "}
              <code>127.0.0.1:7891</code> and opens the board; restart Claude Code to pick it up. Other MCP clients run{" "}
              <code>npx -y @jaximus/tandem-mcp</code> directly.
            </p>
          </div>
        </section>

        <section className="section" aria-labelledby="how">
          <div className="container">
            <h2 id="how">How it works</h2>
            <p className="lede">
              When several agent sessions work on one repo, they usually share a TODO.md. A file can&apos;t track who
              is working on what. Tandem keeps tasks, claims and results in a queue that every session reads.
            </p>
            <ol className="steps">
              <li>
                <h3>An agent proposes an epic</h3>A named batch of tasks, each with a <code>TDM-n</code> ticket. Nothing
                runs yet.
              </li>
              <li>
                <h3>You approve it once</h3>On the board. That approves every task in it, and agents waiting on{" "}
                <code>queue_wait</code> start working.
              </li>
              <li>
                <h3>Sessions claim, atomically</h3>
                <code>queue_next</code>, then <code>task_claim</code>. A claim is one conditional{" "}
                <code>UPDATE … WHERE state = &apos;approved&apos;</code>. If two sessions claim the same task, one gets
                it and the other is told who has it and takes the next one. A claim with no progress for 15 minutes
                expires.
              </li>
              <li>
                <h3>Each task completes with a commit</h3>The session reports a result and links the commit. The board
                updates live.
              </li>
            </ol>
            <p className="stack">
              It all runs on your machine: one Go binary and one SQLite file in <code>~/.tandem</code>. No
              accounts. Any MCP client works: Claude Code, Codex, Cursor, or a script of your own.
            </p>
          </div>
        </section>
      </main>

      <SiteFooter />
    </>
  );
}
