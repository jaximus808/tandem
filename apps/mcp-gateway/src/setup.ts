/**
 * `tandem-mcp setup` — one line from a fresh machine to a Claude Code that
 * knows Tandem in EVERY repo:
 *
 *   npx -y @jaximus/tandem-mcp setup
 *
 * It (1) registers this package as the `tandem` MCP server with Claude Code at
 * USER scope (`claude mcp add --scope user …`), so the tools exist in every
 * project without per-repo config; (2) appends a short, idempotent snippet to
 * `~/.claude/CLAUDE.md` teaching sessions that "sprint board" / "task board" /
 * "canvas" mean a Tandem canvas and how the queue loop goes; (3) starts the
 * local server and opens the board, via the same `up` logic as `tandem-mcp up`.
 *
 * `--project` does the same for ONE repo instead: `.mcp.json` (merged, never
 * clobbered — reusing init.ts's writer) and `./CLAUDE.md`. It does NOT create
 * a canvas or pin a code; that is `tandem-mcp init`.
 *
 * Re-running is safe: an already-registered server is reported as such, the
 * snippet is written once (marker match), `up` reuses a running server.
 *
 * Everything but `nodeSetupDeps()` runs against injected deps (see SetupDeps),
 * the same pattern as init.ts / up.ts, so the flow is unit-testable without
 * spawning `claude` or touching the home directory.
 */

import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

import {
  MCP_SERVER_KEY,
  mcpConfigPath,
  parseMcpConfig,
  serializeMcpConfig,
  type McpConfig,
  type McpServerEntry,
} from "./init.js";
import { UpUsageError, parseUpArgs, runUp, type UpDeps, type UpOptions } from "./up.js";

// ── Constants ────────────────────────────────────────────────────────────────

/** Marker that makes the CLAUDE.md snippet idempotently detectable. */
export const SETUP_MARKER = "<!-- tandem:setup -->";

/** The user-scope registration command, verbatim — printed when `claude` is missing. */
export const CLAUDE_MCP_ADD_ARGS = ["mcp", "add", "--scope", "user", MCP_SERVER_KEY, "--", "npx", "-y", "@jaximus/tandem-mcp"];
export const CLAUDE_MCP_ADD_COMMAND = `claude ${CLAUDE_MCP_ADD_ARGS.join(" ")}`;

/**
 * The lines that make "make me a sprint board" land on Tandem instead of a
 * markdown file. Short on purpose: it is appended to a file the user owns.
 */
export const SETUP_SNIPPET = `${SETUP_MARKER}
## Tandem

"sprint board", "agent board", "task board", "canvas" and "tandem" all mean a
Tandem canvas — the shared task board on the \`tandem\` MCP server. Never plan
work in a markdown file instead.

- Create one with \`canvas_create\` and give me the board URL it returns.
- Plan work on it with \`epic_propose\` (one task per unit of work), then
  \`queue_wait\` for my approval on the board — do not start before it.
- Working the queue: \`canvas_connect\` → \`queue_next\` → \`task_claim\` →
  \`task_progress\` → \`task_complete\`. Only work a task you claimed.
- \`tandem-mcp init --write\` in a repo pins it to its own canvas.
`;

// ── Args ─────────────────────────────────────────────────────────────────────

export interface SetupOptions {
  /** Register into ./.mcp.json + ./CLAUDE.md instead of user scope. */
  project: boolean;
  /** Skip the CLAUDE.md snippet. */
  claudeMd: boolean;
  /** Passed through to `up`. */
  up: UpOptions;
  help: boolean;
}

/**
 * Parse `setup` flags (argv AFTER the word `setup`). Its own flags are peeled
 * off first; the rest are `up`'s (--port, --data-dir, --no-open, …) so the
 * server half accepts exactly what `tandem-mcp up` does.
 */
export function parseSetupArgs(argv: string[], env: NodeJS.ProcessEnv, home: string = homedir()): SetupOptions {
  const own = { project: false, claudeMd: true, help: false };
  const rest: string[] = [];
  for (const arg of argv) {
    switch (arg) {
      case "--project":
        own.project = true;
        break;
      case "--no-claude-md":
        own.claudeMd = false;
        break;
      case "--help":
      case "-h":
        own.help = true;
        break;
      default:
        rest.push(arg);
    }
  }
  let up: UpOptions;
  try {
    up = parseUpArgs(rest, env, "up", home);
  } catch (err) {
    if (err instanceof UpUsageError) {
      throw new UpUsageError(err.message.replace("`tandem-mcp up`", "`tandem-mcp setup`"));
    }
    throw err;
  }
  if (up.code) throw new UpUsageError('Unknown option "--code" for `tandem-mcp setup`');
  return { ...own, up };
}

export function setupHelp(): string {
  return (
    `tandem-mcp setup — set up Claude Code for Tandem, everywhere, in one go.\n` +
    `\n` +
    `Usage:\n` +
    `  tandem-mcp setup [--project] [--no-claude-md] [--no-open] [--port N] [--data-dir P]\n` +
    `\n` +
    `What it does:\n` +
    `  1. Registers the \`tandem\` MCP server with Claude Code at USER scope\n` +
    `     (\`${CLAUDE_MCP_ADD_COMMAND}\`), so the tools\n` +
    `     exist in every repo. Prints the command instead if \`claude\` isn't on PATH.\n` +
    `  2. Appends a short snippet to ~/.claude/CLAUDE.md teaching sessions that\n` +
    `     "sprint board" / "task board" / "canvas" mean a Tandem canvas, and the\n` +
    `     queue loop. Idempotent (marker ${SETUP_MARKER}).\n` +
    `  3. Starts the local server and opens the board (same as \`tandem-mcp up\`).\n` +
    `\n` +
    `Options:\n` +
    `  --project        Do this for the current repo only: ./.mcp.json (merged,\n` +
    `                   never clobbered) and ./CLAUDE.md. Creates no canvas —\n` +
    `                   \`tandem-mcp init\` does that.\n` +
    `  --no-claude-md   Skip the CLAUDE.md snippet.\n` +
    `  --no-open        Don't open the browser.\n` +
    `  --port N         Server port (default $PORT, else 7891).\n` +
    `  --data-dir P     Data directory (default $TANDEM_DATA_DIR, else ~/.tandem).\n` +
    `  -h, --help       This help.\n` +
    `\n` +
    `Safe to re-run. Restart Claude Code afterwards so it picks up the new server.\n`
  );
}

// ── Deps ─────────────────────────────────────────────────────────────────────

export interface RunResult {
  code: number;
  stdout: string;
  stderr: string;
}

export interface SetupDeps {
  /** The `up` half — the same deps `tandem-mcp up` runs on. */
  up: UpDeps;
  home: string;
  cwd: string;
  /** First `name` on PATH, or null (for `claude`). */
  which(name: string): string | null;
  /** Run a command, capturing its output; never throws on a non-zero exit. */
  run(cmd: string, args: string[]): Promise<RunResult>;
  readFile(path: string): string | null;
  writeFile(path: string, content: string): void;
  out(line: string): void;
  err(line: string): void;
}

/** Real deps: node:fs, node:child_process, plus the given `up` deps. */
export function nodeSetupDeps(up: UpDeps): SetupDeps {
  return {
    up,
    home: homedir(),
    cwd: process.cwd(),
    which: up.which,
    run: (cmd, args) =>
      new Promise<RunResult>((resolveRun) => {
        const child = spawn(cmd, args, { stdio: ["ignore", "pipe", "pipe"] });
        let stdout = "";
        let stderr = "";
        child.stdout?.on("data", (d) => (stdout += String(d)));
        child.stderr?.on("data", (d) => (stderr += String(d)));
        child.on("error", (e) => resolveRun({ code: 127, stdout, stderr: `${stderr}${e.message}\n` }));
        child.on("close", (code) => resolveRun({ code: code ?? 1, stdout, stderr }));
      }),
    readFile: (p) => (existsSync(p) ? readFileSync(p, "utf8") : null),
    writeFile: (p, c) => {
      mkdirSync(dirname(p), { recursive: true });
      writeFileSync(p, c, "utf8");
    },
    out: (line) => process.stdout.write(`${line}\n`),
    err: (line) => process.stderr.write(`${line}\n`),
  };
}

// ── Pure pieces ──────────────────────────────────────────────────────────────

/** Where the snippet goes: `~/.claude/CLAUDE.md`, or `./CLAUDE.md` with --project. */
export function claudeMdPath(opts: Pick<SetupOptions, "project">, home: string, cwd: string): string {
  return opts.project ? join(cwd, "CLAUDE.md") : join(home, ".claude", "CLAUDE.md");
}

/**
 * Append the setup snippet unless it is already there. Returns null when
 * nothing needs writing. Marker-based like init.ts's appendSnippet.
 */
export function appendSetupSnippet(existing: string | null): string | null {
  const current = existing ?? "";
  if (current.includes(SETUP_MARKER)) return null;
  const body = current.replace(/\s*$/, "");
  return body ? `${body}\n\n${SETUP_SNIPPET}` : SETUP_SNIPPET;
}

/**
 * Merge the plain `tandem` server entry into a `.mcp.json` (project scope).
 * Like init.ts's mergeMcpConfig but with nothing to pin: an existing entry
 * keeps every field it has (command, args, env, canvas code); only a missing
 * command/args gets the npx default.
 */
export function mergeProjectMcpConfig(existing: McpConfig | null | undefined): McpConfig {
  const base: McpConfig = existing ? { ...existing } : {};
  const servers: Record<string, McpServerEntry> = { ...(base.mcpServers ?? {}) };
  const prior: McpServerEntry = { ...(servers[MCP_SERVER_KEY] ?? {}) };
  servers[MCP_SERVER_KEY] = {
    ...prior,
    command: prior.command ?? "npx",
    args: prior.args ?? ["-y", "@jaximus/tandem-mcp"],
  };
  base.mcpServers = servers;
  return base;
}

/** How `claude mcp add` reports a name that is already registered. */
export function isAlreadyRegistered(result: RunResult): boolean {
  return /already exists/i.test(`${result.stdout}\n${result.stderr}`);
}

// ── Flow ─────────────────────────────────────────────────────────────────────

export interface SetupReport {
  registered: "user" | "project" | "already" | "manual";
  claudeMd: "written" | "unchanged" | "skipped";
}

/**
 * Step 1: the MCP registration. Never fatal — a machine without `claude` on
 * PATH gets the exact command to run and everything else still happens.
 */
export async function registerMcpServer(opts: SetupOptions, deps: SetupDeps): Promise<SetupReport["registered"]> {
  if (opts.project) {
    const path = mcpConfigPath(deps.cwd);
    const raw = deps.readFile(path);
    const existing = raw === null ? null : parseMcpConfig(raw, path);
    if (existing?.mcpServers?.[MCP_SERVER_KEY]) {
      deps.out(`  MCP        ${path} already has a \`${MCP_SERVER_KEY}\` server — left as is.`);
      return "already";
    }
    deps.writeFile(path, serializeMcpConfig(mergeProjectMcpConfig(existing)));
    deps.out(`  MCP        registered \`${MCP_SERVER_KEY}\` in ${path} (this repo only).`);
    return "project";
  }

  if (!deps.which("claude")) {
    deps.out(`  MCP        \`claude\` is not on your PATH, so register it yourself once Claude Code is installed:`);
    deps.out(`             ${CLAUDE_MCP_ADD_COMMAND}`);
    return "manual";
  }
  const result = await deps.run("claude", CLAUDE_MCP_ADD_ARGS);
  if (result.code === 0) {
    deps.out(`  MCP        registered \`${MCP_SERVER_KEY}\` with Claude Code at user scope (every repo).`);
    return "user";
  }
  if (isAlreadyRegistered(result)) {
    deps.out(`  MCP        \`${MCP_SERVER_KEY}\` is already registered with Claude Code at user scope.`);
    return "already";
  }
  deps.err(`  MCP        \`${CLAUDE_MCP_ADD_COMMAND}\` failed (exit ${result.code}):`);
  for (const line of `${result.stdout}${result.stderr}`.trim().split("\n")) {
    if (line.trim()) deps.err(`             ${line}`);
  }
  deps.err(`             Run it yourself after fixing the above; the rest of setup continues.`);
  return "manual";
}

/** Step 2: the CLAUDE.md snippet. */
export function writeClaudeMdSnippet(opts: SetupOptions, deps: SetupDeps): SetupReport["claudeMd"] {
  if (!opts.claudeMd) {
    deps.out(`  CLAUDE.md  skipped (--no-claude-md).`);
    return "skipped";
  }
  const path = claudeMdPath(opts, deps.home, deps.cwd);
  const next = appendSetupSnippet(deps.readFile(path));
  if (next === null) {
    deps.out(`  CLAUDE.md  ${path} already has the Tandem section.`);
    return "unchanged";
  }
  deps.writeFile(path, next);
  deps.out(`  CLAUDE.md  added a Tandem section to ${path}.`);
  return "written";
}

/** `tandem-mcp setup`. Exit code: 0 unless the server could not start. */
export async function runSetup(opts: SetupOptions, deps: SetupDeps): Promise<number> {
  if (opts.help) {
    deps.out(setupHelp());
    return 0;
  }
  deps.out("");
  deps.out(`Setting up Tandem${opts.project ? " for this repo" : " for Claude Code"}:`);
  deps.out("");
  await registerMcpServer(opts, deps);
  writeClaudeMdSnippet(opts, deps);

  // Step 3: the server. runUp prints its own block (board URL, logs, stop).
  const code = await runUp(opts.up, deps.up);
  if (code !== 0) return code;

  deps.out(`  Restart Claude Code so it picks up the \`${MCP_SERVER_KEY}\` server.`);
  deps.out(`  Then in any session: "make me a tandem canvas" — or pin a repo to its own`);
  deps.out(`  canvas with \`tandem-mcp init --write\` inside it.`);
  deps.out("");
  return 0;
}
