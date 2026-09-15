/**
 * `tandem-mcp up` / `open` / `down` — the launcher for the LOCAL Tandem binary.
 *
 * Tandem is a locally run tool: one Go binary (`tandem`) with the web bundle
 * embedded and a SQLite database in the data dir. This module is how the binary
 * gets onto the machine and into a running state, for two callers:
 *
 *   - a human:  `npx @jaximus/tandem-mcp up`   → server running, board in browser
 *   - an agent: the stdio MCP server hits ECONNREFUSED on the local API and runs
 *                the same `ensureServer` once before retrying (see gateway.ts
 *                `autoStart`), so "connect to the canvas" works on a cold machine.
 *
 * Binary resolution, in order: `TANDEM_BIN`, `tandem` on PATH, the cached
 * download at `<dataDir>/bin/tandem[.exe]`, else download the platform's raw
 * binary from the GitHub Release matching this package's version (falling back
 * to `latest` when that tag has no binary) and verify it against SHA256SUMS.
 *
 * Data dir layout the launcher writes (default `~/.tandem`, `TANDEM_DATA_DIR`):
 *   bin/tandem[.exe]   cached download
 *   logs/tandem.log    stdout+stderr of a detached server
 *   tandem.pid         pid of the server `up` started (what `down` stops)
 * The binary itself keeps its database and images in the same dir.
 *
 * Everything but `nodeDeps()` is pure over the injected `UpDeps`, so the flow
 * is unit-testable without a network, a disk, or a spawned process.
 */

import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { delimiter, join, resolve } from "node:path";

// ── Constants ────────────────────────────────────────────────────────────────

export const DEFAULT_PORT = 7891;
export const DEFAULT_DATA_DIR_NAME = ".tandem";
export const BINARY_NAME = "tandem";
export const RELEASE_REPO = "jaximus808/tandam";
export const PID_FILE = "tandem.pid";
export const LOG_FILE = join("logs", "tandem.log");
/** How long `up` waits for /healthz after spawning before giving up. */
export const HEALTH_TIMEOUT_MS = 10_000;
export const HEALTH_POLL_MS = 250;

// ── Args ─────────────────────────────────────────────────────────────────────

export interface UpOptions {
  port: number;
  dataDir: string;
  /** Open the browser once the server answers. */
  open: boolean;
  /** Open at /c/<CODE> rather than the dashboard. */
  code?: string;
  /** Run attached, logs on stderr, instead of detached. */
  foreground: boolean;
  help: boolean;
}

export class UpUsageError extends Error {}

/** `TANDEM_DATA_DIR`, else `~/.tandem`. */
export function defaultDataDir(env: NodeJS.ProcessEnv, home: string = homedir()): string {
  const fromEnv = env.TANDEM_DATA_DIR?.trim();
  return fromEnv ? resolve(fromEnv) : join(home, DEFAULT_DATA_DIR_NAME);
}

/** `PORT`, else 7891. Unparsable values are ignored, not fatal. */
export function defaultPort(env: NodeJS.ProcessEnv): number {
  const n = Number(env.PORT);
  return Number.isInteger(n) && n > 0 && n < 65536 ? n : DEFAULT_PORT;
}

/**
 * Parse the flags shared by `up`, `open` and `down` (argv AFTER the subcommand
 * word). `open` also accepts a bare CODE positional. Unknown flags are a usage
 * error, as in `init`.
 */
export function parseUpArgs(
  argv: string[],
  env: NodeJS.ProcessEnv,
  command: "up" | "open" | "down" = "up",
  home: string = homedir()
): UpOptions {
  const opts: UpOptions = {
    port: defaultPort(env),
    dataDir: defaultDataDir(env, home),
    open: true,
    foreground: false,
    help: false,
  };

  const valueOf = (flag: string, next: string | undefined): string => {
    if (next === undefined || next.startsWith("-")) {
      throw new UpUsageError(`${flag} needs a value`);
    }
    return next;
  };

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    const eq = arg.indexOf("=");
    const flag = arg.startsWith("--") && eq > 0 ? arg.slice(0, eq) : arg;
    const inline = arg.startsWith("--") && eq > 0 ? arg.slice(eq + 1) : undefined;
    const take = (): string => (inline !== undefined ? inline : valueOf(flag, argv[++i]));

    switch (flag) {
      case "--port": {
        const n = Number(take());
        if (!Number.isInteger(n) || n <= 0 || n >= 65536) {
          throw new UpUsageError(`--port must be a port number, got "${argv[i]}"`);
        }
        opts.port = n;
        break;
      }
      case "--data-dir":
        opts.dataDir = resolve(take());
        break;
      case "--code":
        opts.code = take().trim().toUpperCase();
        break;
      case "--no-open":
        opts.open = false;
        break;
      case "--foreground":
      case "-f":
        opts.foreground = true;
        break;
      case "--help":
      case "-h":
        opts.help = true;
        break;
      default:
        // `open AB3XK9QZ` — the one positional, and only for `open`.
        if (command === "open" && !arg.startsWith("-") && !opts.code) {
          opts.code = arg.trim().toUpperCase();
          break;
        }
        throw new UpUsageError(`Unknown option "${arg}" for \`tandem-mcp ${command}\``);
    }
  }
  if (opts.code === "") delete opts.code;
  return opts;
}

// ── Release asset naming ─────────────────────────────────────────────────────

/**
 * The raw binary's file name on a GitHub Release for this platform, e.g.
 * `tandem_darwin_arm64`, `tandem_windows_amd64.exe`. Mirrors scripts/build.sh's
 * matrix; anything outside it has no binary to download.
 */
export function releaseAssetName(platform: string, arch: string): string {
  const os = platform === "darwin" || platform === "linux" || platform === "win32" ? platform : null;
  const goos = os === "win32" ? "windows" : os;
  const goarch = arch === "x64" ? "amd64" : arch === "arm64" ? "arm64" : null;
  if (!goos || !goarch) {
    throw new Error(
      `No prebuilt Tandem binary for ${platform}/${arch}. Build it from source (scripts/build.sh) and point TANDEM_BIN at it.`
    );
  }
  return `${BINARY_NAME}_${goos}_${goarch}${goos === "windows" ? ".exe" : ""}`;
}

/**
 * Download URL for a release asset. `version` is a bare semver (→ tag
 * `v<version>`) or the literal `latest`.
 */
export function releaseAssetUrl(version: string, file: string): string {
  return version === "latest"
    ? `https://github.com/${RELEASE_REPO}/releases/latest/download/${file}`
    : `https://github.com/${RELEASE_REPO}/releases/download/v${version}/${file}`;
}

/** Find `<file>`'s hex digest in a `shasum -a 256` style SHA256SUMS body. */
export function digestFromSums(sums: string, file: string): string | null {
  for (const line of sums.split("\n")) {
    const m = line.trim().match(/^([0-9a-fA-F]{64})\s+\*?(.+)$/);
    if (m && m[2].trim() === file) return m[1].toLowerCase();
  }
  return null;
}

export function sha256Hex(bytes: Uint8Array): string {
  return createHash("sha256").update(bytes).digest("hex");
}

// ── Deps ─────────────────────────────────────────────────────────────────────

export interface FetchResult {
  status: number;
  ok: boolean;
  bytes(): Promise<Uint8Array>;
  text(): Promise<string>;
}

export interface UpDeps {
  env: NodeJS.ProcessEnv;
  platform: string;
  arch: string;
  /** This package's version — the release tag it tries first. */
  version: string;
  exists(path: string): boolean;
  mkdirp(path: string): void;
  readFile(path: string): string | null;
  writeFile(path: string, data: Uint8Array | string, mode?: number): void;
  removeFile(path: string): void;
  /** First `name` on PATH, or null. */
  which(name: string): string | null;
  /** HTTP GET following redirects. Throws only on a transport failure. */
  fetch(url: string): Promise<FetchResult>;
  /** GET <base>/healthz; ok:false on any failure. Never throws. */
  health(baseUrl: string): Promise<{ ok: boolean; version?: string }>;
  /** Spawn the server detached with stdio to `logPath`; returns its pid. */
  spawnDetached(bin: string, env: NodeJS.ProcessEnv, logPath: string): number;
  /** Spawn attached, inheriting stderr; resolves with the exit code. */
  spawnForeground(bin: string, env: NodeJS.ProcessEnv): Promise<number>;
  openBrowser(url: string): void;
  alive(pid: number): boolean;
  kill(pid: number): boolean;
  sleep(ms: number): Promise<void>;
  out(line: string): void;
  err(line: string): void;
}

/** Real deps: node:fs, node:child_process, global fetch. */
export function nodeDeps(version: string): UpDeps {
  return {
    env: process.env,
    platform: process.platform,
    arch: process.arch,
    version,
    exists: (p) => existsSync(p),
    mkdirp: (p) => mkdirSync(p, { recursive: true }),
    readFile: (p) => (existsSync(p) ? readFileSync(p, "utf8") : null),
    writeFile: (p, data, mode) => {
      writeFileSync(p, data);
      if (mode !== undefined) chmodSync(p, mode);
    },
    removeFile: (p) => {
      try {
        unlinkSync(p);
      } catch {
        /* already gone */
      }
    },
    which: (name) => {
      const exts = process.platform === "win32" ? [".exe", ".cmd", ".bat", ""] : [""];
      for (const dir of (process.env.PATH ?? "").split(delimiter)) {
        if (!dir) continue;
        for (const ext of exts) {
          const candidate = join(dir, name + ext);
          if (existsSync(candidate)) return candidate;
        }
      }
      return null;
    },
    fetch: async (url) => {
      const res = await fetch(url, { redirect: "follow" });
      return {
        status: res.status,
        ok: res.ok,
        bytes: async () => new Uint8Array(await res.arrayBuffer()),
        text: () => res.text(),
      };
    },
    health: (baseUrl) => probeHealth(baseUrl),
    spawnDetached: (bin, env, logPath) => {
      const fd = openSync(logPath, "a");
      const child = spawn(bin, [], { env, detached: true, stdio: ["ignore", fd, fd] });
      child.unref();
      if (child.pid === undefined) throw new Error(`could not spawn ${bin}`);
      return child.pid;
    },
    spawnForeground: (bin, env) =>
      new Promise((resolveExit, reject) => {
        const child = spawn(bin, [], { env, stdio: ["ignore", "inherit", "inherit"] });
        child.on("error", reject);
        child.on("exit", (code) => resolveExit(code ?? 1));
        // Pass Ctrl-C through so the server shuts down gracefully.
        const forward = () => child.kill("SIGTERM");
        process.on("SIGINT", forward);
        process.on("SIGTERM", forward);
      }),
    openBrowser: (url) => openBrowser(url),
    alive: (pid) => {
      try {
        process.kill(pid, 0);
        return true;
      } catch {
        return false;
      }
    },
    kill: (pid) => {
      try {
        process.kill(pid, "SIGTERM");
        return true;
      } catch {
        return false;
      }
    },
    sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
    out: (line) => process.stdout.write(`${line}\n`),
    err: (line) => process.stderr.write(`${line}\n`),
  };
}

/** GET /healthz with a short deadline. Any failure is "not up". */
export async function probeHealth(baseUrl: string): Promise<{ ok: boolean; version?: string }> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 1500);
  try {
    const res = await fetch(`${baseUrl}/healthz`, { signal: controller.signal });
    if (!res.ok) return { ok: false };
    const body = (await res.json().catch(() => ({}))) as { version?: string };
    return { ok: true, version: body.version };
  } catch {
    return { ok: false };
  } finally {
    clearTimeout(timer);
  }
}

/** Open a URL with the platform opener. Never a dependency; failures are silent. */
export function openBrowser(url: string): void {
  const [cmd, args] =
    process.platform === "darwin"
      ? ["open", [url]]
      : process.platform === "win32"
        ? ["cmd", ["/c", "start", "", url]]
        : ["xdg-open", [url]];
  try {
    const child = spawn(cmd, args, { detached: true, stdio: "ignore" });
    child.on("error", () => {});
    child.unref();
  } catch {
    /* no opener available — the URL was printed anyway */
  }
}

// ── Binary resolution ────────────────────────────────────────────────────────

export function cachedBinaryPath(dataDir: string, platform: string): string {
  return join(dataDir, "bin", BINARY_NAME + (platform === "win32" ? ".exe" : ""));
}

/**
 * Where the server binary is, downloading it if this machine has none.
 * Returns the path and how it was found (for the report line).
 */
export async function resolveBinary(
  deps: UpDeps,
  dataDir: string
): Promise<{ path: string; source: "env" | "path" | "cache" | "download" }> {
  const fromEnv = deps.env.TANDEM_BIN?.trim();
  if (fromEnv) {
    if (!deps.exists(fromEnv)) throw new Error(`TANDEM_BIN=${fromEnv} does not exist`);
    return { path: fromEnv, source: "env" };
  }
  const onPath = deps.which(BINARY_NAME);
  if (onPath) return { path: onPath, source: "path" };

  const cached = cachedBinaryPath(dataDir, deps.platform);
  if (deps.exists(cached)) return { path: cached, source: "cache" };

  return { path: await downloadBinary(deps, cached), source: "download" };
}

/**
 * Download this platform's raw binary into `dest`. Tries the release tagged
 * with this package's version first, then `latest`; verifies against
 * SHA256SUMS when the release ships one (a mismatch is fatal — a corrupt or
 * substituted binary must never run; a MISSING sums file only warns).
 */
export async function downloadBinary(deps: UpDeps, dest: string): Promise<string> {
  const file = releaseAssetName(deps.platform, deps.arch);

  let version = deps.version;
  let res = await deps.fetch(releaseAssetUrl(version, file));
  if (res.status === 404 && version !== "latest") {
    version = "latest";
    res = await deps.fetch(releaseAssetUrl(version, file));
  }
  if (!res.ok) {
    throw new Error(
      `Could not download ${file} from the ${RELEASE_REPO} releases (HTTP ${res.status}). ` +
        `Build it from source (scripts/build.sh) and set TANDEM_BIN, or install \`tandem\` on your PATH.`
    );
  }
  const bytes = await res.bytes();
  if (bytes.byteLength === 0) throw new Error(`Downloaded ${file} is empty — refusing to install it.`);

  const sums = await deps.fetch(releaseAssetUrl(version, "SHA256SUMS")).catch(() => null);
  if (sums && sums.ok) {
    const want = digestFromSums(await sums.text(), file);
    if (want) {
      const got = sha256Hex(bytes);
      if (got !== want) {
        throw new Error(
          `SHA256 mismatch for ${file}: release says ${want}, download is ${got}. Not installing it.`
        );
      }
    } else {
      deps.err(`[tandem] warning: SHA256SUMS on the release does not list ${file}; installing unverified.`);
    }
  } else {
    deps.err(`[tandem] warning: no SHA256SUMS on the release; installing ${file} unverified.`);
  }

  deps.mkdirp(join(dest, ".."));
  deps.writeFile(dest, bytes, 0o755);
  deps.err(`[tandem] downloaded ${file} (${version === "latest" ? "latest release" : `v${version}`}) → ${dest}`);
  return dest;
}

// ── Server lifecycle ─────────────────────────────────────────────────────────

export function localUrl(port: number): string {
  return `http://127.0.0.1:${port}`;
}

export function boardUrl(port: number, code?: string): string {
  return code ? `${localUrl(port)}/c/${code}` : `${localUrl(port)}/`;
}

/** The env the spawned server sees: ours, plus the port and data dir. */
export function serverEnv(base: NodeJS.ProcessEnv, opts: Pick<UpOptions, "port" | "dataDir">): NodeJS.ProcessEnv {
  return { ...base, PORT: String(opts.port), TANDEM_DATA_DIR: opts.dataDir };
}

export interface EnsureResult {
  url: string;
  /** false when a server was already answering on the port. */
  started: boolean;
  pid?: number;
  binary?: string;
  version?: string;
}

/**
 * Make sure a Tandem server answers on `opts.port`: reuse one that already
 * does, otherwise resolve the binary, spawn it detached, and wait for /healthz.
 */
export async function ensureServer(
  deps: UpDeps,
  opts: Pick<UpOptions, "port" | "dataDir">
): Promise<EnsureResult> {
  const url = localUrl(opts.port);
  const already = await deps.health(url);
  if (already.ok) return { url, started: false, version: already.version };

  const bin = await resolveBinary(deps, opts.dataDir);
  deps.mkdirp(join(opts.dataDir, "logs"));
  const logPath = join(opts.dataDir, LOG_FILE);
  const pid = deps.spawnDetached(bin.path, serverEnv(deps.env, opts), logPath);
  deps.writeFile(join(opts.dataDir, PID_FILE), `${pid}\n`);

  const deadline = Date.now() + HEALTH_TIMEOUT_MS;
  while (Date.now() < deadline) {
    await deps.sleep(HEALTH_POLL_MS);
    const h = await deps.health(url);
    if (h.ok) return { url, started: true, pid, binary: bin.path, version: h.version };
    if (!deps.alive(pid)) break;
  }
  throw new Error(
    `Tandem (${bin.path}) did not answer on ${url} within ${HEALTH_TIMEOUT_MS / 1000}s — see ${logPath}.`
  );
}

/** pid recorded by `up`, if the process is still alive. */
export function recordedPid(deps: UpDeps, dataDir: string): number | null {
  const raw = deps.readFile(join(dataDir, PID_FILE));
  const pid = Number(raw?.trim());
  if (!Number.isInteger(pid) || pid <= 0) return null;
  return deps.alive(pid) ? pid : null;
}

// ── Commands ─────────────────────────────────────────────────────────────────

export function upHelp(): string {
  return [
    "tandem-mcp up — start the local Tandem server and open the board.",
    "",
    "Usage:",
    "  npx @jaximus/tandem-mcp up [options]",
    "  npx @jaximus/tandem-mcp open [CODE]     open the board (starting the server if needed)",
    "  npx @jaximus/tandem-mcp down            stop the server `up` started",
    "",
    "Options:",
    `  --port <n>         Port to serve on. Default: $PORT or ${DEFAULT_PORT}.`,
    `  --data-dir <path>  Where the database, images and logs live. Default: $TANDEM_DATA_DIR or ~/${DEFAULT_DATA_DIR_NAME}.`,
    "  --code <CODE>      Open the browser on this canvas instead of the dashboard.",
    "  --no-open          Don't open the browser.",
    "  -f, --foreground   Run attached with logs on stderr (Ctrl-C stops it) instead of detached.",
    "  -h, --help         Show this help.",
    "",
    "The server binary is found via TANDEM_BIN, then `tandem` on PATH, then the cached",
    `download in <data-dir>/bin — else it is downloaded from the ${RELEASE_REPO} GitHub`,
    "release matching this package's version (or the latest one) and verified against",
    "its SHA256SUMS. A server already answering on the port is reused, never duplicated.",
    "",
    "Environment:",
    "  TANDEM_BIN              Path to a `tandem` binary to use instead of downloading.",
    "  TANDEM_DATA_DIR         Data dir (see --data-dir).",
    "  TANDEM_AUTOSTART        Set to 0 to stop the MCP server from starting the backend itself",
    "                          when a tool call finds nothing listening locally.",
    "  TANDEM_AUTOSTART_OPEN   Set to 1 to also open the browser when the MCP server auto-starts it.",
  ].join("\n");
}

/** `tandem-mcp up`. Returns the exit code. */
export async function runUp(opts: UpOptions, deps: UpDeps): Promise<number> {
  if (opts.help) {
    deps.out(upHelp());
    return 0;
  }
  deps.mkdirp(opts.dataDir);

  if (opts.foreground) {
    const url = localUrl(opts.port);
    if ((await deps.health(url)).ok) {
      deps.err(`Tandem is already running at ${url} — not starting a second one.`);
      if (opts.open) deps.openBrowser(boardUrl(opts.port, opts.code));
      return 0;
    }
    const bin = await resolveBinary(deps, opts.dataDir);
    deps.err(`[tandem] running ${bin.path} on ${url} (data: ${opts.dataDir}) — Ctrl-C to stop`);
    if (opts.open) {
      // Open once it answers; don't block the attached server on the poll.
      void (async () => {
        const deadline = Date.now() + HEALTH_TIMEOUT_MS;
        while (Date.now() < deadline) {
          await deps.sleep(HEALTH_POLL_MS);
          if ((await deps.health(url)).ok) {
            deps.openBrowser(boardUrl(opts.port, opts.code));
            return;
          }
        }
      })();
    }
    return deps.spawnForeground(bin.path, serverEnv(deps.env, opts));
  }

  const result = await ensureServer(deps, opts);
  const target = boardUrl(opts.port, opts.code);
  deps.out("");
  deps.out(
    result.started
      ? `  Tandem is up${result.version ? ` (v${result.version})` : ""} — pid ${result.pid}, data in ${opts.dataDir}`
      : `  Tandem is already running${result.version ? ` (v${result.version})` : ""} — reusing it`
  );
  deps.out(`  Board   ${target}`);
  if (result.started) deps.out(`  Logs    ${join(opts.dataDir, LOG_FILE)}`);
  deps.out(`  Stop    npx @jaximus/tandem-mcp down`);
  deps.out("");
  if (opts.open) deps.openBrowser(target);
  return 0;
}

/** `tandem-mcp open [CODE]` — `up` with the browser always opened. */
export async function runOpen(opts: UpOptions, deps: UpDeps): Promise<number> {
  if (opts.help) {
    deps.out(upHelp());
    return 0;
  }
  return runUp({ ...opts, open: true, foreground: false }, deps);
}

/** `tandem-mcp down` — stop the server `up` recorded in the pid file. */
export async function runDown(opts: UpOptions, deps: UpDeps): Promise<number> {
  if (opts.help) {
    deps.out(upHelp());
    return 0;
  }
  const pidPath = join(opts.dataDir, PID_FILE);
  const pid = recordedPid(deps, opts.dataDir);
  if (pid === null) {
    deps.removeFile(pidPath);
    const url = localUrl(opts.port);
    if ((await deps.health(url)).ok) {
      deps.out(`Something answers on ${url}, but it was not started by \`tandem-mcp up\` (no live pid in ${pidPath}) — stop it yourself.`);
      return 1;
    }
    deps.out("Tandem is not running.");
    return 0;
  }
  if (!deps.kill(pid)) {
    deps.out(`Could not signal pid ${pid} — stop it yourself.`);
    return 1;
  }
  deps.removeFile(pidPath);
  deps.out(`Stopped Tandem (pid ${pid}).`);
  return 0;
}

// ── MCP auto-start ───────────────────────────────────────────────────────────

/** Is this API base the local machine? Only then does the gateway start it. */
export function isLocalApiUrl(apiUrl: string): boolean {
  try {
    const host = new URL(apiUrl).hostname;
    return host === "127.0.0.1" || host === "localhost" || host === "::1" || host === "[::1]";
  } catch {
    return false;
  }
}

/** `TANDEM_AUTOSTART` — on unless exactly "0" / "false". */
export function autoStartEnabled(env: NodeJS.ProcessEnv): boolean {
  const v = (env.TANDEM_AUTOSTART ?? "").trim().toLowerCase();
  return v !== "0" && v !== "false";
}

/**
 * The gateway's hook: bring the local server up once, quietly (no browser
 * unless TANDEM_AUTOSTART_OPEN=1), so a tool call that found nothing listening
 * can be retried. Returns true when a server is now answering.
 */
export async function autoStartLocal(apiUrl: string, deps: UpDeps): Promise<boolean> {
  if (!isLocalApiUrl(apiUrl) || !autoStartEnabled(deps.env)) return false;
  const port = Number(new URL(apiUrl).port) || DEFAULT_PORT;
  const dataDir = defaultDataDir(deps.env);
  try {
    deps.mkdirp(dataDir);
    const result = await ensureServer(deps, { port, dataDir });
    if (result.started) {
      deps.err(`[tandem] started the local Tandem server (pid ${result.pid}) at ${result.url} — data in ${dataDir}`);
      const open = (deps.env.TANDEM_AUTOSTART_OPEN ?? "").trim() === "1";
      if (open) deps.openBrowser(boardUrl(port));
    }
    return true;
  } catch (err) {
    deps.err(`[tandem] could not auto-start the local server: ${err instanceof Error ? err.message : err}`);
    return false;
  }
}
