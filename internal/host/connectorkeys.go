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

// capturePath takes a note from outside the browser, such as an iPhone Shortcut, with the same
// key. The note lands in the inbox as raw text, as if typed into the app.
const capturePath = "/capture"

// askPath sends a message to the app's chat with the same key and answers with the reply as plain
// text, so a Shortcut can show it as a notification. Both land in the app's chat.
const askPath = "/ask"

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
		Key        *ConnectorKey `json:"key"`
		URL        string        `json:"url"`
		CaptureURL string        `json:"capture_url"`
		AskURL     string        `json:"ask_url"`
	}{k, f.origin + connectorPath, f.origin + capturePath, f.origin + askPath})
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

// connector hands a request from Claude to the key's user's App.
func (f *frontDoor) connector(w http.ResponseWriter, r *http.Request) {
	if u, ok := f.keyUser(w, r); ok {
		f.registry.Serve(w, r, u.ID)
	}
}

// capture saves a note sent with the connector key, such as from an iPhone Shortcut, as a raw
// task. Its body is {"raw_text": "..."}, the same as capturing in the app, so with a model set up
// cleanup tidies it, and without one it waits in the inbox for Claude.
func (f *frontDoor) capture(w http.ResponseWriter, r *http.Request) {
	u, ok := f.keyUser(w, r)
	if !ok {
		return
	}
	fwd := r.Clone(r.Context())
	fwd.URL.Path = "/api/v1/tasks"
	fwd.URL.RawPath = ""
	f.registry.Serve(w, fwd, u.ID)
}

// ask hands a message sent with the connector key to the key's user's chat, and answers with the
// reply's text. Its body is {"text": "..."}, the same as chatting in the app.
func (f *frontDoor) ask(w http.ResponseWriter, r *http.Request) {
	u, ok := f.keyUser(w, r)
	if !ok {
		return
	}
	fwd := r.Clone(r.Context())
	fwd.URL.Path = "/api/v1/ask"
	fwd.URL.RawPath = ""
	f.registry.Serve(w, fwd, u.ID)
}

// keyUser returns the user whose connector key the request carries, or answers the request with
// why not. Each request counts against the user's limiter, like any other write from outside.
func (f *frontDoor) keyUser(w http.ResponseWriter, r *http.Request) (User, bool) {
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || key == "" {
		writeJSON(w, http.StatusUnauthorized, errorBody{"send your connector key as a Bearer token"})
		return User{}, false
	}
	u, err := f.accounts.UserForConnectorKey(r.Context(), key)
	if errors.Is(err, ErrNoConnectorKey) {
		writeJSON(w, http.StatusUnauthorized, errorBody{"unknown key"})
		return User{}, false
	}
	if err != nil {
		log.Printf("find connector key: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{"could not check your key"})
		return User{}, false
	}
	if !f.limiter(u.ID).AllowN(f.now(), 1) {
		writeJSON(w, http.StatusTooManyRequests, errorBody{"too many requests; wait a minute and try again"})
		return User{}, false
	}
	return u, true
}
