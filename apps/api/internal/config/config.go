package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultJWTTokenTTL = 24 * time.Hour

// defaultClaimTTL is how long an 'executing' task claim is honored before a
// new claimer may atomically take it over (lazy expiry — no background
// sweeper). Override with CLAIM_TTL_MINUTES; 0 disables expiry entirely.
const defaultClaimTTL = 15 * time.Minute

type Config struct {
	// AuthMode is TANDEM_AUTH: "none" (default — local mode: one machine, one
	// user, no sign-in) or "google" (self-hosted: Google sign-in, accounts,
	// PATs, sharing; requires GOOGLE_CLIENT_ID).
	AuthMode string

	// DataDir is TANDEM_DATA_DIR (default ~/.tandem): the SQLite database, the
	// auto-generated JWT secret, uploaded images and logs all live under it.
	DataDir string
	// DBPath is TANDEM_DB (default <DataDir>/tandem.db). ":memory:" works and
	// gives a throwaway board.
	DBPath string

	// JWTSecret signs canvas tokens and sessions. JWT_SECRET when set; otherwise
	// read from — or generated into — <DataDir>/secret, so a local install never
	// has to configure one and a restart keeps every token valid.
	JWTSecret   string
	JWTTokenTTL time.Duration
	Port        string
	// WebDistPath overrides the embedded web bundle with a directory (the dev
	// loop). Empty = serve the bundle compiled into the binary.
	WebDistPath string
	ImageDir    string

	// GoogleClientID enables Google sign-in. Only read under AuthMode "google".
	GoogleClientID string
	// CookieSecure marks the session cookie Secure. Leave false in local dev
	// (http); set COOKIE_SECURE=true when serving over https.
	CookieSecure bool

	// PublicBaseURL is the externally-reachable origin (e.g.
	// https://tandem.example.com), used as the OAuth issuer + to build the
	// authorization-server metadata URLs. Empty → the OAuth handlers derive it
	// per-request from the Host / X-Forwarded-Proto headers (fine locally).
	PublicBaseURL string

	// ClaimTTL is how long an 'executing' task claim is honored before it is
	// considered stuck and becomes atomically claimable by the next
	// task_start (lazy expiry, evaluated at claim time — no sweeper job).
	// 0 disables expiry (claims are held until released/completed).
	ClaimTTL time.Duration

	// WebhooksEnabled starts the outbound-webhook delivery worker. Defaults to
	// enabled — set WEBHOOKS_ENABLED=false to keep the loop off.
	WebhooksEnabled bool

	// WebhooksAllowPrivateTargets disables the delivery worker's SSRF guard
	// (webhooks.IsBlockedIP), letting deliveries reach loopback / RFC1918 /
	// link-local addresses.
	//
	// In LOCAL mode this defaults to ON: the only receiver that exists is a
	// `tandem-mcp listen` on this same machine, which the guard would refuse.
	// TANDEM_WEBHOOKS_ALLOW_PRIVATE=0|false turns it back off there.
	//
	// Under TANDEM_AUTH=google (a server other people reach) it stays a strict
	// opt-in — only the exact values "1" / "true" enable it — because with it
	// on, anyone who can edit a canvas's webhook config can make the server
	// connect to the internal network, including a cloud metadata endpoint.
	WebhooksAllowPrivateTargets bool

	// MetricsEnabled registers GET /api/metrics — in-memory per-route and
	// per-op latency percentiles, task-queue counters and gauges. Aggregates
	// only. METRICS_ENABLED=false switches the whole subsystem off.
	MetricsEnabled bool
}

// AuthMode values. Mirrored (not imported) by internal/api so config stays a
// leaf package.
const (
	AuthModeNone   = "none"
	AuthModeGoogle = "google"
)

// Local reports whether this process runs in local mode (no sign-in).
func (c *Config) Local() bool { return c.AuthMode == AuthModeNone }

// Load reads the environment. Nothing is required: a bare `tandem` starts in
// local mode with everything under ~/.tandem.
func Load() (*Config, error) {
	authMode := strings.ToLower(strings.TrimSpace(os.Getenv("TANDEM_AUTH")))
	switch authMode {
	case "":
		authMode = AuthModeNone
	case AuthModeNone, AuthModeGoogle:
	default:
		return nil, fmt.Errorf("TANDEM_AUTH=%q: expected \"none\" (local, the default) or \"google\"", authMode)
	}
	googleClientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	if authMode == AuthModeGoogle && googleClientID == "" {
		return nil, fmt.Errorf("TANDEM_AUTH=google requires GOOGLE_CLIENT_ID")
	}

	dataDir, err := resolveDataDir(os.Getenv("TANDEM_DATA_DIR"))
	if err != nil {
		return nil, err
	}
	dbPath := os.Getenv("TANDEM_DB")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "tandem.db")
	}
	imageDir := os.Getenv("IMAGE_DIR")
	if imageDir == "" {
		imageDir = filepath.Join(dataDir, "images")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret, err = loadOrCreateSecret(filepath.Join(dataDir, "secret"))
		if err != nil {
			return nil, err
		}
	}

	jwtTTL := defaultJWTTokenTTL
	if raw := os.Getenv("JWT_TOKEN_TTL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("JWT_TOKEN_TTL: %w (expected Go duration, e.g. 24h, 30m, 7d→use 168h)", err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("JWT_TOKEN_TTL must be positive, got %s", parsed)
		}
		jwtTTL = parsed
	}

	claimTTL := defaultClaimTTL
	if raw := os.Getenv("CLAIM_TTL_MINUTES"); raw != "" {
		mins, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("CLAIM_TTL_MINUTES: %w (expected whole minutes, e.g. 15; 0 disables claim expiry)", err)
		}
		if mins < 0 {
			return nil, fmt.Errorf("CLAIM_TTL_MINUTES must be >= 0, got %d", mins)
		}
		claimTTL = time.Duration(mins) * time.Minute
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "7891"
	}

	allowPrivate := envOptIn("TANDEM_WEBHOOKS_ALLOW_PRIVATE")
	if authMode == AuthModeNone {
		allowPrivate = !envOptOut("TANDEM_WEBHOOKS_ALLOW_PRIVATE")
	}

	return &Config{
		AuthMode:                    authMode,
		DataDir:                     dataDir,
		DBPath:                      dbPath,
		JWTSecret:                   jwtSecret,
		JWTTokenTTL:                 jwtTTL,
		ClaimTTL:                    claimTTL,
		Port:                        port,
		WebDistPath:                 os.Getenv("WEB_DIST_PATH"),
		ImageDir:                    imageDir,
		GoogleClientID:              googleClientID,
		CookieSecure:                os.Getenv("COOKIE_SECURE") == "true",
		PublicBaseURL:               strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		MetricsEnabled:              os.Getenv("METRICS_ENABLED") != "false",
		WebhooksEnabled:             os.Getenv("WEBHOOKS_ENABLED") != "false",
		WebhooksAllowPrivateTargets: allowPrivate,
	}, nil
}

// resolveDataDir expands the data directory (default ~/.tandem) and creates it.
func resolveDataDir(raw string) (string, error) {
	dir := strings.TrimSpace(raw)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("TANDEM_DATA_DIR unset and no home directory: %w", err)
		}
		dir = filepath.Join(home, ".tandem")
	} else if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("TANDEM_DATA_DIR: %w", err)
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("TANDEM_DATA_DIR: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("TANDEM_DATA_DIR %s: %w", dir, err)
	}
	return dir, nil
}

// loadOrCreateSecret returns the JWT secret persisted at path, generating a
// 32-byte random one (hex) on first run. Mode 0600: it is a credential.
func loadOrCreateSecret(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate JWT secret: %w", err)
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write JWT secret to %s: %w", path, err)
	}
	return secret, nil
}

// envOptOut is the mirror of envOptIn: only the exact values "0" / "false"
// count as a deliberate off.
func envOptOut(name string) bool {
	switch strings.TrimSpace(os.Getenv(name)) {
	case "0", "false":
		return true
	default:
		return false
	}
}

func envOptIn(name string) bool {
	switch strings.TrimSpace(os.Getenv(name)) {
	case "1", "true":
		return true
	default:
		return false
	}
}
