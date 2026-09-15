# syntax=docker/dockerfile:1.7
#
# Tandem — OPTIONAL container image for people who want to self-host.
#
# Tandem is a locally run tool first: the normal path is the release binary
# (`npx @jaximus/tandem-mcp up`, or download it from GitHub Releases), which
# needs no container. This image exists so a team can put one Tandem on a box:
# the same single binary, SQLite in a /data volume, and — if they want accounts
# — Google sign-in turned on with TANDEM_AUTH=google.
#
#   docker build -t tandem .
#   docker run -p 7891:7891 -v tandem-data:/data tandem
#
# Self-hosting with sign-in (all optional; local mode needs none of these):
#   -e TANDEM_AUTH=google -e GOOGLE_CLIENT_ID=… -e PUBLIC_BASE_URL=https://… \
#   -e COOKIE_SECURE=true  and build with --build-arg VITE_GOOGLE_CLIENT_ID=…

# ── Web bundle ────────────────────────────────────────────────────────────────
FROM node:22-alpine AS web-builder
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable && corepack prepare pnpm@11.2.2 --activate
WORKDIR /repo

COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/web/package.json apps/web/package.json
COPY apps/mcp-gateway/package.json apps/mcp-gateway/package.json
COPY internal/shared/package.json internal/shared/package.json
RUN pnpm install --frozen-lockfile

COPY internal/shared internal/shared
COPY apps/web apps/web

# Only needed when the image will run with TANDEM_AUTH=google: Vite inlines the
# client id into the bundle at build time. Empty is fine for local mode.
ARG VITE_GOOGLE_CLIENT_ID
ENV VITE_GOOGLE_CLIENT_ID=$VITE_GOOGLE_CLIENT_ID
RUN pnpm --filter @agentcanvas/shared build && pnpm --filter web build

# ── Go binary with the bundle embedded ────────────────────────────────────────
# Tracks apps/api/go.mod's `go` directive.
FROM golang:1.25-alpine AS api-builder
WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api ./
COPY --from=web-builder /repo/apps/web/dist ./internal/webdist/bundle
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=$(tr -d '[:space:]' < VERSION)" \
      -o /out/tandem ./cmd/server

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tini \
 && adduser -D -u 10001 tandem \
 && mkdir -p /data && chown tandem:tandem /data
COPY --from=api-builder /out/tandem /usr/local/bin/tandem
USER tandem

ENV PORT=7891 \
    TANDEM_DATA_DIR=/data \
    TANDEM_AUTH=none
VOLUME ["/data"]
EXPOSE 7891
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD wget -qO- http://127.0.0.1:7891/healthz || exit 1
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["tandem"]
