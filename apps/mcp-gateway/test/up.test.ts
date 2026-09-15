/**
 * `tandem-mcp up` / `open` / `down` and the MCP auto-start path (local-first).
 * Everything runs against injected deps: no disk, no network, no spawn. What's
 * pinned is the contract the launcher and the Go binary agree on — binary
 * resolution order, the release asset naming, the SHA256 gate, "reuse a running
 * server, never duplicate it", and the data-dir layout.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { join } from "node:path";

import {
  BINARY_NAME,
  DEFAULT_PORT,
  HEALTH_TIMEOUT_MS,
  LOG_FILE,
  PID_FILE,
  UpUsageError,
  autoStartEnabled,
  autoStartLocal,
  boardUrl,
  cachedBinaryPath,
  defaultDataDir,
  digestFromSums,
  downloadBinary,
  ensureServer,
  isLocalApiUrl,
  parseUpArgs,
  releaseAssetName,
  releaseAssetUrl,
  resolveBinary,
  runDown,
  runUp,
  serverEnv,
  sha256Hex,
  type FetchResult,
  type UpDeps,
} from "../src/up.js";
import { Gateway, isConnectionRefused } from "../src/gateway.js";

// ── Fake deps ────────────────────────────────────────────────────────────────

interface Fake {
  deps: UpDeps;
  files: Map<string, Uint8Array | string>;
  modes: Map<string, number>;
  dirs: Set<string>;
  spawned: Array<{ bin: string; env: NodeJS.ProcessEnv; logPath: string }>;
  opened: string[];
  killed: number[];
  out: string[];
  err: string[];
  /** Mutable: what /healthz answers. Set to true to simulate a live server. */
  healthy: boolean;
  /** Health flips to true after this many probes (simulates startup). */
  healthyAfter: number;
  fetches: string[];
}

function fake(over: Partial<UpDeps> & { onPath?: string | null; responses?: Record<string, FetchResult | (() => FetchResult)> } = {}): Fake {
  // One object, mutated in place: the deps close over `f`, and tests flip
  // `f.healthy` / `f.healthyAfter` after construction — a spread copy would
  // silently detach them.
  const f = {
    deps: undefined as unknown as UpDeps,
    files: new Map<string, Uint8Array | string>(),
    modes: new Map<string, number>(),
    dirs: new Set<string>(),
    spawned: [] as Fake["spawned"],
    opened: [] as string[],
    killed: [] as number[],
    out: [] as string[],
    err: [] as string[],
    healthy: false,
    healthyAfter: 0,
    fetches: [] as string[],
  };
  let probes = 0;
  let alivePids = new Set<number>();
  const deps: UpDeps = {
    env: {},
    platform: "darwin",
    arch: "arm64",
    version: "3.0.0",
    exists: (p) => f.files.has(p),
    mkdirp: (p) => void f.dirs.add(p),
    readFile: (p) => {
      const v = f.files.get(p);
      return v === undefined ? null : typeof v === "string" ? v : Buffer.from(v).toString("utf8");
    },
    writeFile: (p, data, mode) => {
      f.files.set(p, data);
      if (mode !== undefined) f.modes.set(p, mode);
    },
    removeFile: (p) => void f.files.delete(p),
    which: () => over.onPath ?? null,
    fetch: async (url) => {
      f.fetches.push(url);
      const r = over.responses?.[url];
      if (!r) return { status: 404, ok: false, bytes: async () => new Uint8Array(), text: async () => "" };
      return typeof r === "function" ? r() : r;
    },
    health: async () => {
      probes++;
      if (f.healthyAfter > 0 && probes >= f.healthyAfter) f.healthy = true;
      return f.healthy ? { ok: true, version: "0.1.0" } : { ok: false };
    },
    spawnDetached: (bin, env, logPath) => {
      f.spawned.push({ bin, env, logPath });
      const pid = 4242;
      alivePids.add(pid);
      return pid;
    },
    spawnForeground: async () => 0,
    openBrowser: (url) => void f.opened.push(url),
    alive: (pid) => alivePids.has(pid),
    kill: (pid) => {
      f.killed.push(pid);
      return alivePids.delete(pid);
    },
    sleep: async () => {},
    out: (l) => void f.out.push(l),
    err: (l) => void f.err.push(l),
    ...over,
  };
  // Let tests mark a pid alive without spawning (for `down`).
  (deps as UpDeps & { _alive: Set<number> })._alive = alivePids;
  f.deps = deps;
  return f;
}

function bytesResponse(bytes: Uint8Array): FetchResult {
  return { status: 200, ok: true, bytes: async () => bytes, text: async () => Buffer.from(bytes).toString("utf8") };
}
function textResponse(text: string): FetchResult {
  return { status: 200, ok: true, bytes: async () => new TextEncoder().encode(text), text: async () => text };
}

// ── Args ─────────────────────────────────────────────────────────────────────

test("parseUpArgs: defaults come from PORT / TANDEM_DATA_DIR, else 7891 and ~/.tandem", () => {
  const bare = parseUpArgs([], {}, "up", "/home/me");
  assert.equal(bare.port, DEFAULT_PORT);
  assert.equal(bare.dataDir, join("/home/me", ".tandem"));
  assert.equal(bare.open, true);
  assert.equal(bare.foreground, false);
  assert.equal(bare.code, undefined);

  const env = parseUpArgs([], { PORT: "9000", TANDEM_DATA_DIR: "/srv/tandem" }, "up", "/home/me");
  assert.equal(env.port, 9000);
  assert.equal(env.dataDir, "/srv/tandem");

  // Garbage PORT is ignored, not fatal.
  assert.equal(parseUpArgs([], { PORT: "lots" }, "up", "/h").port, DEFAULT_PORT);
});

test("parseUpArgs: flags, = form, --code upper-cases, --no-open, -f", () => {
  const o = parseUpArgs(
    ["--port=8000", "--data-dir", "/tmp/t", "--code", "ab3xk9qz", "--no-open", "-f"],
    {},
    "up",
    "/h"
  );
  assert.equal(o.port, 8000);
  assert.equal(o.dataDir, "/tmp/t");
  assert.equal(o.code, "AB3XK9QZ");
  assert.equal(o.open, false);
  assert.equal(o.foreground, true);
});

test("parseUpArgs: `open CODE` takes one positional; `up` does not", () => {
  assert.equal(parseUpArgs(["ab3xk9qz"], {}, "open", "/h").code, "AB3XK9QZ");
  assert.throws(() => parseUpArgs(["AB3XK9QZ"], {}, "up", "/h"), UpUsageError);
  assert.throws(() => parseUpArgs(["--bogus"], {}, "up", "/h"), UpUsageError);
  assert.throws(() => parseUpArgs(["--port", "nope"], {}, "up", "/h"), UpUsageError);
  assert.throws(() => parseUpArgs(["--port"], {}, "up", "/h"), UpUsageError);
});

test("defaultDataDir: env wins, else ~/.tandem", () => {
  assert.equal(defaultDataDir({}, "/Users/x"), "/Users/x/.tandem");
  assert.equal(defaultDataDir({ TANDEM_DATA_DIR: "/data" }, "/Users/x"), "/data");
  assert.equal(defaultDataDir({ TANDEM_DATA_DIR: "  " }, "/Users/x"), "/Users/x/.tandem");
});

// ── Release assets ───────────────────────────────────────────────────────────

test("releaseAssetName mirrors scripts/build.sh's matrix", () => {
  assert.equal(releaseAssetName("darwin", "arm64"), "tandem_darwin_arm64");
  assert.equal(releaseAssetName("darwin", "x64"), "tandem_darwin_amd64");
  assert.equal(releaseAssetName("linux", "x64"), "tandem_linux_amd64");
  assert.equal(releaseAssetName("linux", "arm64"), "tandem_linux_arm64");
  assert.equal(releaseAssetName("win32", "x64"), "tandem_windows_amd64.exe");
  assert.equal(releaseAssetName("win32", "arm64"), "tandem_windows_arm64.exe");
  assert.throws(() => releaseAssetName("freebsd", "x64"), /No prebuilt Tandem binary/);
  assert.throws(() => releaseAssetName("linux", "ia32"), /No prebuilt Tandem binary/);
});

test("releaseAssetUrl: v<version> tag, or latest", () => {
  assert.equal(
    releaseAssetUrl("3.0.0", "tandem_linux_amd64"),
    "https://github.com/jaximus808/tandam/releases/download/v3.0.0/tandem_linux_amd64"
  );
  assert.equal(
    releaseAssetUrl("latest", "SHA256SUMS"),
    "https://github.com/jaximus808/tandam/releases/latest/download/SHA256SUMS"
  );
});

test("digestFromSums parses shasum output, including the binary-mode `*` marker", () => {
  const sums = [
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  tandem_darwin_arm64",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb *tandem_windows_amd64.exe",
  ].join("\n");
  assert.equal(digestFromSums(sums, "tandem_darwin_arm64"), "a".repeat(64));
  assert.equal(digestFromSums(sums, "tandem_windows_amd64.exe"), "b".repeat(64));
  assert.equal(digestFromSums(sums, "tandem_linux_amd64"), null);
});

// ── Binary resolution ────────────────────────────────────────────────────────

test("resolveBinary: TANDEM_BIN, then PATH, then the cache — no download when any is present", async () => {
  const viaEnv = fake({ env: { TANDEM_BIN: "/opt/tandem" } });
  viaEnv.files.set("/opt/tandem", "bin");
  assert.deepEqual(await resolveBinary(viaEnv.deps, "/d"), { path: "/opt/tandem", source: "env" });
  assert.equal(viaEnv.fetches.length, 0);

  const missingEnv = fake({ env: { TANDEM_BIN: "/nope" } });
  await assert.rejects(resolveBinary(missingEnv.deps, "/d"), /TANDEM_BIN=\/nope does not exist/);

  const viaPath = fake({ onPath: "/usr/local/bin/tandem" });
  assert.deepEqual(await resolveBinary(viaPath.deps, "/d"), { path: "/usr/local/bin/tandem", source: "path" });

  const viaCache = fake();
  viaCache.files.set(cachedBinaryPath("/d", "darwin"), "bin");
  assert.deepEqual(await resolveBinary(viaCache.deps, "/d"), { path: join("/d", "bin", BINARY_NAME), source: "cache" });
  assert.equal(viaCache.fetches.length, 0);
});

test("cachedBinaryPath: <dataDir>/bin/tandem, .exe on windows", () => {
  assert.equal(cachedBinaryPath("/d", "darwin"), join("/d", "bin", "tandem"));
  assert.equal(cachedBinaryPath("/d", "win32"), join("/d", "bin", "tandem.exe"));
});

test("downloadBinary: verified download lands at <dataDir>/bin/tandem with mode 0755", async () => {
  const bin = new TextEncoder().encode("ELF-ish");
  const file = "tandem_darwin_arm64";
  const f = fake({
    responses: {
      [releaseAssetUrl("3.0.0", file)]: bytesResponse(bin),
      [releaseAssetUrl("3.0.0", "SHA256SUMS")]: textResponse(`${sha256Hex(bin)}  ${file}\n`),
    },
  });
  const dest = cachedBinaryPath("/d", "darwin");
  assert.equal(await downloadBinary(f.deps, dest), dest);
  assert.deepEqual(f.files.get(dest), bin);
  assert.equal(f.modes.get(dest), 0o755);
  assert.ok(f.dirs.has(join(dest, "..")), "creates bin/");
  assert.equal(f.err.filter((l) => /warning/.test(l)).length, 0);
});

test("downloadBinary: a 404 on this version's tag falls back to the latest release", async () => {
  const bin = new TextEncoder().encode("bin");
  const file = "tandem_linux_amd64";
  const f = fake({
    platform: "linux",
    arch: "x64",
    responses: {
      [releaseAssetUrl("latest", file)]: bytesResponse(bin),
      [releaseAssetUrl("latest", "SHA256SUMS")]: textResponse(`${sha256Hex(bin)}  ${file}\n`),
    },
  });
  await downloadBinary(f.deps, "/d/bin/tandem");
  assert.deepEqual(f.fetches.slice(0, 2), [releaseAssetUrl("3.0.0", file), releaseAssetUrl("latest", file)]);
  assert.match(f.err.join("\n"), /latest release/);
});

test("downloadBinary: SHA256 mismatch refuses to install; missing sums only warns", async () => {
  const bin = new TextEncoder().encode("bin");
  const file = "tandem_darwin_arm64";
  const bad = fake({
    responses: {
      [releaseAssetUrl("3.0.0", file)]: bytesResponse(bin),
      [releaseAssetUrl("3.0.0", "SHA256SUMS")]: textResponse(`${"0".repeat(64)}  ${file}\n`),
    },
  });
  await assert.rejects(downloadBinary(bad.deps, "/d/bin/tandem"), /SHA256 mismatch/);
  assert.equal(bad.files.size, 0, "nothing written");

  const noSums = fake({ responses: { [releaseAssetUrl("3.0.0", file)]: bytesResponse(bin) } });
  await downloadBinary(noSums.deps, "/d/bin/tandem");
  assert.ok(noSums.files.has("/d/bin/tandem"));
  assert.match(noSums.err.join("\n"), /no SHA256SUMS/);

  const notListed = fake({
    responses: {
      [releaseAssetUrl("3.0.0", file)]: bytesResponse(bin),
      [releaseAssetUrl("3.0.0", "SHA256SUMS")]: textResponse(`${"0".repeat(64)}  tandem_linux_amd64\n`),
    },
  });
  await downloadBinary(notListed.deps, "/d/bin/tandem");
  assert.match(notListed.err.join("\n"), /does not list/);
});

test("downloadBinary: no release anywhere is a clear, actionable error", async () => {
  const f = fake();
  await assert.rejects(downloadBinary(f.deps, "/d/bin/tandem"), /Could not download tandem_darwin_arm64 .*HTTP 404.*TANDEM_BIN/);
  const empty = fake({ responses: { [releaseAssetUrl("3.0.0", "tandem_darwin_arm64")]: bytesResponse(new Uint8Array()) } });
  await assert.rejects(downloadBinary(empty.deps, "/d/bin/tandem"), /empty/);
});

// ── Server lifecycle ─────────────────────────────────────────────────────────

test("ensureServer: reuses a server already answering on the port — never spawns a second", async () => {
  const f = fake({ onPath: "/usr/local/bin/tandem" });
  f.healthy = true;
  const r = await ensureServer(f.deps, { port: 7891, dataDir: "/d" });
  assert.equal(r.started, false);
  assert.equal(r.url, "http://127.0.0.1:7891");
  assert.equal(r.version, "0.1.0");
  assert.equal(f.spawned.length, 0);
});

test("ensureServer: spawns detached with PORT + TANDEM_DATA_DIR, logs to logs/tandem.log, records the pid, waits for /healthz", async () => {
  const f = fake({ onPath: "/usr/local/bin/tandem", env: { HOME: "/h", PATH: "/bin" } });
  f.healthyAfter = 4; // first probe (pre-spawn) fails, then a few polls
  const r = await ensureServer(f.deps, { port: 8000, dataDir: "/d" });
  assert.equal(r.started, true);
  assert.equal(r.pid, 4242);
  assert.equal(r.binary, "/usr/local/bin/tandem");
  assert.equal(f.spawned.length, 1);
  assert.equal(f.spawned[0].env.PORT, "8000");
  assert.equal(f.spawned[0].env.TANDEM_DATA_DIR, "/d");
  assert.equal(f.spawned[0].env.HOME, "/h", "inherits our env");
  assert.equal(f.spawned[0].logPath, join("/d", LOG_FILE));
  assert.ok(f.dirs.has(join("/d", "logs")));
  assert.equal(f.files.get(join("/d", PID_FILE)), "4242\n");
});

test("ensureServer: a binary that dies (or never answers) fails with the log path", async () => {
  const f = fake({ onPath: "/usr/local/bin/tandem" });
  const alive = (f.deps as UpDeps & { _alive: Set<number> })._alive;
  f.deps.spawnDetached = (bin, env, logPath) => {
    f.spawned.push({ bin, env, logPath });
    return 99; // never added to alive → "died"
  };
  void alive;
  await assert.rejects(ensureServer(f.deps, { port: 7891, dataDir: "/d" }), /did not answer .*logs\/tandem\.log/);

  // Alive but silent: bounded by HEALTH_TIMEOUT_MS via the injected clock-free
  // sleep — the loop must still terminate.
  const g = fake({ onPath: "/usr/local/bin/tandem" });
  let now = 0;
  const realNow = Date.now;
  Date.now = () => (now += HEALTH_TIMEOUT_MS / 4);
  try {
    await assert.rejects(ensureServer(g.deps, { port: 7891, dataDir: "/d" }), /did not answer/);
  } finally {
    Date.now = realNow;
  }
});

test("serverEnv / boardUrl", () => {
  assert.deepEqual(serverEnv({ A: "1" }, { port: 7891, dataDir: "/d" }), { A: "1", PORT: "7891", TANDEM_DATA_DIR: "/d" });
  assert.equal(boardUrl(7891), "http://127.0.0.1:7891/");
  assert.equal(boardUrl(7891, "AB3XK9QZ"), "http://127.0.0.1:7891/c/AB3XK9QZ");
});

// ── Commands ─────────────────────────────────────────────────────────────────

test("runUp: starts, prints the board URL + stop hint, opens the browser at the dashboard or --code", async () => {
  const f = fake({ onPath: "/usr/local/bin/tandem" });
  f.healthyAfter = 2;
  const code = await runUp(parseUpArgs(["--data-dir", "/d"], {}, "up", "/h"), f.deps);
  assert.equal(code, 0);
  assert.deepEqual(f.opened, ["http://127.0.0.1:7891/"]);
  const text = f.out.join("\n");
  assert.match(text, /Tandem is up \(v0\.1\.0\) — pid 4242/);
  assert.match(text, /Board {3}http:\/\/127\.0\.0\.1:7891\//);
  assert.match(text, /tandem-mcp down/);

  const g = fake({ onPath: "/usr/local/bin/tandem" });
  g.healthy = true;
  await runUp(parseUpArgs(["--code", "ab3xk9qz", "--port", "8000"], {}, "up", "/h"), g.deps);
  assert.deepEqual(g.opened, ["http://127.0.0.1:8000/c/AB3XK9QZ"]);
  assert.match(g.out.join("\n"), /already running/);
  assert.equal(g.spawned.length, 0);

  const h = fake({ onPath: "/usr/local/bin/tandem" });
  h.healthy = true;
  await runUp(parseUpArgs(["--no-open"], {}, "up", "/h"), h.deps);
  assert.deepEqual(h.opened, []);
});

test("runUp --help prints the contract (flags + env vars) and touches nothing", async () => {
  const f = fake();
  assert.equal(await runUp(parseUpArgs(["--help"], {}, "up", "/h"), f.deps), 0);
  const text = f.out.join("\n");
  for (const needle of ["--port", "--data-dir", "--code", "--no-open", "--foreground", "TANDEM_BIN", "TANDEM_DATA_DIR", "TANDEM_AUTOSTART", "TANDEM_AUTOSTART_OPEN", "SHA256SUMS"]) {
    assert.match(text, new RegExp(needle.replace(/[-]/g, "\\-")), needle);
  }
  assert.equal(f.spawned.length, 0);
  assert.equal(f.fetches.length, 0);
});

test("runDown: stops the recorded pid and clears the pid file; dead pid is 'not running'; foreign server is refused", async () => {
  const f = fake();
  (f.deps as UpDeps & { _alive: Set<number> })._alive.add(555);
  f.files.set(join("/d", PID_FILE), "555\n");
  assert.equal(await runDown(parseUpArgs(["--data-dir", "/d"], {}, "down", "/h"), f.deps), 0);
  assert.deepEqual(f.killed, [555]);
  assert.equal(f.files.has(join("/d", PID_FILE)), false);
  assert.match(f.out.join("\n"), /Stopped Tandem \(pid 555\)/);

  const dead = fake();
  dead.files.set(join("/d", PID_FILE), "555\n");
  assert.equal(await runDown(parseUpArgs(["--data-dir", "/d"], {}, "down", "/h"), dead.deps), 0);
  assert.match(dead.out.join("\n"), /not running/);
  assert.equal(dead.files.has(join("/d", PID_FILE)), false, "stale pid file is cleaned up");

  const foreign = fake();
  foreign.healthy = true;
  assert.equal(await runDown(parseUpArgs(["--data-dir", "/d"], {}, "down", "/h"), foreign.deps), 1);
  assert.match(foreign.out.join("\n"), /not started by/);
  assert.deepEqual(foreign.killed, []);
});

// ── MCP auto-start ───────────────────────────────────────────────────────────

test("isLocalApiUrl: only loopback hosts qualify", () => {
  assert.equal(isLocalApiUrl("http://127.0.0.1:7891"), true);
  assert.equal(isLocalApiUrl("http://localhost:7891"), true);
  assert.equal(isLocalApiUrl("http://[::1]:7891"), true);
  assert.equal(isLocalApiUrl("https://tandem.example.com"), false);
  assert.equal(isLocalApiUrl("http://tandem:7891"), false);
  assert.equal(isLocalApiUrl("not a url"), false);
});

test("autoStartEnabled: on unless TANDEM_AUTOSTART is 0/false", () => {
  assert.equal(autoStartEnabled({}), true);
  assert.equal(autoStartEnabled({ TANDEM_AUTOSTART: "1" }), true);
  assert.equal(autoStartEnabled({ TANDEM_AUTOSTART: "0" }), false);
  assert.equal(autoStartEnabled({ TANDEM_AUTOSTART: "false" }), false);
});

test("autoStartLocal: starts the local server quietly (no browser unless TANDEM_AUTOSTART_OPEN=1), never for a remote API_URL", async () => {
  const f = fake({ onPath: "/usr/local/bin/tandem", env: { HOME: "/h", TANDEM_DATA_DIR: "/d" } });
  f.healthyAfter = 2;
  assert.equal(await autoStartLocal("http://127.0.0.1:7891", f.deps), true);
  assert.equal(f.spawned.length, 1);
  assert.equal(f.spawned[0].env.TANDEM_DATA_DIR, "/d");
  assert.deepEqual(f.opened, []);
  assert.match(f.err.join("\n"), /started the local Tandem server \(pid 4242\)/);

  const opens = fake({ onPath: "/usr/local/bin/tandem", env: { TANDEM_DATA_DIR: "/d", TANDEM_AUTOSTART_OPEN: "1" } });
  opens.healthyAfter = 2;
  await autoStartLocal("http://127.0.0.1:8000", opens.deps);
  assert.deepEqual(opens.opened, ["http://127.0.0.1:8000/"]);

  const remote = fake({ onPath: "/usr/local/bin/tandem" });
  assert.equal(await autoStartLocal("https://tandem.example.com", remote.deps), false);
  assert.equal(remote.spawned.length, 0);

  const off = fake({ onPath: "/usr/local/bin/tandem", env: { TANDEM_AUTOSTART: "0" } });
  assert.equal(await autoStartLocal("http://127.0.0.1:7891", off.deps), false);
  assert.equal(off.spawned.length, 0);

  // A failed start is reported, not thrown — the tool call then fails with the
  // plain "nothing listening" error.
  const broken = fake({ env: { TANDEM_DATA_DIR: "/d" } });
  assert.equal(await autoStartLocal("http://127.0.0.1:7891", broken.deps), false);
  assert.match(broken.err.join("\n"), /could not auto-start/);
});

test("isConnectionRefused: undici's cause.code, a bare code, the message, or the wrapped flag", () => {
  assert.equal(isConnectionRefused(Object.assign(new TypeError("fetch failed"), { cause: { code: "ECONNREFUSED" } })), true);
  assert.equal(isConnectionRefused(Object.assign(new Error("x"), { code: "ECONNREFUSED" })), true);
  assert.equal(isConnectionRefused(new Error("connect ECONNREFUSED 127.0.0.1:7891")), true);
  assert.equal(isConnectionRefused(Object.assign(new Error("x"), { refused: true })), true);
  assert.equal(isConnectionRefused(Object.assign(new TypeError("fetch failed"), { cause: { code: "ENOTFOUND" } })), false);
  assert.equal(isConnectionRefused(null), false);
});

test("Gateway: a refused request triggers autoStart ONCE, then retries; a second refusal does not re-spawn", async () => {
  const real = globalThis.fetch;
  let refuse = true;
  const calls: string[] = [];
  globalThis.fetch = (async (input: string | URL | Request) => {
    calls.push(String(input));
    if (refuse) throw Object.assign(new TypeError("fetch failed"), { cause: { code: "ECONNREFUSED" } });
    return new Response(JSON.stringify({ token: "t", canvasId: "c1", canvasName: "C", canvasCode: "AB3XK9QZ" }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  }) as typeof globalThis.fetch;

  let starts = 0;
  const gw = new Gateway({
    apiUrl: "http://127.0.0.1:7891",
    autoStart: async () => {
      starts++;
      refuse = false; // the server is now "up"
      return true;
    },
  });
  try {
    const session = await gw.connectWithCode("AB3XK9QZ");
    assert.equal(session.canvasCode, "AB3XK9QZ");
    assert.equal(starts, 1);
    assert.equal(calls.length, 2, "one refused attempt, one retry");

    // Later refusal (server died): no second spawn attempt, plain error.
    refuse = true;
    await assert.rejects(gw.connectWithCode("AB3XK9QZ"), /Nothing is listening at http:\/\/127\.0\.0\.1:7891.*tandem-mcp up/);
    assert.equal(starts, 1);
  } finally {
    globalThis.fetch = real;
  }
});

test("Gateway: without an autoStart hook (self-hosted / sidecar) a refusal is just the error", async () => {
  const real = globalThis.fetch;
  globalThis.fetch = (async () => {
    throw Object.assign(new TypeError("fetch failed"), { cause: { code: "ECONNREFUSED" } });
  }) as typeof globalThis.fetch;
  try {
    const gw = new Gateway({ apiUrl: "http://tandem:7891" });
    await assert.rejects(gw.connectWithCode("AB3XK9QZ"), /Nothing is listening at http:\/\/tandem:7891/);
  } finally {
    globalThis.fetch = real;
  }
});
