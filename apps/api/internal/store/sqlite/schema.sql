-- Tandem local store — SQLite schema, current shape (no migration history; the
-- 44 Postgres migrations under supabase/migrations are what this collapses).
--
-- Conventions: uuids are TEXT (canonical lowercase), timestamps are fixed-width
-- RFC3339 UTC TEXT with nanoseconds ("2006-01-02T15:04:05.000000000Z") so that
-- lexical order == temporal order in every WHERE predicate, JSON / array
-- columns are TEXT holding JSON, booleans are INTEGER 0/1. Every updated_at is
-- set explicitly by the Go side (no triggers).

CREATE TABLE IF NOT EXISTS users (
  id                        TEXT PRIMARY KEY,
  google_sub                TEXT NOT NULL UNIQUE,
  email                     TEXT NOT NULL DEFAULT '',
  display_name              TEXT NOT NULL DEFAULT '',
  avatar_url                TEXT NOT NULL DEFAULT '',
  default_canvas_visibility TEXT NOT NULL DEFAULT 'public'
                            CHECK (default_canvas_visibility IN ('public', 'private')),
  default_public_role       TEXT NOT NULL DEFAULT 'read'
                            CHECK (default_public_role IN ('read', 'write')),
  agent_follow_style        TEXT NOT NULL DEFAULT 'cinematic'
                            CHECK (agent_follow_style IN ('cinematic', 'minimal')),
  created_at                TEXT NOT NULL,
  last_seen_at              TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS canvases (
  id                         TEXT PRIMARY KEY,
  code                       TEXT NOT NULL UNIQUE,
  name                       TEXT NOT NULL,
  mode                       TEXT NOT NULL DEFAULT 'welcome',
  enabled_modes              TEXT NOT NULL DEFAULT '[]',
  owner_user_id              TEXT REFERENCES users(id) ON DELETE SET NULL,
  claim_token                TEXT UNIQUE,
  visibility                 TEXT NOT NULL DEFAULT 'public'
                             CHECK (visibility IN ('public', 'private')),
  public_role                TEXT NOT NULL DEFAULT 'write'
                             CHECK (public_role IN ('read', 'write')),
  approval_policy            TEXT NOT NULL DEFAULT 'epic'
                             CHECK (approval_policy IN ('strict', 'epic', 'auto', 'peer')),
  require_cross_model_review INTEGER NOT NULL DEFAULT 0,
  briefing_doc_id            TEXT REFERENCES documents(id) ON DELETE SET NULL,
  next_ticket                INTEGER NOT NULL DEFAULT 1,
  version                    INTEGER NOT NULL DEFAULT 0,
  created_at                 TEXT NOT NULL,
  updated_at                 TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS canvases_owner_user_id_idx ON canvases(owner_user_id);

CREATE TABLE IF NOT EXISTS documents (
  id                  TEXT PRIMARY KEY,
  canvas_id           TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  type                TEXT NOT NULL,
  name                TEXT NOT NULL DEFAULT '',
  parent_id           TEXT REFERENCES documents(id) ON DELETE SET NULL,
  sort_order          INTEGER NOT NULL DEFAULT 0,
  config              TEXT NOT NULL DEFAULT '{}',
  created_by          TEXT NOT NULL DEFAULT 'agent',
  authored_by         TEXT,
  verified_at         TEXT,
  stale_after_seconds INTEGER CHECK (stale_after_seconds IS NULL OR stale_after_seconds > 0),
  created_at          TEXT NOT NULL,
  updated_at          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS documents_canvas_id_idx ON documents(canvas_id);
CREATE INDEX IF NOT EXISTS documents_parent_id_idx ON documents(parent_id);

CREATE TABLE IF NOT EXISTS notes (
  id                  TEXT PRIMARY KEY,
  canvas_id           TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  document_id         TEXT REFERENCES documents(id) ON DELETE CASCADE,
  body                TEXT NOT NULL DEFAULT '',
  image_refs          TEXT NOT NULL DEFAULT '[]',
  parent_id           TEXT,
  parent_kind         TEXT,
  sort_order          INTEGER NOT NULL DEFAULT 0,
  created_by          TEXT NOT NULL DEFAULT 'agent',
  authored_by         TEXT,
  verified_at         TEXT,
  stale_after_seconds INTEGER CHECK (stale_after_seconds IS NULL OR stale_after_seconds > 0),
  created_at          TEXT NOT NULL,
  updated_at          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS notes_canvas_id_idx ON notes(canvas_id);
CREATE INDEX IF NOT EXISTS notes_document_id_idx ON notes(document_id);

CREATE TABLE IF NOT EXISTS roadmap_items (
  id                  TEXT PRIMARY KEY,
  canvas_id           TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  document_id         TEXT REFERENCES documents(id) ON DELETE CASCADE,
  parent_id           TEXT REFERENCES roadmap_items(id) ON DELETE CASCADE,
  title               TEXT NOT NULL,
  body                TEXT NOT NULL DEFAULT '',
  status              TEXT NOT NULL DEFAULT 'todo'
                      CHECK (status IN ('todo', 'in_progress', 'done', 'blocked')),
  stage               TEXT,
  assignee            TEXT,
  sort_order          INTEGER NOT NULL DEFAULT 0,
  created_by          TEXT NOT NULL DEFAULT 'agent',
  verified_at         TEXT,
  stale_after_seconds INTEGER CHECK (stale_after_seconds IS NULL OR stale_after_seconds > 0),
  created_at          TEXT NOT NULL,
  updated_at          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS roadmap_items_canvas_id_idx ON roadmap_items(canvas_id);
CREATE INDEX IF NOT EXISTS roadmap_items_canvas_parent_idx ON roadmap_items(canvas_id, parent_id);

CREATE TABLE IF NOT EXISTS agents (
  id              TEXT PRIMARY KEY,
  canvas_id       TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  role            TEXT NOT NULL CHECK (role IN ('planner', 'executor')),
  model           TEXT,
  parent_agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL,
  status          TEXT NOT NULL DEFAULT 'online' CHECK (status IN ('online', 'offline')),
  created_at      TEXT NOT NULL,
  last_seen_at    TEXT NOT NULL,
  UNIQUE (canvas_id, name)
);
CREATE INDEX IF NOT EXISTS agents_canvas_id_idx ON agents(canvas_id);

CREATE TABLE IF NOT EXISTS actions (
  id             TEXT PRIMARY KEY,
  canvas_id      TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  type           TEXT NOT NULL DEFAULT 'navigate',
  state          TEXT NOT NULL DEFAULT 'proposed'
                 CHECK (state IN ('proposed', 'approved', 'rejected', 'executing', 'done', 'failed')),
  payload        TEXT NOT NULL DEFAULT '{}',
  proposed_by    TEXT NOT NULL DEFAULT 'agent',
  approved_by    TEXT,
  claimed_by     TEXT,
  claimed_at     TEXT,
  result         TEXT,
  error          TEXT,
  ticket         INTEGER,
  authored_by    TEXT,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS actions_canvas_id_idx    ON actions(canvas_id);
CREATE INDEX IF NOT EXISTS actions_canvas_state_idx ON actions(canvas_id, state);
CREATE INDEX IF NOT EXISTS actions_canvas_ticket_idx ON actions(canvas_id, ticket);

CREATE TABLE IF NOT EXISTS canvas_access (
  canvas_id  TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  user_id    TEXT NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
  role       TEXT NOT NULL CHECK (role IN ('read', 'write')),
  created_at TEXT NOT NULL,
  PRIMARY KEY (canvas_id, user_id)
);
CREATE INDEX IF NOT EXISTS canvas_access_user_id_idx ON canvas_access(user_id);

CREATE TABLE IF NOT EXISTS notifications (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
  kind          TEXT NOT NULL,
  canvas_id     TEXT REFERENCES canvases(id) ON DELETE CASCADE,
  actor_user_id TEXT REFERENCES users(id)    ON DELETE SET NULL,
  role          TEXT,
  read_at       TEXT,
  created_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS notifications_user_idx ON notifications(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS personal_access_tokens (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash   TEXT NOT NULL UNIQUE,
  name         TEXT NOT NULL DEFAULT '',
  last_four    TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  last_used_at TEXT,
  expires_at   TEXT
);
CREATE INDEX IF NOT EXISTS personal_access_tokens_user_id_idx ON personal_access_tokens(user_id);

CREATE TABLE IF NOT EXISTS oauth_clients (
  id                         TEXT PRIMARY KEY,
  client_name                TEXT NOT NULL DEFAULT '',
  redirect_uris              TEXT NOT NULL DEFAULT '[]',
  grant_types                TEXT NOT NULL DEFAULT '["authorization_code","refresh_token"]',
  token_endpoint_auth_method TEXT NOT NULL DEFAULT 'none',
  created_at                 TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
  code_hash             TEXT PRIMARY KEY,
  client_id             TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
  user_id               TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  redirect_uri          TEXT NOT NULL,
  code_challenge        TEXT NOT NULL,
  code_challenge_method TEXT NOT NULL DEFAULT 'S256',
  scope                 TEXT NOT NULL DEFAULT '',
  resource              TEXT NOT NULL DEFAULT '',
  expires_at            TEXT NOT NULL,
  created_at            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS oauth_authorization_codes_expires_idx ON oauth_authorization_codes(expires_at);

CREATE TABLE IF NOT EXISTS oauth_tokens (
  id                 TEXT PRIMARY KEY,
  access_token_hash  TEXT NOT NULL UNIQUE,
  refresh_token_hash TEXT UNIQUE,
  client_id          TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
  user_id            TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scope              TEXT NOT NULL DEFAULT '',
  resource           TEXT NOT NULL DEFAULT '',
  access_expires_at  TEXT NOT NULL,
  refresh_expires_at TEXT,
  created_at         TEXT NOT NULL,
  last_used_at       TEXT,
  revoked_at         TEXT
);
CREATE INDEX IF NOT EXISTS oauth_tokens_user_id_idx ON oauth_tokens(user_id);

CREATE TABLE IF NOT EXISTS webhooks (
  id          TEXT PRIMARY KEY,
  canvas_id   TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  url         TEXT NOT NULL,
  secret      TEXT NOT NULL,
  events      TEXT NOT NULL DEFAULT '["task.approved","task.completed","task.claim_expired","task.returned","task.rejected"]',
  enabled     INTEGER NOT NULL DEFAULT 1,
  name        TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  created_by  TEXT NOT NULL DEFAULT 'user',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS webhooks_canvas_id_idx ON webhooks(canvas_id);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id              TEXT PRIMARY KEY,
  webhook_id      TEXT NOT NULL REFERENCES webhooks(id)  ON DELETE CASCADE,
  canvas_id       TEXT NOT NULL REFERENCES canvases(id)  ON DELETE CASCADE,
  event_id        TEXT NOT NULL,
  event_type      TEXT NOT NULL,
  payload         TEXT NOT NULL DEFAULT '{}',
  status          TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'delivering', 'ok', 'failed', 'dead')),
  attempt_count   INTEGER NOT NULL DEFAULT 0,
  last_attempt_at TEXT,
  next_attempt_at TEXT NOT NULL,
  response_status INTEGER,
  response_body   TEXT,
  error           TEXT,
  created_at      TEXT NOT NULL,
  UNIQUE (webhook_id, event_id)
);
CREATE INDEX IF NOT EXISTS webhook_deliveries_due_idx ON webhook_deliveries(next_attempt_at);
CREATE INDEX IF NOT EXISTS webhook_deliveries_webhook_status_idx ON webhook_deliveries(webhook_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS webhook_deliveries_canvas_idx ON webhook_deliveries(canvas_id, created_at DESC);

CREATE TABLE IF NOT EXISTS pending_edits (
  id          TEXT PRIMARY KEY,
  canvas_id   TEXT NOT NULL REFERENCES canvases(id) ON DELETE CASCADE,
  entity_id   TEXT NOT NULL,
  instruction TEXT NOT NULL,
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS pending_edits_canvas_id_idx ON pending_edits(canvas_id);
