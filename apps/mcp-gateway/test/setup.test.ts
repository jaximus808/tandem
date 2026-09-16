/**
 * `tandem-mcp setup` — everything runs against injected deps: no `claude`
 * spawned, no home directory touched, no server started. What's pinned is
 * the contract that makes it safe to run twice and safe on a machine without
 * Claude Code: the registration is reported (never fatal), the CLAUDE.md
 * snippet lands once, .mcp.json is merged not clobbered, and `up` still runs.
 */
import { test } from "node:test";
import assert from "node:assert/strict";

import {
  CLAUDE_MCP_ADD_ARGS,
  CLAUDE_MCP_ADD_COMMAND,
  SETUP_MARKER,
  SETUP_SNIPPET,
  appendSetupSnippet,
  claudeMdPath,
  isAlreadyRegistered,
  mergeProjectMcpConfig,
  parseSetupArgs,
  runSetup,
  type RunResult,
  type SetupDeps,
  type SetupOptions,
} from "../src/setup.js";
import { UpUsageError, type UpDeps } from "../src/up.js";

// ── Harness ──────────────────────────────────────────────────────────────────

interface Fake {
  deps: SetupDeps;
  files: Map<string, string>;
  runs: { cmd: string; args: string[] }[];
  out: string[];
  err: string[];
  spawned: string[];
  opened: string[];
}

function fake(over: { claudeOnPath?: boolean; runResult?: RunResult; files?: Record<string, string> } = {}): Fake {
  const f: Fake = {
    deps: undefined as unknown as SetupDeps,
    files: new Map(Object.entries(over.files ?? {})),
    runs: [],
    out: [],
    err: [],
    spawned: [],
    opened: [],
  };
  let healthy = false;
  const up: UpDeps = {
    env: {},
    platform: "darwin",
    arch: "arm64",
    version: "latest",
    exists: (p) => f.files.has(p),
    mkdirp: () => {},
    readFile: (p) => f.files.get(p) ?? null,
    writeFile: (p, data) => void f.files.set(p, typeof data === "string" ? data : Buffer.from(data).toString("utf8")),
    removeFile: (p) => void f.files.delete(p),
    which: (name) => (name === "tandem" ? "/usr/local/bin/tandem" : null),
    fetch: async () => ({ status: 404, ok: false, bytes: async () => new Uint8Array(), text: async () => "" }),
    health: async () => (healthy ? { ok: true, version: "0.1.1" } : { ok: false }),
    spawnDetached: (bin) => {
      f.spawned.push(bin);
      healthy = true;
      return 4242;
    },
    spawnForeground: async () => 0,
    openBrowser: (url) => void f.opened.push(url),
    alive: () => true,
    kill: () => true,
    sleep: async () => {},
    out: (l) => void f.out.push(l),
    err: (l) => void f.err.push(l),
  };
  f.deps = {
    up,
    home: "/home/u",
    cwd: "/proj",
    which: (name) => (name === "claude" ? (over.claudeOnPath === false ? null : "/usr/local/bin/claude") : null),
    run: async (cmd, args) => {
      f.runs.push({ cmd, args });
      return over.runResult ?? { code: 0, stdout: "Added stdio MCP server tandem\n", stderr: "" };
    },
    readFile: (p) => f.files.get(p) ?? null,
    writeFile: (p, c) => void f.files.set(p, c),
    out: (l) => void f.out.push(l),
    err: (l) => void f.err.push(l),
  };
  return f;
}

function opts(over: Partial<SetupOptions> = {}): SetupOptions {
  return {
    project: false,
    claudeMd: true,
    help: false,
    up: { port: 7891, dataDir: "/home/u/.tandem", open: true, foreground: false, help: false },
    ...over,
  };
}

// ── Args ─────────────────────────────────────────────────────────────────────

test("parseSetupArgs: own flags peel off, the rest are up's", () => {
  const o = parseSetupArgs(["--project", "--no-claude-md", "--no-open", "--port", "7900", "--data-dir", "/d"], {}, "/home/u");
  assert.equal(o.project, true);
  assert.equal(o.claudeMd, false);
  assert.equal(o.up.open, false);
  assert.equal(o.up.port, 7900);
  assert.equal(o.up.dataDir, "/d");

  const bare = parseSetupArgs([], {}, "/home/u");
  assert.equal(bare.project, false);
  assert.equal(bare.claudeMd, true);
  assert.equal(bare.up.open, true);
  assert.equal(bare.up.port, 7891);
  assert.equal(bare.up.dataDir, "/home/u/.tandem");

  assert.throws(() => parseSetupArgs(["--bogus"], {}, "/home/u"), (e: unknown) => e instanceof UpUsageError && /tandem-mcp setup/.test((e as Error).message));
  assert.throws(() => parseSetupArgs(["--code", "ABCDEFGH"], {}, "/home/u"), UpUsageError);
});

test("claudeMdPath: user scope is ~/.claude/CLAUDE.md, --project is ./CLAUDE.md", () => {
  assert.equal(claudeMdPath({ project: false }, "/home/u", "/proj"), "/home/u/.claude/CLAUDE.md");
  assert.equal(claudeMdPath({ project: true }, "/home/u", "/proj"), "/proj/CLAUDE.md");
});

// ── Registration ─────────────────────────────────────────────────────────────

test("user scope: runs `claude mcp add --scope user tandem -- npx -y @jaximus/tandem-mcp`", async () => {
  const f = fake();
  assert.equal(await runSetup(opts(), f.deps), 0);
  assert.deepEqual(f.runs, [{ cmd: "claude", args: CLAUDE_MCP_ADD_ARGS }]);
  assert.equal(CLAUDE_MCP_ADD_COMMAND, "claude mcp add --scope user tandem -- npx -y @jaximus/tandem-mcp");
  assert.ok(f.out.some((l) => /registered `tandem` with Claude Code at user scope/.test(l)));
  assert.ok(f.out.some((l) => /Restart Claude Code/.test(l)));
});

test("an already-registered server is reported as such, not as a failure", async () => {
  const f = fake({ runResult: { code: 1, stdout: "", stderr: "MCP server tandem already exists in user config\n" } });
  assert.equal(await runSetup(opts(), f.deps), 0);
  assert.ok(f.out.some((l) => /already registered/.test(l)));
  assert.equal(f.err.length, 0);
  assert.equal(isAlreadyRegistered({ code: 1, stdout: "", stderr: "MCP server tandem already exists in user config" }), true);
  assert.equal(isAlreadyRegistered({ code: 1, stdout: "", stderr: "unknown flag" }), false);
});

test("without `claude` on PATH the exact command is printed and setup still succeeds", async () => {
  const f = fake({ claudeOnPath: false });
  assert.equal(await runSetup(opts(), f.deps), 0);
  assert.equal(f.runs.length, 0);
  assert.ok(f.out.some((l) => l.includes(CLAUDE_MCP_ADD_COMMAND)));
  // …and the other two steps happened anyway.
  assert.ok(f.files.has("/home/u/.claude/CLAUDE.md"));
  assert.equal(f.spawned.length, 1);
});

test("a real `claude mcp add` failure is surfaced but does not abort setup", async () => {
  const f = fake({ runResult: { code: 2, stdout: "", stderr: "boom\n" } });
  assert.equal(await runSetup(opts(), f.deps), 0);
  assert.ok(f.err.some((l) => /failed \(exit 2\)/.test(l)));
  assert.ok(f.err.some((l) => /boom/.test(l)));
  assert.equal(f.spawned.length, 1);
});

test("--project writes ./.mcp.json without clobbering other servers or an existing tandem entry", async () => {
  const f = fake({
    files: {
      "/proj/.mcp.json": JSON.stringify({ mcpServers: { other: { command: "x" } } }),
    },
  });
  assert.equal(await runSetup(opts({ project: true }), f.deps), 0);
  assert.equal(f.runs.length, 0, "project scope never spawns claude");
  const cfg = JSON.parse(f.files.get("/proj/.mcp.json")!);
  assert.deepEqual(cfg.mcpServers.other, { command: "x" });
  assert.deepEqual(cfg.mcpServers.tandem, { command: "npx", args: ["-y", "@jaximus/tandem-mcp"] });

  // An existing tandem entry (e.g. init's, with a canvas code) is kept whole.
  const prior = { command: "node", args: ["dist/index.js"], env: { TANDEM_CANVAS_CODE: "ABCDEFGH" } };
  const merged = mergeProjectMcpConfig({ mcpServers: { tandem: prior } });
  assert.deepEqual(merged.mcpServers!.tandem, prior);

  // Second run: left as is.
  const before = f.files.get("/proj/.mcp.json");
  await runSetup(opts({ project: true }), f.deps);
  assert.equal(f.files.get("/proj/.mcp.json"), before);
  assert.ok(f.out.some((l) => /already has a `tandem` server/.test(l)));
});

test("--project puts the snippet in ./CLAUDE.md, not the home one", async () => {
  const f = fake();
  await runSetup(opts({ project: true }), f.deps);
  assert.ok(f.files.has("/proj/CLAUDE.md"));
  assert.equal(f.files.has("/home/u/.claude/CLAUDE.md"), false);
});

// ── CLAUDE.md snippet ────────────────────────────────────────────────────────

test("snippet: appended once to an existing file, created when missing, never twice", () => {
  assert.equal(appendSetupSnippet(null), SETUP_SNIPPET);
  const withExisting = appendSetupSnippet("# My rules\n\nbe nice\n\n\n")!;
  assert.equal(withExisting, `# My rules\n\nbe nice\n\n${SETUP_SNIPPET}`);
  assert.equal(appendSetupSnippet(withExisting), null);
  assert.equal(appendSetupSnippet(SETUP_SNIPPET), null);
});

test("snippet content: the marker, the vocabulary, the loop, and short", () => {
  assert.ok(SETUP_SNIPPET.startsWith(SETUP_MARKER));
  for (const word of ['"sprint board"', '"task board"', '"canvas"', "`canvas_create`", "`epic_propose`", "`queue_wait`", "`queue_next`", "`task_claim`", "`task_complete`", "markdown file"]) {
    assert.ok(SETUP_SNIPPET.includes(word), `snippet is missing ${word}`);
  }
  assert.ok(SETUP_SNIPPET.trim().split("\n").length <= 15, "snippet must stay short — it lands in a file the user owns");
});

test("the snippet lands in ~/.claude/CLAUDE.md exactly once across two runs", async () => {
  const f = fake({ files: { "/home/u/.claude/CLAUDE.md": "# mine\n" } });
  await runSetup(opts(), f.deps);
  await runSetup(opts(), f.deps);
  const md = f.files.get("/home/u/.claude/CLAUDE.md")!;
  assert.equal(md.split(SETUP_MARKER).length - 1, 1);
  assert.ok(md.startsWith("# mine"));
  assert.ok(f.out.some((l) => /added a Tandem section/.test(l)));
  assert.ok(f.out.some((l) => /already has the Tandem section/.test(l)));
});

test("--no-claude-md skips the snippet", async () => {
  const f = fake();
  await runSetup(opts({ claudeMd: false }), f.deps);
  assert.equal(f.files.has("/home/u/.claude/CLAUDE.md"), false);
  assert.ok(f.out.some((l) => /skipped \(--no-claude-md\)/.test(l)));
});

// ── The server half ──────────────────────────────────────────────────────────

test("setup starts the server and opens the board; --no-open keeps the browser closed", async () => {
  const f = fake();
  await runSetup(opts(), f.deps);
  assert.deepEqual(f.spawned, ["/usr/local/bin/tandem"]);
  assert.deepEqual(f.opened, ["http://127.0.0.1:7891/"]);

  const g = fake();
  await runSetup(opts({ up: { ...opts().up, open: false } }), g.deps);
  assert.equal(g.opened.length, 0);
});

test("--help prints usage and does nothing else", async () => {
  const f = fake();
  assert.equal(await runSetup(opts({ help: true }), f.deps), 0);
  assert.equal(f.runs.length, 0);
  assert.equal(f.spawned.length, 0);
  assert.ok(f.out[0].startsWith("tandem-mcp setup"));
});
