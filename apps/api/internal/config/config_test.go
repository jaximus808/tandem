package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setRequired isolates Load() from the developer's real ~/.tandem and pins the
// posture under test to TANDEM_AUTH=google, where the SSRF opt-in is strict.
// (Local mode defaults the flag ON — see TestLocalModeAllowsPrivateWebhooks.)
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("TANDEM_DATA_DIR", t.TempDir())
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("TANDEM_AUTH", "google")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
}

// TANDEM_WEBHOOKS_ALLOW_PRIVATE turns off the webhook delivery worker's SSRF
// guard, so it has to fail closed: only the two documented values enable it, and
// anything ambiguous leaves prod's posture alone.
func TestWebhooksAllowPrivateTargetsIsStrictOptIn(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{" true ", true}, // whitespace from a docker env file is not a typo
		{"", false},      // unset — the default, and the only prod-safe value
		{"0", false},
		{"false", false},
		{"TRUE", false}, // case-sensitive on purpose: an opt-in to an unsafe
		{"yes", false},  // behaviour should not have fuzzy spellings
		{"maybe", false},
	}
	for _, tc := range cases {
		t.Run("value="+tc.raw, func(t *testing.T) {
			setRequired(t)
			t.Setenv("TANDEM_WEBHOOKS_ALLOW_PRIVATE", tc.raw)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.WebhooksAllowPrivateTargets != tc.want {
				t.Errorf("WebhooksAllowPrivateTargets = %v for %q, want %v",
					cfg.WebhooksAllowPrivateTargets, tc.raw, tc.want)
			}
		})
	}
}

// The flag is absent from the environment entirely, not merely empty — the case
// every real deployment other than local dev is in.
func TestWebhooksAllowPrivateTargetsDefaultsOff(t *testing.T) {
	setRequired(t)
	// t.Setenv records the pre-test value and restores it on cleanup; unsetting
	// afterwards gives a genuinely absent var without leaking it out of the test.
	t.Setenv("TANDEM_WEBHOOKS_ALLOW_PRIVATE", "")
	if err := os.Unsetenv("TANDEM_WEBHOOKS_ALLOW_PRIVATE"); err != nil {
		t.Fatalf("unset: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WebhooksAllowPrivateTargets {
		t.Error("the SSRF guard must be ON when TANDEM_WEBHOOKS_ALLOW_PRIVATE is unset")
	}
}

// Local mode is the default and needs NO environment: the data dir is created,
// the JWT secret is generated once and reused on the next Load.
func TestLocalModeNeedsNoEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TANDEM_DATA_DIR", dir)
	t.Setenv("JWT_SECRET", "")
	t.Setenv("TANDEM_AUTH", "")
	t.Setenv("GOOGLE_CLIENT_ID", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Local() || cfg.AuthMode != AuthModeNone {
		t.Fatalf("AuthMode = %q, want none", cfg.AuthMode)
	}
	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if want := filepath.Join(dir, "tandem.db"); cfg.DBPath != want {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, want)
	}
	if want := filepath.Join(dir, "images"); cfg.ImageDir != want {
		t.Errorf("ImageDir = %q, want %q", cfg.ImageDir, want)
	}
	if len(cfg.JWTSecret) != 64 {
		t.Fatalf("generated JWTSecret = %q, want 64 hex chars", cfg.JWTSecret)
	}
	again, err := Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if again.JWTSecret != cfg.JWTSecret {
		t.Errorf("JWT secret not persisted: %q then %q", cfg.JWTSecret, again.JWTSecret)
	}
	fi, err := os.Stat(filepath.Join(dir, "secret"))
	if err != nil {
		t.Fatalf("secret file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("secret file mode = %o, want 600", fi.Mode().Perm())
	}
}

// The only receiver a local install ever has is a `tandem-mcp listen` on the
// same machine, so local mode allows private targets unless told otherwise.
func TestLocalModeAllowsPrivateWebhooks(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"", true},
		{"1", true},
		{"true", true},
		{"0", false},
		{"false", false},
		{"maybe", true},
	}
	for _, tc := range cases {
		t.Run("value="+tc.raw, func(t *testing.T) {
			t.Setenv("TANDEM_DATA_DIR", t.TempDir())
			t.Setenv("JWT_SECRET", "secret")
			t.Setenv("TANDEM_AUTH", "none")
			t.Setenv("TANDEM_WEBHOOKS_ALLOW_PRIVATE", tc.raw)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.WebhooksAllowPrivateTargets != tc.want {
				t.Errorf("WebhooksAllowPrivateTargets = %v for %q, want %v", cfg.WebhooksAllowPrivateTargets, tc.raw, tc.want)
			}
		})
	}
}

func TestGoogleModeRequiresClientID(t *testing.T) {
	t.Setenv("TANDEM_DATA_DIR", t.TempDir())
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("TANDEM_AUTH", "google")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	if _, err := Load(); err == nil {
		t.Fatal("TANDEM_AUTH=google without GOOGLE_CLIENT_ID must fail to load")
	}
	t.Setenv("TANDEM_AUTH", "basic")
	t.Setenv("GOOGLE_CLIENT_ID", "x")
	if _, err := Load(); err == nil {
		t.Fatal("unknown TANDEM_AUTH must fail to load")
	}
}
