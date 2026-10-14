#!/usr/bin/env bash
# claim-race.sh — the helper behind scripts/demo/claim-race.tape (TDM-4).
#
# Two workers go for the same approved task at the same instant against a REAL
# Tandem server. Every line the workers print comes from an API response: the
# winner's 200 carries the ticket id, the loser's 409 carries `claimedBy`, and
# the loser then claims the next ready task.
#
# Nothing here touches ~/.tandem or a server you already run: `up` starts a
# throwaway server on its own port with TANDEM_DATA_DIR in a temp dir, and
# `down` kills it and deletes that dir.
#
#   scripts/demo/claim-race.sh up              # start server, seed canvas + 2 approved tasks
#   scripts/demo/claim-race.sh worker worker-a # (in one terminal)
#   scripts/demo/claim-race.sh worker worker-b # (in another, at the same time)
#   scripts/demo/claim-race.sh down            # stop server, remove temp data
#
#   scripts/demo/claim-race.sh split           # tmux: both workers typed, side by side,
#                                              # panes synchronized so ONE Enter fires both
#
# Env: TANDEM_BIN (server binary; default dist/tandem, then ~/.tandem/bin/tandem,
# then `tandem` on PATH), RACE_PORT (default 17962), RACE_DIR (state dir;
# default ${TMPDIR:-/tmp}/tandem-claim-race). Needs curl + jq.
set -euo pipefail

PORT="${RACE_PORT:-17962}"
DIR="${RACE_DIR:-${TMPDIR:-/tmp}/tandem-claim-race}"
DIR="${DIR%/}"
# localhost, not 127.0.0.1: the owner cookie below is scoped to this host name.
API="http://localhost:${PORT}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

die() { echo "claim-race: $*" >&2; exit 1; }

find_bin() {
  if [[ -n "${TANDEM_BIN:-}" ]]; then echo "$TANDEM_BIN"; return; fi
  for b in "$ROOT/dist/tandem" "$HOME/.tandem/bin/tandem"; do
    [[ -x "$b" ]] && { echo "$b"; return; }
  done
  command -v tandem || die "no tandem binary (set TANDEM_BIN, or run scripts/build.sh)"
}

cmd_up() {
  command -v jq >/dev/null || die "jq is required"
  cmd_down >/dev/null 2>&1 || true
  mkdir -p "$DIR/data"
  local bin; bin="$(find_bin)"
  TANDEM_DATA_DIR="$DIR/data" PORT="$PORT" nohup "$bin" >"$DIR/server.log" 2>&1 &
  echo $! >"$DIR/server.pid"
  for _ in $(seq 1 100); do
    curl -sf "$API/healthz" >/dev/null 2>&1 && break
    sleep 0.1
  done
  curl -sf "$API/healthz" >/dev/null || die "server did not start (see $DIR/server.log)"

  # Local mode: loading the page mints the local owner's session cookie — the
  # same way a browser becomes "the human". Only the owner may create a task
  # born approved; the workers below connect WITHOUT it, as agents.
  curl -sf -c "$DIR/owner.cookies" -o /dev/null "$API/"
  local code token
  code="$(curl -sf -b "$DIR/owner.cookies" -H 'Content-Type: application/json' \
    -d '{"name":"claim race"}' "$API/api/canvases" | jq -r .code)"
  token="$(curl -sf -b "$DIR/owner.cookies" -H 'Content-Type: application/json' \
    -d "{\"code\":\"$code\"}" "$API/api/mcp/auth" | jq -r .token)"
  for title in "Fix the login redirect" "Add rate limits to /api"; do
    curl -sf -b "$DIR/owner.cookies" -H "Authorization: Bearer $token" \
      -H 'Content-Type: application/json' \
      -d "$(jq -nc --arg t "$title" '{type:"task",state:"approved",payload:{title:$t}}')" \
      "$API/api/canvas/actions" >/dev/null
  done
  echo "$code" >"$DIR/code"
  echo "canvas $code on $API — 2 approved tasks"
}

cmd_down() {
  [[ -f "$DIR/server.pid" ]] && kill "$(cat "$DIR/server.pid")" 2>/dev/null || true
  rm -rf "$DIR"
  # Last: when `down` runs inside a `split` pane this kills our own shell.
  tmux -L claimrace kill-server 2>/dev/null || true
}

# Two panes, one worker command typed into each (not yet run), panes
# synchronized: a single Enter starts both workers at once. Own tmux socket and
# no config, so it neither sees nor touches your tmux.
cmd_split() {
  local t=(tmux -L claimrace -f /dev/null)
  local sh="env -i HOME=$HOME PATH=$PATH TERM=xterm-256color BASH_SILENCE_DEPRECATION_WARNING=1 RACE_DIR=$DIR RACE_PORT=$PORT PS1='$ ' bash --norc --noprofile"
  "${t[@]}" kill-server 2>/dev/null || true
  "${t[@]}" new-session -d -s race -c "$ROOT/scripts/demo" "$sh"
  "${t[@]}" split-window -h -t race -c "$ROOT/scripts/demo" "$sh"
  "${t[@]}" set -g status off \; set -g pane-border-status top \
    \; set -g pane-border-format ' #{pane_title} ' \
    \; set -g pane-border-style fg=colour240 \; set -g pane-active-border-style fg=colour240
  # Let both shells reach their prompt first, or the typed command is echoed
  # twice (once as typeahead, once at the prompt).
  sleep 0.5
  local i name
  for i in 0 1; do
    name="worker-$([[ $i == 0 ]] && echo a || echo b)"
    "${t[@]}" select-pane -t "race.$i" -T "$name"
    "${t[@]}" send-keys -t "race.$i" "./claim-race.sh worker $name"
  done
  "${t[@]}" set -w -t race synchronize-panes on
  exec "${t[@]}" attach -t race
}

# Colours: dim for plumbing, green for the win, red for the loss.
G=$'\e[32m'; R=$'\e[31m'; D=$'\e[2m'; B=$'\e[1m'; X=$'\e[0m'

cmd_worker() {
  local me="${1:?usage: claim-race.sh worker <name>}"
  local code; code="$(cat "$DIR/code" 2>/dev/null)" || die "run 'up' first"
  # Connect with the bare canvas code — no cookie — exactly like the MCP gateway.
  local token
  token="$(curl -sf -H 'Content-Type: application/json' -d "{\"code\":\"$code\"}" \
    "$API/api/mcp/auth" | jq -r .token)"
  echo "${B}$me${X} connected to ${B}$code${X}"

  # The ready queue, read from the API (tickets in order).
  local queue
  queue="$(curl -sf -H "Authorization: Bearer $token" \
    "$API/api/canvas/actions?state=approved&type=task" |
    jq -r '(.actions // .) | sort_by(.ticket) | .[] | "\(.id) \(.ticketId)"')"
  echo "ready: $(echo "$queue" | awk '{print $2}' | xargs)"

  # Barrier: both workers fire their claim at the same instant.
  touch "$DIR/ready.$me"
  while [[ $(ls "$DIR"/ready.* 2>/dev/null | wc -l) -lt 2 ]]; do sleep 0.01; done

  local id ref resp status body
  while read -r id ref; do
    [[ -z "$id" ]] && continue
    echo "→ task_claim $ref"
    resp="$(curl -sS -X PATCH -H "Authorization: Bearer $token" \
      -H "X-Tandem-Agent: $me" -H 'Content-Type: application/json' \
      -d "{\"state\":\"executing\",\"agentName\":\"$me\"}" \
      -w '\n%{http_code}' "$API/api/canvas/actions/$id")"
    status="${resp##*$'\n'}"; body="${resp%$'\n'*}"
    if [[ "$status" == 200 ]]; then
      echo "${G}${B}✓ claimed $(jq -r .action.ticketId <<<"$body")${X}"
      echo "  ${D}$(jq -r .action.payload.title <<<"$body")${X}"
      return 0
    fi
    echo "${R}✗ claimed:false${X}"
    echo "  claimedBy: ${B}$(jq -r '.claimedBy // .holder // "?"' <<<"$body")${X}"
    echo "  ${D}taking the next task${X}"
  done <<<"$queue"
  echo "nothing left to claim"
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  worker) shift; cmd_worker "$@" ;;
  split) cmd_split ;;
  *) sed -n '2,23p' "$0"; exit 2 ;;
esac
