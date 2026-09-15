package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/agentcanvas/api/internal/store"
)

// ── Users (migration 0008 + prefs) ────────────────────────────────────────────

const userCols = `id, google_sub, email, display_name, avatar_url, default_canvas_visibility, default_public_role, agent_follow_style, created_at, last_seen_at`

func scanUser(r rowScanner) (*store.User, error) {
	var u store.User
	var id, createdAt, lastSeen string
	if err := r.Scan(&id, &u.GoogleSub, &u.Email, &u.DisplayName, &u.AvatarURL, &u.DefaultCanvasVisibility, &u.DefaultPublicRole, &u.AgentFollowStyle, &createdAt, &lastSeen); err != nil {
		return nil, err
	}
	u.ID = mustUUID(id)
	u.CreatedAt = parseTime(createdAt)
	u.LastSeenAt = parseTime(lastSeen)
	return &u, nil
}

// UpsertUserByGoogleSub inserts on first sign-in and refreshes profile fields +
// last_seen_at on every later one. The id never changes.
func (s *Store) UpsertUserByGoogleSub(ctx context.Context, u *store.User) (*store.User, error) {
	now := nowStr()
	id := u.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	saved, err := scanUser(s.db.QueryRowContext(ctx, `INSERT INTO users
		(id, google_sub, email, display_name, avatar_url, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (google_sub) DO UPDATE SET email = excluded.email, display_name = excluded.display_name,
			avatar_url = excluded.avatar_url, last_seen_at = excluded.last_seen_at
		RETURNING `+userCols,
		id.String(), u.GoogleSub, u.Email, u.DisplayName, u.AvatarURL, now, now))
	if err != nil {
		return nil, fmt.Errorf("user upsert: %w", err)
	}
	return saved, nil
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*store.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user not found: %s", id)
	}
	return u, err
}

// GetUserByEmail backs share-by-email: case-insensitive exact match.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*store.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE LOWER(email) = LOWER(?)`, strings.TrimSpace(email)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrUserNotFound
	}
	return u, err
}

func (s *Store) updateUserPref(ctx context.Context, id uuid.UUID, col, value string) (*store.User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `UPDATE users SET `+col+` = ? WHERE id = ? RETURNING `+userCols, value, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrUserNotFound
	}
	return u, err
}

func (s *Store) UpdateUserDefaultVisibility(ctx context.Context, id uuid.UUID, visibility string) (*store.User, error) {
	return s.updateUserPref(ctx, id, "default_canvas_visibility", visibility)
}

func (s *Store) UpdateUserDefaultPublicRole(ctx context.Context, id uuid.UUID, role string) (*store.User, error) {
	return s.updateUserPref(ctx, id, "default_public_role", role)
}

func (s *Store) UpdateUserAgentFollowStyle(ctx context.Context, id uuid.UUID, style string) (*store.User, error) {
	return s.updateUserPref(ctx, id, "agent_follow_style", style)
}

// DeleteUserAccount removes a user and everything anchored to them: owned
// canvases first (owner_user_id is ON DELETE SET NULL, so the user delete alone
// would orphan them), then the user row, which cascades memberships, inbox,
// PATs and OAuth grants. Canvases merely shared with the user survive.
func (s *Store) DeleteUserAccount(ctx context.Context, userID uuid.UUID) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM canvases WHERE owner_user_id = ?`, userID.String()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID.String())
		return err
	})
}

// ── Personal access tokens (migration 0027) ───────────────────────────────────

const patCols = `id, name, last_four, created_at, last_used_at`

func scanPAT(r rowScanner) (*store.PersonalAccessToken, error) {
	var t store.PersonalAccessToken
	var id, createdAt string
	var lastUsed sql.NullString
	if err := r.Scan(&id, &t.Name, &t.LastFour, &createdAt, &lastUsed); err != nil {
		return nil, err
	}
	t.ID = mustUUID(id)
	t.CreatedAt = parseTime(createdAt)
	t.LastUsedAt = parseTimePtr(lastUsed)
	return &t, nil
}

func (s *Store) CreatePersonalAccessToken(ctx context.Context, userID uuid.UUID, name, tokenHash, lastFour string) (*store.PersonalAccessToken, error) {
	return scanPAT(s.db.QueryRowContext(ctx, `INSERT INTO personal_access_tokens (id, user_id, token_hash, name, last_four, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING `+patCols,
		uuid.New().String(), userID.String(), tokenHash, name, lastFour, nowStr()))
}

// ListPersonalAccessTokens lists a user's tokens newest-first.
func (s *Store) ListPersonalAccessTokens(ctx context.Context, userID uuid.UUID) ([]*store.PersonalAccessToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+patCols+` FROM personal_access_tokens WHERE user_id = ? ORDER BY created_at DESC, id`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*store.PersonalAccessToken{}
	for rows.Next() {
		t, err := scanPAT(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeletePersonalAccessToken is owner-scoped; a miss (wrong owner / gone) is
// ErrInvalidToken.
func (s *Store) DeletePersonalAccessToken(ctx context.Context, userID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM personal_access_tokens WHERE id = ? AND user_id = ?`, id.String(), userID.String())
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return store.ErrInvalidToken
	}
	return nil
}

// UserIDByTokenHash resolves the owning user from a token hash and best-effort
// touches last_used_at. ErrInvalidToken when nothing matches.
func (s *Store) UserIDByTokenHash(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var id, userID string
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id FROM personal_access_tokens WHERE token_hash = ?`, tokenHash).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, store.ErrInvalidToken
	}
	if err != nil {
		return uuid.Nil, err
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return uuid.Nil, store.ErrInvalidToken
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE personal_access_tokens SET last_used_at = ? WHERE id = ?`, nowStr(), id)
	return uid, nil
}

// ── OAuth 2.1 authorization server (migration 0029) ───────────────────────────

func (s *Store) CreateOAuthClient(ctx context.Context, c *store.OAuthClient) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_clients (id, client_name, redirect_uris, grant_types, token_endpoint_auth_method, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.ClientName, jsonText(c.RedirectURIs), jsonText(c.GrantTypes), c.TokenEndpointAuthMethod, fmtTime(c.CreatedAt))
	return err
}

func (s *Store) GetOAuthClient(ctx context.Context, id string) (*store.OAuthClient, error) {
	var c store.OAuthClient
	var redirects, grants, createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, client_name, redirect_uris, grant_types, token_endpoint_auth_method, created_at FROM oauth_clients WHERE id = ?`, id).
		Scan(&c.ID, &c.ClientName, &redirects, &grants, &c.TokenEndpointAuthMethod, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	c.RedirectURIs = jsonStrings(redirects)
	c.GrantTypes = jsonStrings(grants)
	c.CreatedAt = parseTime(createdAt)
	return &c, nil
}

func (s *Store) CreateAuthCode(ctx context.Context, codeHash string, c *store.OAuthCode) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_authorization_codes
		(code_hash, client_id, user_id, redirect_uri, code_challenge, code_challenge_method, scope, resource, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		codeHash, c.ClientID, c.UserID.String(), c.RedirectURI, c.CodeChallenge, c.CodeChallengeMethod, c.Scope, c.Resource, fmtTime(c.ExpiresAt), nowStr())
	return err
}

// ConsumeAuthCode deletes the code row and returns what it deleted — single
// use by construction. An expired code is still consumed, then refused.
func (s *Store) ConsumeAuthCode(ctx context.Context, codeHash string) (*store.OAuthCode, error) {
	var c store.OAuthCode
	var userID, expiresAt string
	err := s.db.QueryRowContext(ctx, `DELETE FROM oauth_authorization_codes WHERE code_hash = ?
		RETURNING client_id, user_id, redirect_uri, code_challenge, code_challenge_method, scope, resource, expires_at`, codeHash).
		Scan(&c.ClientID, &userID, &c.RedirectURI, &c.CodeChallenge, &c.CodeChallengeMethod, &c.Scope, &c.Resource, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, store.ErrInvalidGrant
	}
	c.UserID = uid
	c.ExpiresAt = parseTime(expiresAt)
	if time.Now().After(c.ExpiresAt) {
		return nil, store.ErrInvalidGrant
	}
	return &c, nil
}

func (s *Store) CreateOAuthGrant(ctx context.Context, accessHash, refreshHash string, g *store.OAuthGrant) error {
	id := g.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_tokens
		(id, access_token_hash, refresh_token_hash, client_id, user_id, scope, resource, access_expires_at, refresh_expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), accessHash, nullIfEmpty(refreshHash), g.ClientID, g.UserID.String(), g.Scope, g.Resource,
		fmtTime(g.AccessExpiresAt), timeArg(g.RefreshExpiresAt), nowStr())
	return err
}

// OAuthUserByAccessHash resolves the user + client behind a live (not expired,
// not revoked) access token and best-effort touches last_used_at.
func (s *Store) OAuthUserByAccessHash(ctx context.Context, accessHash string) (uuid.UUID, string, error) {
	var id, clientID, userID, expiresAt string
	err := s.db.QueryRowContext(ctx, `SELECT id, client_id, user_id, access_expires_at FROM oauth_tokens
		WHERE access_token_hash = ? AND revoked_at IS NULL`, accessHash).Scan(&id, &clientID, &userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, "", store.ErrInvalidToken
	}
	if err != nil {
		return uuid.Nil, "", err
	}
	if time.Now().After(parseTime(expiresAt)) {
		return uuid.Nil, "", store.ErrInvalidToken
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return uuid.Nil, "", store.ErrInvalidToken
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE oauth_tokens SET last_used_at = ? WHERE id = ?`, nowStr(), id)
	return uid, clientID, nil
}

func (s *Store) HasLiveOAuthGrant(ctx context.Context, userID uuid.UUID, clientID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM oauth_tokens WHERE user_id = ? AND client_id = ? AND revoked_at IS NULL`,
		userID.String(), clientID).Scan(&n)
	return n > 0, err
}

// ConsumeRefreshGrant atomically revokes the grant behind a refresh-token hash
// (only if still live) and returns its metadata; a replayed refresh token finds
// the row already revoked. Expiry is checked after.
func (s *Store) ConsumeRefreshGrant(ctx context.Context, refreshHash string) (*store.OAuthGrant, error) {
	var g store.OAuthGrant
	var id, userID string
	var refreshExpires sql.NullString
	err := s.db.QueryRowContext(ctx, `UPDATE oauth_tokens SET revoked_at = ? WHERE refresh_token_hash = ? AND revoked_at IS NULL
		RETURNING id, client_id, user_id, scope, resource, refresh_expires_at`, nowStr(), refreshHash).
		Scan(&id, &g.ClientID, &userID, &g.Scope, &g.Resource, &refreshExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, store.ErrInvalidGrant
	}
	g.ID = mustUUID(id)
	g.UserID = uid
	if exp := parseTimePtr(refreshExpires); exp != nil {
		if time.Now().After(*exp) {
			return nil, store.ErrInvalidGrant
		}
		g.RefreshExpiresAt = exp
	}
	return &g, nil
}

// ListOAuthConnections returns one entry per client the user has a live token
// for: earliest created_at, latest last_used_at, in first-seen order.
func (s *Store) ListOAuthConnections(ctx context.Context, userID uuid.UUID) ([]*store.OAuthConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.client_id, t.created_at, t.last_used_at, c.client_name
		FROM oauth_tokens t LEFT JOIN oauth_clients c ON c.id = t.client_id
		WHERE t.user_id = ? AND t.revoked_at IS NULL ORDER BY t.created_at, t.id`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byClient := map[string]*store.OAuthConnection{}
	order := []string{}
	for rows.Next() {
		var clientID, createdAt string
		var lastUsed, name sql.NullString
		if err := rows.Scan(&clientID, &createdAt, &lastUsed, &name); err != nil {
			return nil, err
		}
		conn, ok := byClient[clientID]
		if !ok {
			conn = &store.OAuthConnection{ClientID: clientID, ClientName: name.String}
			byClient[clientID] = conn
			order = append(order, clientID)
		}
		created := parseTime(createdAt)
		if conn.CreatedAt.IsZero() || created.Before(conn.CreatedAt) {
			conn.CreatedAt = created
		}
		if lu := parseTimePtr(lastUsed); lu != nil && (conn.LastUsedAt == nil || lu.After(*conn.LastUsedAt)) {
			conn.LastUsedAt = lu
		}
	}
	out := make([]*store.OAuthConnection, 0, len(order))
	for _, id := range order {
		out = append(out, byClient[id])
	}
	return out, rows.Err()
}

// RevokeOAuthConnection revokes all of the user's live tokens for a client.
func (s *Store) RevokeOAuthConnection(ctx context.Context, userID uuid.UUID, clientID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE oauth_tokens SET revoked_at = ? WHERE user_id = ? AND client_id = ? AND revoked_at IS NULL`,
		nowStr(), userID.String(), clientID)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return store.ErrInvalidGrant
	}
	return nil
}
