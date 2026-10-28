/**
 * TDM-22 — distinct default agent names.
 *
 * Agent names are unique per canvas and registering a held name refreshes the
 * holder, so subagents that connected with no `name` (default: the bare role)
 * merged into one "executor" on the board. The API now mints the name; what the
 * gateway must do is (1) tell the API which agent this session already is, so a
 * re-register refreshes instead of forking, (2) never let a FRESH connect
 * inherit an identity, and (3) claim under the name the API answered with, not
 * the one it asked for.
 *
 * No network: fetch is replaced with a fake API that applies the same rules as
 * store.ResolveAgentName for the cases exercised here.
 */
import { test } from "node:test";
import assert from "node:assert/strict";

import { Gateway, parseSession } from "../src/gateway.js";
import { handleTool } from "../src/tools.js";

type Row = { id: string; name: string; role: string; parentAgentId?: string };
type Posted = { body: any; identity: string | null };

/** A fake API: auth handshake + an agents table with TDM-22's naming rules. */
function withFakeApiTDM22(run: (posts: Posted[], rows: Row[]) => Promise<void>): Promise<void> {
  const posts: Posted[] = [];
  const rows: Row[] = [];
  let seq = 0;
  const real = globalThis.fetch;
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input));
    const json = (v: unknown, status = 200) =>
      new Response(JSON.stringify(v), { status, headers: { "content-type": "application/json" } });
    if (url.pathname === "/api/mcp/auth") {
      return json({ token: "jwt", canvasId: "canvas-1", canvasName: "C", canvasCode: "TESTCODE" });
    }
    if (url.pathname === "/api/canvas/agents") {
      const body = JSON.parse(String(init?.body ?? "{}"));
      const identity = new Headers(init?.headers).get("X-Tandem-Agent");
      posts.push({ body, identity });
      const sessionRow = rows.find((r) => r.id === body.agentId);
      let row: Row | undefined;
      if (!body.name) {
        row = sessionRow;
        if (!row) {
          row = { id: `agent-${++seq}`, name: `${body.role}-${String(seq).padStart(4, "x")}`, role: body.role };
          rows.push(row);
        }
      } else {
        const holder = rows.find((r) => r.name === body.name);
        const same = holder && (holder === sessionRow || holder.name === identity);
        if (!holder || same || !body.parentAgentId) {
          row = holder ?? { id: `agent-${++seq}`, name: body.name, role: body.role };
          if (!holder) rows.push(row);
        } else {
          row = { id: `agent-${++seq}`, name: `${body.name}-zz${seq}x`, role: body.role };
          rows.push(row);
        }
      }
      if (body.parentAgentId) row.parentAgentId = body.parentAgentId;
      return json(
        {
          agentId: row.id,
          name: row.name,
          ...(body.name && row.name !== body.name ? { renamedFrom: body.name } : {}),
        },
        201
      );
    }
    throw new Error(`unexpected request: ${url.pathname}`);
  }) as typeof globalThis.fetch;
  return run(posts, rows).finally(() => {
    globalThis.fetch = real;
  });
}

function claimantOf(handle: string): string {
  const gw = new Gateway({ apiUrl: "http://api.test" });
  gw.adoptSession(handle);
  return gw.claimant();
}

test("TDM-22: two nameless executor connects are two agents, each claiming under its minted name", async () => {
  await withFakeApiTDM22(async (posts, rows) => {
    // ONE gateway process, as when a planner's subagents share the stdio server.
    const gw = new Gateway({ apiUrl: "http://api.test" });
    const a = (await handleTool(gw, "canvas_connect", { code: "TESTCODE", role: "executor" })) as any;
    const b = (await handleTool(gw, "canvas_connect", { code: "TESTCODE", role: "executor" })) as any;

    assert.equal(rows.length, 2, "two agent rows, not one merged executor");
    assert.notEqual(a.agentId, b.agentId);
    assert.notEqual(a.agent.name, b.agent.name);
    // A fresh connect never presents the previous session's agent.
    assert.equal(posts[1].body.agentId, undefined);
    assert.equal(claimantOf(a.session), a.agent.name);
    assert.equal(claimantOf(b.session), b.agent.name);
    assert.equal(parseSession(b.session).agentName, rows[1].name);
  });
});

test("TDM-22: the same session re-registering keeps its agent", async () => {
  await withFakeApiTDM22(async (posts, rows) => {
    const gw = new Gateway({ apiUrl: "http://api.test" });
    const first = (await handleTool(gw, "canvas_connect", { code: "TESTCODE", role: "executor" })) as any;
    const again = (await handleTool(gw, "agent_register", {
      role: "executor",
      model: "claude-opus-5-5",
      session: first.session,
    })) as any;

    assert.equal(posts[1].body.agentId, first.agentId, "the session's own agent rides the register");
    assert.equal(again.agentId, first.agentId);
    assert.equal(rows.length, 1);
    assert.equal(claimantOf(again.session), first.agent.name);
  });
});

test("TDM-22: existing named registration is unchanged", async () => {
  await withFakeApiTDM22(async (posts) => {
    const res = (await handleTool(new Gateway({ apiUrl: "http://api.test" }), "canvas_connect", {
      code: "TESTCODE",
      role: "executor",
      name: "opus-executor-3",
    })) as any;
    assert.equal(posts[0].body.name, "opus-executor-3");
    assert.equal(res.agent.name, "opus-executor-3");
    assert.equal("renamedFrom" in res.agent, false);
    assert.equal(claimantOf(res.session), "opus-executor-3");
  });
});

test("TDM-22: a suffixed sibling is told, and claims under the name it was given", async () => {
  await withFakeApiTDM22(async () => {
    const gw = new Gateway({ apiUrl: "http://api.test" });
    const args = { code: "TESTCODE", role: "executor", name: "worker", parentAgentId: "planner-1" };
    const a = (await handleTool(gw, "canvas_connect", { ...args })) as any;
    const b = (await handleTool(gw, "canvas_connect", { ...args })) as any;

    assert.equal(a.agent.name, "worker");
    assert.notEqual(b.agentId, a.agentId);
    assert.equal(b.agent.renamedFrom, "worker");
    assert.match(b.agent.nameNote, /already names a different agent/);
    assert.equal(claimantOf(b.session), b.agent.name);
    assert.notEqual(b.agent.name, "worker");
  });
});
