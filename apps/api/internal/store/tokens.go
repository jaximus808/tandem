package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Identifiers, secrets and hashing shared by every Store implementation and by
// the API layer. Nothing here touches a database.

// Ambiguous glyphs (0/O, 1/I/L) intentionally excluded so codes are easy to
// dictate over voice/chat. Length 8 over this 32-char alphabet → 32^8 ≈ 1.1e12
// keyspace, which is what makes brute-forcing the canvas code (our only auth)
// infeasible — provided the generator is unpredictable. Hence crypto/rand.
const codeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func GenerateCode() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is a kernel-level problem; we'd rather refuse
		// to issue a guessable code than silently fall back to math/rand.
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	out := make([]byte, 8)
	for i, x := range b {
		out[i] = codeChars[int(x)%len(codeChars)]
	}
	return string(out)
}

// generateClaimToken mints a private "own this canvas" token. Its format is
// deliberately distinct from a canvas code (see migration 0020): a "clm_" prefix
// plus 32 lowercase hex chars. Different prefix, length, alphabet, and case mean
// a code can never be mistaken for or collide with a token. 16 bytes = 128 bits
// of entropy, appropriate for an unguessable bearer secret (vs. the code's ~40
// bits, which only gates view access). crypto/rand for the same reason as codes.
func GenerateClaimToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return "clm_" + hex.EncodeToString(b)
}

// PATPrefix namespaces personal access tokens. Prefix-gating on this string lets
// the auth middleware skip a DB lookup for any bearer that isn't a PAT, and keeps
// PATs unmistakable for canvas codes ('clm_' claim tokens, 8-char codes).
const PATPrefix = "tdm_pat_"

// GeneratePAT mints a user's personal access token: PATPrefix + 32 random bytes
// (hex) = 256 bits of entropy, appropriate for a long-lived bearer secret.
// crypto/rand for the same reason as codes/claim tokens.
func GeneratePAT() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return PATPrefix + hex.EncodeToString(b)
}

// HashToken returns the hex SHA-256 of a token. We store only this hash, so a DB
// leak can't yield usable tokens; auth hashes the presented secret and matches.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LastFour returns the trailing 4 chars of a token, kept for UI disambiguation.
func LastFour(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}

// ── DB row types (snake_case = Supabase column names) ─────────────────────────

// DefaultClaimTTL is how long an 'executing' task claim is honored before it
// is considered stuck and becomes atomically claimable by the next claimer
// (lazy expiry, evaluated inside ClaimAction — there is NO background sweeper).
// Mirrors config's CLAIM_TTL_MINUTES default; override via the store's
// WithClaimTTL option.
const DefaultClaimTTL = 15 * time.Minute

// skipBumpKey marks a context whose per-item Update*/Delete* store calls should
// NOT bump the canvas version — the batch path opts in via WithoutVersionBump and
// bumps once itself via BumpCanvasVersion.
type skipBumpKey struct{}

// WithoutVersionBump returns a context that suppresses the per-item canvas-version
// bump inside the Update*/Delete* store methods. The batch handlers wrap the
// context they hand to their per-item store calls with this, then bump the version
// exactly once (BumpCanvasVersion) after every write in the batch has landed.
// Single (non-batch) callers never set it, so their behavior is unchanged.
func WithoutVersionBump(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipBumpKey{}, true)
}

// ShouldSkipVersionBump is the read side of WithoutVersionBump, for store
// implementations (they live in their own packages and cannot see the key).
func ShouldSkipVersionBump(ctx context.Context) bool {
	v, _ := ctx.Value(skipBumpKey{}).(bool)
	return v
}

// OAuth 2.1 authorization-server storage (migration 0029). Secrets (auth codes,
// access + refresh tokens) are stored ONLY as SHA-256 hashes — see HashToken in
// supabase.go, shared with the PAT model. Plaintext is returned to the client
// once and never persisted or logged.

// OAuthAccessPrefix namespaces OAuth access tokens. The auth middleware
// prefix-gates on it so a bearer that isn't an OAuth access token skips the DB
// lookup, and it keeps these unmistakable for PATs (tdm_pat_) / claim tokens.
const (
	OAuthAccessPrefix   = "tdm_oat_"
	oauthRefreshPrefix  = "tdm_ort_"
	oauthCodePrefix     = "tdm_ac_"
	oauthClientIDPrefix = "tdm_client_"
)

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("crypto/rand: %w", err))
	}
	return hex.EncodeToString(b)
}

// GenerateOAuthClientID mints a client identifier for dynamic registration. Not
// a secret (public clients use PKCE), but unguessable to avoid enumeration.
func GenerateOAuthClientID() string { return oauthClientIDPrefix + randomHex(16) }

// GenerateAuthCode / GenerateAccessToken / GenerateRefreshToken each mint a
// 256-bit opaque secret with its namespacing prefix.
func GenerateAuthCode() string     { return oauthCodePrefix + randomHex(32) }
func GenerateAccessToken() string  { return OAuthAccessPrefix + randomHex(32) }
func GenerateRefreshToken() string { return oauthRefreshPrefix + randomHex(32) }
