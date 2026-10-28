/* Site-wide constants. Every link and command the page repeats lives here once,
   so a rename (package, repo, install command) is a one-line edit. */

export const SITE_URL = "https://tandemcanvas.com";

export const REPO_URL = "https://github.com/jaximus808/tandem";
export const RELEASES_URL = `${REPO_URL}/releases`;
export const NPM_URL = "https://www.npmjs.com/package/@jaximus/tandem-mcp";
export const AUTHOR_URL = "https://www.jaxonp.com/";

/** The one-line install: registers the MCP server in Claude Code (user scope),
 *  starts the local Tandem server, opens the board. */
export const SETUP_CMD = "npx -y @jaximus/tandem-mcp setup";
