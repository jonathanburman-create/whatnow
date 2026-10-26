package host

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jameynakama/whatnow/internal/domain"
)

type connectorKeyView struct {
	Key *ConnectorKey `json:"key"`
	URL string        `json:"url"`
}

// connectorCall is a tool call as Claude's connector sends it: no cookie and no Origin, just the
// key, as a JSON-RPC request to /mcp.
func connectorCall(t *testing.T, key, tool string, args any) *http.Request {
	t.Helper()
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(b))
	req.Host = "whatnow.test"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req
}

func (f *frontTest) createConnectorKey(c *http.Cookie) string {
	f.t.Helper()
	w := f.do(http.MethodPost, "/api/v1/connector/key", c, nil)
	f.want(w, http.StatusOK)
	key := decodeBody[struct {
		Key string `json:"key"`
	}](f.t, w).Key
	if !strings.HasPrefix(key, "wnc_") {
		f.t.Fatalf("key = %q", key)
	}
	return key
}

func TestConnectorFilesIntoTheKeysUser(t *testing.T) {
	f := newFrontTest(t)
	_, alice := f.signIn("a")
	_, bob := f.signIn("b")

	v := decodeBody[connectorKeyView](t, f.do(http.MethodGet, "/api/v1/connector/key", alice, nil))
	if v.Key != nil || v.URL != publicURL+"/mcp" {
		t.Fatalf("before = %+v", v)
	}
	key := f.createConnectorKey(alice)

	w := f.serve(connectorCall(t, key, "add_items", map[string]any{
		"tasks": []map[string]any{{"title": "Read chapter 4 for ECO2000", "area": "uoft"}}}))
	f.want(w, http.StatusOK)
	if strings.Contains(w.Body.String(), `"isError":true`) {
		t.Fatalf("tool error: %s", w.Body)
	}

	aliceTasks := decodeBody[[]domain.Task](t, f.do(http.MethodGet, "/api/v1/tasks", alice, nil))
	if len(aliceTasks) != 1 || aliceTasks[0].Title != "Read chapter 4 for ECO2000" {
		t.Errorf("alice's tasks = %+v", aliceTasks)
	}
	if bobTasks := decodeBody[[]domain.Task](t, f.do(http.MethodGet, "/api/v1/tasks", bob, nil)); len(bobTasks) != 0 {
		t.Errorf("bob's tasks = %+v", bobTasks)
	}
	v = decodeBody[connectorKeyView](t, f.do(http.MethodGet, "/api/v1/connector/key", alice, nil))
	if v.Key == nil || v.Key.LastUsedAt == nil {
		t.Errorf("key = %+v, want it used", v.Key)
	}
}

func TestConnectorRefused(t *testing.T) {
	f := newFrontTest(t)
	_, c := f.signIn("a")
	key := f.createConnectorKey(c)

	f.want(f.serve(connectorCall(t, "", "get_overview", map[string]any{})), http.StatusUnauthorized)
	f.want(f.serve(connectorCall(t, "wnc_nope", "get_overview", map[string]any{})), http.StatusUnauthorized)

	cookieOnly := connectorCall(t, "", "get_overview", map[string]any{})
	cookieOnly.AddCookie(c)
	cookieOnly.Header.Set("Origin", publicURL)
	f.want(f.serve(cookieOnly), http.StatusUnauthorized)

	// A Jira push key opens only the push route.
	push := f.createKey(c)
	f.want(f.serve(connectorCall(t, push, "get_overview", map[string]any{})), http.StatusUnauthorized)

	wrongHost := connectorCall(t, key, "get_overview", map[string]any{})
	wrongHost.Host = "evil.test"
	f.want(f.serve(wrongHost), http.StatusForbidden)

	f.want(f.do(http.MethodDelete, "/api/v1/connector/key", c, nil), http.StatusNoContent)
	f.want(f.serve(connectorCall(t, key, "get_overview", map[string]any{})), http.StatusUnauthorized)
}

func captureRequest(t *testing.T, key, text string) *http.Request {
	t.Helper()
	b, err := json.Marshal(map[string]string{"raw_text": text})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/capture", bytes.NewReader(b))
	req.Host = "whatnow.test"
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req
}

func TestCaptureFromAShortcut(t *testing.T) {
	f := newFrontTest(t)
	_, c := f.signIn("a")
	key := f.createConnectorKey(c)

	f.want(f.serve(captureRequest(t, key, "pack before tomorrow, clean the floor")), http.StatusCreated)
	tasks := decodeBody[[]domain.Task](t, f.do(http.MethodGet, "/api/v1/tasks", c, nil))
	if len(tasks) != 1 || tasks[0].RawText != "pack before tomorrow, clean the floor" || tasks[0].CreatedBy != domain.ActorUser {
		t.Errorf("tasks = %+v", tasks)
	}

	f.want(f.serve(captureRequest(t, "", "x")), http.StatusUnauthorized)
	f.want(f.serve(captureRequest(t, "wnc_nope", "x")), http.StatusUnauthorized)
	push := f.createKey(c)
	f.want(f.serve(captureRequest(t, push, "x")), http.StatusUnauthorized)

	v := decodeBody[struct {
		CaptureURL string `json:"capture_url"`
	}](t, f.do(http.MethodGet, "/api/v1/connector/key", c, nil))
	if v.CaptureURL != publicURL+"/capture" {
		t.Errorf("capture url = %q", v.CaptureURL)
	}
}

func askRequest(t *testing.T, key, text string) *http.Request {
	t.Helper()
	b, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(b))
	req.Host = "whatnow.test"
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req
}

func TestAskFromAShortcut(t *testing.T) {
	f := newFrontTest(t)
	_, c := f.signIn("a")
	key := f.createConnectorKey(c)

	// No provider is set up in these tests, so the reply says so, in plain text a notification can show.
	rec := f.serve(askRequest(t, key, "what's next?"))
	f.want(rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type = %q", ct)
	}
	if got := rec.Body.String(); !strings.Contains(got, "No AI provider is set up yet") {
		t.Errorf("reply = %q", got)
	}

	// Both the question and the reply are in the app's chat.
	history := decodeBody[[]domain.Message](t, f.do(http.MethodGet, "/api/v1/chat", c, nil))
	if len(history) != 2 || history[0].Text != "what's next?" || history[0].Role != domain.RoleUser || history[1].Role != domain.RoleAssistant {
		t.Errorf("history = %+v", history)
	}

	f.want(f.serve(askRequest(t, "", "x")), http.StatusUnauthorized)
	f.want(f.serve(askRequest(t, "wnc_nope", "x")), http.StatusUnauthorized)
	push := f.createKey(c)
	f.want(f.serve(askRequest(t, push, "x")), http.StatusUnauthorized)

	v := decodeBody[struct {
		AskURL string `json:"ask_url"`
	}](t, f.do(http.MethodGet, "/api/v1/connector/key", c, nil))
	if v.AskURL != publicURL+"/ask" {
		t.Errorf("ask url = %q", v.AskURL)
	}
}

func TestGlanceFromAShortcut(t *testing.T) {
	f := newFrontTest(t)
	_, c := f.signIn("a")
	key := f.createConnectorKey(c)

	glance := func(key string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/glance", nil)
		req.Host = "whatnow.test"
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		return req
	}
	rec := f.serve(glance(key))
	f.want(rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type = %q", ct)
	}
	if got := rec.Body.String(); got != "Today isn't planned yet. Open the planner to start your day." {
		t.Errorf("glance = %q", got)
	}
	f.want(f.serve(glance("")), http.StatusUnauthorized)
	f.want(f.serve(glance("wnc_nope")), http.StatusUnauthorized)

	v := decodeBody[struct {
		GlanceURL string `json:"glance_url"`
	}](t, f.do(http.MethodGet, "/api/v1/connector/key", c, nil))
	if v.GlanceURL != publicURL+"/glance" {
		t.Errorf("glance url = %q", v.GlanceURL)
	}
}
