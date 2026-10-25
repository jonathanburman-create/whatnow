package host

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
)

// connectorPath is where Claude reaches the user's planner as a custom connector. It answers to
// a connector key in the Authorization header, never to the session cookie.
const connectorPath = "/mcp"

// ErrNoConnectorKey means the connector key is unknown, replaced, or revoked.
var ErrNoConnectorKey = errors.New("host: no such connector key")

// ConnectorKey is what the settings card shows of a user's key: never the key itself.
type ConnectorKey struct {
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// CreateConnectorKey returns a new key for the user, replacing any they had.
func (a *Accounts) CreateConnectorKey(ctx context.Context, userID string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := "wnc_" + base64.RawURLEncoding.EncodeToString(raw)
	_, err := a.db.ExecContext(ctx,
		`INSERT INTO connector_keys (user_id, key_hash, created_at) VALUES (?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET key_hash = excluded.key_hash, created_at = excluded.created_at, last_used_at = NULL`,
		userID, hashToken(key), a.now().UTC().Format(timeLayout))
	if err != nil {
		return "", err
	}
	return key, nil
}

// ConnectorKey returns the user's key as the card shows it, or nil when they have none.
func (a *Accounts) ConnectorKey(ctx context.Context, userID string) (*ConnectorKey, error) {
	var created string
	var used sql.NullString
	err := a.db.QueryRowContext(ctx, `SELECT created_at, last_used_at FROM connector_keys WHERE user_id = ?`, userID).
		Scan(&created, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	k := &ConnectorKey{}
	if k.CreatedAt, err = time.Parse(timeLayout, created); err != nil {
		return nil, err
	}
	if used.Valid {
		t, err := time.Parse(timeLayout, used.String)
		if err != nil {
			return nil, err
		}
		k.LastUsedAt = &t
	}
	return k, nil
}

func (a *Accounts) DeleteConnectorKey(ctx context.Context, userID string) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM connector_keys WHERE user_id = ?`, userID)
	return err
}

// UserForConnectorKey returns the key's user and notes the key was used, or ErrNoConnectorKey.
func (a *Accounts) UserForConnectorKey(ctx context.Context, key string) (User, error) {
	var userID string
	err := a.db.QueryRowContext(ctx,
		`UPDATE connector_keys SET last_used_at = ? WHERE key_hash = ? RETURNING user_id`,
		a.now().UTC().Format(timeLayout), hashToken(key)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNoConnectorKey
	}
	if err != nil {
		return User{}, err
	}
	return userByID(ctx, a.db, userID)
}

func (f *frontDoor) connectorKey(w http.ResponseWriter, r *http.Request, u User) {
	k, err := f.accounts.ConnectorKey(r.Context(), u.ID)
	if err != nil {
		log.Printf("find connector key for %s: %v", u.ID, err)
		writeJSON(w, http.StatusInternalServerError, errorBody{"could not look up your key"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Key *ConnectorKey `json:"key"`
		URL string        `json:"url"`
	}{k, f.origin + connectorPath})
}

func (f *frontDoor) createConnectorKey(w http.ResponseWriter, r *http.Request, u User) {
	key, err := f.accounts.CreateConnectorKey(r.Context(), u.ID)
	if err != nil {
		log.Printf("create connector key for %s: %v", u.ID, err)
		writeJSON(w, http.StatusInternalServerError, errorBody{"could not create a key"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Key string `json:"key"`
	}{key})
}

func (f *frontDoor) deleteConnectorKey(w http.ResponseWriter, r *http.Request, u User) {
	if err := f.accounts.DeleteConnectorKey(r.Context(), u.ID); err != nil {
		log.Printf("delete connector key for %s: %v", u.ID, err)
		writeJSON(w, http.StatusInternalServerError, errorBody{"could not revoke the key"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// connector hands a request from Claude to the key's user's App. Each request counts against the
// user's limiter, like any other write from outside.
func (f *frontDoor) connector(w http.ResponseWriter, r *http.Request) {
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || key == "" {
		writeJSON(w, http.StatusUnauthorized, errorBody{"send your connector key as a Bearer token"})
		return
	}
	u, err := f.accounts.UserForConnectorKey(r.Context(), key)
	if errors.Is(err, ErrNoConnectorKey) {
		writeJSON(w, http.StatusUnauthorized, errorBody{"unknown key"})
		return
	}
	if err != nil {
		log.Printf("find connector key: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{"could not check your key"})
		return
	}
	if !f.limiter(u.ID).AllowN(f.now(), 1) {
		writeJSON(w, http.StatusTooManyRequests, errorBody{"too many requests; wait a minute and try again"})
		return
	}
	f.registry.Serve(w, r, u.ID)
}
