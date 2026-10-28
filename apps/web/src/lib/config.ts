import { createContext, useContext } from "react";

/**
 * The server's boot-time configuration, read ONCE from GET /api/config before
 * the app renders anything (see ConfigProvider in main.tsx).
 *
 * `auth` is the deploy flag: "none" is LOCAL MODE — one owner, no sign-in, the
 * dashboard at "/", no landing page; "google" is the hosted/self-hosted shape
 * with accounts, sharing and the marketing surfaces. `local` mirrors it as a
 * boolean for the common `if (config.local)` guard. `version` is the binary's
 * version string (empty when the server doesn't report one). `transcripts` says
 * whether GET /api/canvas/agents/{id}/transcript is served (TDM-18: local mode
 * and not TANDEM_TRANSCRIPTS=off) — false on a server that doesn't report it.
 */
export interface AppConfig {
  auth: "none" | "google";
  local: boolean;
  version: string;
  transcripts: boolean;
}

/**
 * What an OLD server (no /api/config route) is treated as: the hosted shape,
 * so a web bundle deployed ahead of its API regresses nothing — sign-in, the
 * landing page and the account chrome all keep working exactly as before.
 */
export const HOSTED_FALLBACK: AppConfig = { auth: "google", local: false, version: "", transcripts: false };

let cached: AppConfig | null = null;
let inflight: Promise<AppConfig> | null = null;

function normalize(raw: unknown): AppConfig {
  const r = (raw ?? {}) as Record<string, unknown>;
  const auth: AppConfig["auth"] = r.auth === "none" ? "none" : "google";
  // `local` follows `auth` unless the server says otherwise — a server that
  // reports auth:"none" is local whether or not it also set the boolean.
  const local = typeof r.local === "boolean" ? r.local : auth === "none";
  const version = typeof r.version === "string" ? r.version : "";
  const transcripts = r.transcripts === true;
  return { auth, local, version, transcripts };
}

/** Fetch /api/config once; every later call shares the same promise. */
export function loadConfig(): Promise<AppConfig> {
  if (cached) return Promise.resolve(cached);
  if (inflight) return inflight;
  inflight = fetch("/api/config", { credentials: "same-origin" })
    .then(async (res) => {
      if (!res.ok) return HOSTED_FALLBACK;
      return normalize(await res.json());
    })
    .catch(() => HOSTED_FALLBACK)
    .then((cfg) => {
      cached = cfg;
      return cfg;
    });
  return inflight;
}

/**
 * The resolved config, synchronously — for non-React code (lib/auth.ts) that
 * needs to branch on the mode. `null` only before loadConfig has resolved,
 * which, given the boot gate in main.tsx, never happens from rendered code.
 */
export function getConfig(): AppConfig | null {
  return cached;
}

/** Sync convenience for the one question most callers ask. False while unknown. */
export function isLocalMode(): boolean {
  return cached?.local === true;
}

/** Test/mock seam: pre-seed the cache so no fetch happens. */
export function setConfigForTesting(cfg: AppConfig | null): void {
  cached = cfg;
  inflight = null;
}

export const ConfigContext = createContext<AppConfig>(HOSTED_FALLBACK);

/**
 * The resolved config from inside React. Always non-null: the provider does
 * not render its children until /api/config has answered (or failed over to
 * HOSTED_FALLBACK), so there is no loading state for components to handle.
 */
export function useConfig(): AppConfig {
  return useContext(ConfigContext);
}
