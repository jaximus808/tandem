package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embed Go's tzdata so time.LoadLocation works in any container, regardless
	// of whether the base image ships system tzdata. Needed for per-event IANA
	// timezones anywhere a location-aware time is rendered.
	_ "time/tzdata"

	"github.com/agentcanvas/api/internal/api"
	"github.com/agentcanvas/api/internal/auth"
	"github.com/agentcanvas/api/internal/config"
	"github.com/agentcanvas/api/internal/metrics"
	"github.com/agentcanvas/api/internal/store/sqlite"
	"github.com/agentcanvas/api/internal/webhooks"
	"github.com/agentcanvas/api/internal/ws"
)

// version is stamped by scripts/build.sh (-X main.version=…); "dev" for a
// plain `go run`. Reported by /healthz and /api/config.
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := os.MkdirAll(cfg.ImageDir, 0755); err != nil {
		log.Fatalf("image dir: %v", err)
	}

	db, err := sqlite.Open(cfg.DBPath, sqlite.WithClaimTTL(cfg.ClaimTTL))
	if err != nil {
		log.Fatalf("sqlite %s: %v", cfg.DBPath, err)
	}
	defer db.Close()
	log.Printf("tandem %s — data in %s (db %s)", version, cfg.DataDir, cfg.DBPath)

	authSvc := auth.NewService(cfg.JWTSecret, cfg.JWTTokenTTL)

	var googleVerifier *auth.GoogleVerifier
	if cfg.Local() {
		// Local mode: one user, no sign-in. The row exists so ownership, the
		// approval gate and /api/auth/me all work exactly as for a signed-in
		// owner — the browser gets its session from index.html (api/local.go).
		if err := ensureLocalUser(db); err != nil {
			log.Fatalf("local user: %v", err)
		}
		log.Printf("local mode (TANDEM_AUTH=none) — no sign-in; the browser is the owner")
	} else {
		googleVerifier, err = auth.NewGoogleVerifier(context.Background(), cfg.GoogleClientID)
		if err != nil {
			log.Fatalf("google verifier: %v", err)
		}
		log.Printf("google sign-in enabled (TANDEM_AUTH=google)")
	}

	// Operational metrics (TDM-42): ONE registry for the process, built here
	// because three things need the same one — the hub (broadcast fan-out +
	// the ws_clients gauge), the webhook delivery worker (delivery outcomes)
	// and the API handlers (per-route latency + claim counters). It stays nil
	// when METRICS_ENABLED=false, and every seam below is nil-safe, so "off"
	// really means nothing is recorded rather than recorded-and-hidden.
	var metricsReg *metrics.Registry
	if cfg.MetricsEnabled {
		metricsReg = metrics.NewRegistry()
	} else {
		log.Printf("METRICS_ENABLED=false — GET /api/metrics disabled, nothing recorded")
	}

	hub := ws.NewHub()
	if metricsReg != nil {
		// Wired BEFORE Run so there is no window where broadcasts go unmeasured.
		// (SetObserver is safe on a running hub too — the field is mutex-guarded.)
		hub.SetObserver(metricsReg)
	}
	go hub.Run()

	// Outbound-webhook delivery worker (TDM-36 / migration 0038). Same shape as
	// the hub above: a goroutine started at boot, stopped by cancelling its
	// context during graceful shutdown. It owns all outbound HTTP for webhooks —
	// request handlers only ever enqueue (webhooks.Emitter), so a slow or dead
	// receiver can never slow a canvas mutation.
	// The Emitter is the request-path half: handlers hand it task-lifecycle
	// events (TDM-37) and it enqueues one delivery row per subscribed webhook.
	// It stays nil when webhooks are off, which makes every emit in the handlers
	// a no-op — nothing is enqueued that no worker would ever drain.
	webhookCtx, stopWebhooks := context.WithCancel(context.Background())
	defer stopWebhooks()
	var emitter *webhooks.Emitter
	if cfg.WebhooksEnabled {
		emitter = webhooks.NewEmitter(db)
		var wOpts []webhooks.WorkerOption
		if metricsReg != nil {
			wOpts = append(wOpts, webhooks.WithObserver(metricsReg))
		}
		if cfg.WebhooksAllowPrivateTargets {
			// Default in local mode (the receiver is a `tandem-mcp listen` on this
			// machine); a strict opt-in under TANDEM_AUTH=google. NewWorker's default
			// sender has the guard on, so this is the only way it can be off.
			if cfg.Local() {
				log.Printf("webhooks may target private addresses (local mode; TANDEM_WEBHOOKS_ALLOW_PRIVATE=0 to re-arm the guard)")
			} else {
				log.Printf("WARNING: webhook SSRF guard disabled — private targets allowed (TANDEM_WEBHOOKS_ALLOW_PRIVATE)")
			}
			wOpts = append(wOpts, webhooks.WithSender(webhooks.NewSender(webhooks.WithAllowPrivateTargets(true))))
		}
		go webhooks.NewWorker(db, wOpts...).Run(webhookCtx)
	} else {
		log.Printf("WEBHOOKS_ENABLED=false — outbound webhook delivery worker disabled")
	}

	// Confidentiality gate for the GitHub status proxy (TDM-140): drop a
	// GH_STATUS_TOKEN that can read PRIVATE repos BEFORE the (lazily-built) client
	// reads the env, so the proxy can never surface private-repo state cross-canvas.
	api.VetGitHubTokenEnv()

	router := api.NewRouter(db, hub, authSvc, googleVerifier, cfg.CookieSecure, cfg.WebDistPath, cfg.ImageDir, cfg.PublicBaseURL, metricsReg, emitter,
		api.WithAuthMode(cfg.AuthMode), api.WithVersion(version))

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: router,
		// WriteTimeout is intentionally NOT set: it would kill long-lived
		// WebSocket upgrades (gorilla hijacks the conn but Go still enforces
		// the server-level timeout against the response). The per-message
		// deadlines in ws/client.go cover slow WS peers; ReadHeaderTimeout +
		// IdleTimeout cover slowloris on plain HTTP.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Tandem listening on http://127.0.0.1:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Fatalf("server: %v", err)
	case sig := <-stop:
		log.Printf("received %s, shutting down…", sig)
	}

	// Give in-flight requests up to 15s to finish before forcing the close.
	// WebSocket connections are sent a close frame by hub.Shutdown so clients
	// can reconnect cleanly instead of dropping mid-frame.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hub.Shutdown()
	stopWebhooks()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown: %v (forcing close)", err)
		_ = srv.Close()
	}
	log.Printf("bye")
}

// ensureLocalUser upserts the fixed local owner (api.LocalUserID) with the
// posture local canvases need: public + writable, so a bare canvas code lets an
// agent propose and claim work. Idempotent; runs on every boot.
func ensureLocalUser(db *sqlite.Store) error {
	ctx := context.Background()
	u, err := db.UpsertUserByGoogleSub(ctx, api.LocalUserTemplate())
	if err != nil {
		return err
	}
	if u.ID != api.LocalUserID {
		return fmt.Errorf("local user has id %s, want %s (was the database created by a different build?)", u.ID, api.LocalUserID)
	}
	if u.DefaultCanvasVisibility != "public" {
		if _, err := db.UpdateUserDefaultVisibility(ctx, u.ID, "public"); err != nil {
			return err
		}
	}
	if u.DefaultPublicRole != "write" {
		if _, err := db.UpdateUserDefaultPublicRole(ctx, u.ID, "write"); err != nil {
			return err
		}
	}
	return nil
}
