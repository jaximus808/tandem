/* ─────────────────────────────────────────────────────────────────────────────
   Site-wide constants. Every link and command the marketing pages repeat lives
   here once, so a rename (package, repo, install command) is a one-line edit.
   ───────────────────────────────────────────────────────────────────────────── */

export const SITE_URL = "https://tandemcanvas.com";

export const REPO_URL = "https://github.com/jaximus808/tandam";
export const RELEASES_URL = `${REPO_URL}/releases`;
export const COMMITS_URL = `${REPO_URL}/commits/main`;
export const NPM_URL = "https://www.npmjs.com/package/@jaximus/tandem-mcp";
export const MCP_SPEC_URL = "https://modelcontextprotocol.io";

// Jaxon's links (About page + footers).
export const WEBSITE_URL = "https://www.jaxonp.com/";
export const LINKEDIN_URL = "https://www.linkedin.com/in/jaxon-poentis/";
export const GITHUB_URL = "https://github.com/jaximus808";

/** The one-line install: registers the MCP server in Claude Code (user scope),
 *  starts the local Tandem server, opens the board. */
export const SETUP_CMD = "npx -y @jaximus/tandem-mcp setup";
/** Start / open the board without touching Claude Code config. */
export const UP_CMD = "npx -y @jaximus/tandem-mcp up";
/** The manual registration, for people who'd rather not run `setup`. */
export const MCP_ADD_CMD = "claude mcp add --scope user tandem -- npx -y @jaximus/tandem-mcp";

/** Clients that speak MCP, named under the hero CTAs. */
export const HERO_CLIENTS = ["Claude Code", "Codex", "Cursor"];

/** The recorded walkthrough. Served from public/ like the section loops. */
export const FULL_DEMO_VIDEO_URL = "/demo/tandem-demo-full.mp4";
