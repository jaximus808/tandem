/**
 * Tandem MCP Gateway
 *
 * Spawned by an MCP-aware agent (Claude Code, Cursor, Codex, OpenAI Agents
 * SDK, custom orchestrators) as a stdio process. The canvas binding is
 * established at runtime: the agent calls the `canvas_connect` tool with a
 * canvas code, the gateway exchanges it for a JWT, then proxies all
 * subsequent tool calls to the Tandem HTTP API.
 *
 * Example MCP config (npx form):
 *   {
 *     "mcpServers": {
 *       "tandem": {
 *         "command": "npx",
 *         "args": ["-y", "@jaximus/tandem-mcp"]
 *       }
 *     }
 *   }
 *
 * Defaults to the LOCAL Tandem server (http://127.0.0.1:7891). `tandem-mcp up`
 * starts it, and the MCP server starts it itself the first time a tool call
 * finds nothing listening (up.ts). Set the API_URL env var to point at a
 * self-hosted instance instead.
 */

import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { Gateway } from "./gateway.js";
import { createTandemServer, VERSION, DEFAULT_API_URL } from "./server.js";
import { installSessionSummary } from "./trace.js";
import {
  CANVAS_CODE_ENV,
  fsDeps,
  InitUsageError,
  parseInitArgs,
  runInit,
  type CanvasHandle,
} from "./init.js";
import { ListenUsageError, listenHelp, parseListenArgs, runListen } from "./listen/index.js";
import {
  UpUsageError,
  autoStartLocal,
  isLocalApiUrl,
  nodeDeps,
  parseUpArgs,
  runDown,
  runOpen,
  runUp,
} from "./up.js";

function printHelp() {
  process.stdout.write(
    `@jaximus/tandem-mcp ${VERSION} — MCP server for Tandem.\n` +
      `\n` +
      `Usage:\n` +
      `  tandem-mcp                 Run as an MCP stdio server (default).\n` +
      `  tandem-mcp up              Start the local Tandem server (downloading the\n` +
      `                             binary on first run) and open the board.\n` +
      `                             (\`tandem-mcp up --help\` for its options.)\n` +
      `  tandem-mcp open [CODE]     Open the board — or one canvas — in the browser,\n` +
      `                             starting the server first if it isn't running.\n` +
      `  tandem-mcp down            Stop the server \`up\` started.\n` +
      `  tandem-mcp init            Set up this project: create a canvas, write\n` +
      `                             .mcp.json, print the agent snippet.\n` +
      `                             (\`tandem-mcp init --help\` for its options.)\n` +
      `  tandem-mcp listen          Run a command when work is approved: a local\n` +
      `                             webhook listener that launches your\n` +
      `                             orchestrator. Needs --exec.\n` +
      `                             (\`tandem-mcp listen --help\` for its options.)\n` +
      `  tandem-mcp --full-tools    Also advertise the full CRUD tool surface\n` +
      `                             (documents, notes, roadmap items, raw\n` +
      `                             actions, …) alongside\n` +
      `                             the default intent facade.\n` +
      `  tandem-mcp --version, -v   Print version and exit.\n` +
      `  tandem-mcp --help, -h      Show this help.\n` +
      `\n` +
      `Environment:\n` +
      `  API_URL                    Tandem API base URL. Default: the local server,\n` +
      `                             ${DEFAULT_API_URL}. Set it only for a\n` +
      `                             self-hosted instance.\n` +
      `  TANDEM_DATA_DIR            Where the local server keeps its database,\n` +
      `                             images and logs. Default: ~/.tandem\n` +
      `  TANDEM_VERSION             Server release to download (e.g. 0.1.0).\n` +
      `                             Default: latest.\n` +
      `  TANDEM_BIN                 Path to a \`tandem\` binary to run instead of\n` +
      `                             the downloaded one.\n` +
      `  TANDEM_AUTOSTART           Set to 0 to stop the MCP server from starting\n` +
      `                             the local backend itself when nothing is\n` +
      `                             listening. Default: on.\n` +
      `  TANDEM_AUTOSTART_OPEN      Set to 1 to also open the browser when the MCP\n` +
      `                             server auto-starts the backend.\n` +
      `  ${CANVAS_CODE_ENV}       This project's canvas code (written by\n` +
      `                             \`init\`). Used as the default for canvas_connect.\n` +
      `  TANDEM_FULL_TOOLS          Set to 1 for the same effect as --full-tools.\n` +
      `  TANDEM_TOKEN               Personal access token for a SELF-HOSTED Tandem\n` +
      `                             running with sign-in (TANDEM_AUTH=google): lets\n` +
      `                             Claude act as you on private / shared canvases.\n` +
      `                             Mint one at <API_URL>/me. Not needed locally.\n` +
      `  MCP_TRACE                  Per-tool-call timing on stderr plus a session\n` +
      `                             summary on exit. 1 = human-readable, json =\n` +
      `                             one JSON object per line. Off by default.\n` +
      `\n` +
      `This binary is normally spawned by an MCP client (Claude Code, Cursor,\n` +
      `Codex, OpenAI Agents SDK, …) over stdio. See:\n` +
      `  https://github.com/jaximus808/tandam#readme\n`
  );
}

const cliArgs = process.argv.slice(2);
// Subcommands — `up` / `open` / `down` (the local server), `init` and `listen`.
// Everything else (including no args at all) keeps the stdio-MCP-server default
// untouched.
const INIT_MODE = cliArgs[0] === "init";
const LISTEN_MODE = cliArgs[0] === "listen";
const UP_MODE = cliArgs[0] === "up";
const OPEN_MODE = cliArgs[0] === "open";
const DOWN_MODE = cliArgs[0] === "down";
const LAUNCHER_MODE = UP_MODE || OPEN_MODE || DOWN_MODE;
const SUBCOMMAND = INIT_MODE || LISTEN_MODE || LAUNCHER_MODE;
if (!SUBCOMMAND && (cliArgs.includes("--version") || cliArgs.includes("-v"))) {
  process.stdout.write(`@jaximus/tandem-mcp ${VERSION}\n`);
  process.exit(0);
}
if (!SUBCOMMAND && (cliArgs.includes("--help") || cliArgs.includes("-h"))) {
  printHelp();
  process.exit(0);
}
// Opt in to the full CRUD surface on top of the default intent facade. The flag
// is an explicit override; without it, TANDEM_FULL_TOOLS decides (see server.ts).
const FULL_TOOLS = cliArgs.includes("--full-tools") ? true : undefined;

// Which server binary `up` downloads. The gateway (this package) and the Go
// binary are separate version lines — a gateway bump is not a server release —
// so the default is the newest server release; TANDEM_VERSION pins one
// (e.g. 0.1.0).
const BINARY_VERSION = process.env.TANDEM_VERSION?.trim() || "latest";

const apiUrlFromEnv = process.env.API_URL;
const API_URL = (apiUrlFromEnv ?? DEFAULT_API_URL).replace(/\/$/, "");
// Public origin for share/claim links. Defaults to the API base, which is
// correct for the common stdio case (API_URL = public domain) and for local
// dev (links point at the same local instance the canvas lives in).
const WEB_URL = (process.env.PUBLIC_URL ?? API_URL).replace(/\/$/, "");
// Optional personal access token, for a SELF-HOSTED Tandem running with sign-in.
// Set it to let Claude act as you on your private / shared canvases — mint one
// at <API_URL>/me. Locally there are no accounts and it is never needed.
const USER_TOKEN = process.env.TANDEM_TOKEN?.trim() || undefined;
// Optional per-request timeout override (ms) for calls to the Tandem API.
// Defaults to 15000 in the Gateway when unset or unparsable.
const requestTimeoutFromEnv = Number(process.env.REQUEST_TIMEOUT_MS);
const REQUEST_TIMEOUT_MS =
  Number.isFinite(requestTimeoutFromEnv) && requestTimeoutFromEnv > 0
    ? requestTimeoutFromEnv
    : undefined;

const gateway = new Gateway({
  apiUrl: API_URL,
  webUrl: WEB_URL,
  userToken: USER_TOKEN,
  requestTimeoutMs: REQUEST_TIMEOUT_MS,
  // Local-first: when the API is this machine and nothing is listening, the
  // first tool call (or `init`) starts the server rather than failing. Only
  // ever wired for a loopback API_URL — a self-hosted address is never spawned.
  autoStart: isLocalApiUrl(API_URL) ? () => autoStartLocal(API_URL, nodeDeps(BINARY_VERSION)) : undefined,
});

/**
 * `up` / `open` / `down`: the local server's lifecycle. Pure CLI — shares the
 * data-dir / port defaults with the auto-start path above so both agree on
 * which server "the local one" is.
 */
async function runLauncherCommand(): Promise<number> {
  const command = UP_MODE ? "up" : OPEN_MODE ? "open" : "down";
  const opts = parseUpArgs(cliArgs.slice(1), process.env, command);
  const deps = nodeDeps(BINARY_VERSION);
  if (UP_MODE) return runUp(opts, deps);
  if (OPEN_MODE) return runOpen(opts, deps);
  return runDown(opts, deps);
}

/**
 * `init` (TDM-33): one command from nothing to a wired-up project. Uses the
 * same Gateway plumbing as the server path, so it inherits API_URL, the
 * personal access token, and the claim-link behaviour of canvas_create.
 */
async function runInitCommand(): Promise<number> {
  const toHandle = (
    s: { canvasCode: string; canvasName: string; claimToken?: string }
  ): CanvasHandle => ({
    code: s.canvasCode,
    name: s.canvasName,
    url: gateway.canvasUrl(s.canvasCode),
    claimUrl: s.claimToken ? gateway.canvasClaimUrl(s.canvasCode, s.claimToken) : undefined,
  });

  const opts = parseInitArgs(cliArgs.slice(1), process.cwd());
  return runInit(opts, {
    ...fsDeps(),
    webUrl: WEB_URL,
    // Only pin API_URL into .mcp.json when it isn't the package default —
    // a local/self-hosted setup needs it, the hosted one must stay unpinned.
    apiUrl: API_URL === DEFAULT_API_URL ? undefined : API_URL,
    createCanvas: async (name) => toHandle(await gateway.createCanvas(name)),
    connectCanvas: async (code) => toHandle(await gateway.connectWithCode(code)),
  });
}

/**
 * `listen` (TDM-56): the approval → orchestrator bridge. Deliberately shares
 * nothing with the Gateway — it talks to no API, it only receives signed
 * webhooks from one and execs a command.
 */
async function runListenCommand(): Promise<number> {
  const opts = parseListenArgs(cliArgs.slice(1), process.env);
  if (opts.help) {
    process.stdout.write(listenHelp());
    return 0;
  }
  return runListen(opts);
}

async function main() {
  if (LAUNCHER_MODE) {
    process.exit(await runLauncherCommand());
  }
  if (INIT_MODE) {
    process.exit(await runInitCommand());
  }
  if (LISTEN_MODE) {
    process.exit(await runListenCommand());
  }
  const server = createTandemServer(gateway, VERSION, { fullTools: FULL_TOOLS });
  // MCP_TRACE only: prints the session summary when the client goes away
  // (stdin close) or the process is signalled. No-op — and no signal handlers
  // installed at all — when tracing is off.
  installSessionSummary();
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  if (LAUNCHER_MODE) {
    const word = UP_MODE ? "up" : OPEN_MODE ? "open" : "down";
    process.stderr.write(`\ntandem ${word}: ${err instanceof Error ? err.message : err}\n`);
    if (err instanceof UpUsageError) {
      process.stderr.write(`Run \`tandem-mcp up --help\` for options.\n`);
    }
    process.exit(1);
  }
  if (INIT_MODE) {
    // A CLI failure: a clean one-liner, not a stack, and usage help when the
    // flags were wrong.
    process.stderr.write(`\ntandem init: ${err instanceof Error ? err.message : err}\n`);
    if (err instanceof InitUsageError) {
      process.stderr.write(`Run \`tandem-mcp init --help\` for options.\n`);
    }
    process.exit(1);
  }
  if (LISTEN_MODE) {
    process.stderr.write(`\ntandem listen: ${err instanceof Error ? err.message : err}\n`);
    if (err instanceof ListenUsageError) {
      process.stderr.write(`Run \`tandem-mcp listen --help\` for options.\n`);
    }
    process.exit(1);
  }
  process.stderr.write(`[tandem] Fatal: ${err}\n`);
  process.exit(1);
});
