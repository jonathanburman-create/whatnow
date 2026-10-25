package connector_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jameynakama/whatnow/internal/connector"
	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/llm"
	"github.com/jameynakama/whatnow/internal/planner"
	"github.com/jameynakama/whatnow/internal/service"
	"github.com/jameynakama/whatnow/internal/settings"
	"github.com/jameynakama/whatnow/internal/store"
	"github.com/jameynakama/whatnow/internal/stream"
)

// loc is Toronto's offset in October, so local times and UTC differ in every assertion.
var loc = time.FixedZone("EDT", -4*3600)

type fixture struct {
	svc     *service.Service
	hub     *stream.Hub
	session *mcp.ClientSession
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	now := func() time.Time { return time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC) } // Monday 09:00 local
	svc := service.New(db, now)
	hub := stream.NewHub()
	noModel := func(context.Context) (llm.Client, settings.ProviderConfig, settings.Settings, error) {
		return nil, settings.ProviderConfig{}, settings.Defaults(), settings.ErrNotConfigured
	}
	plans := planner.New(svc, hub, noModel, loc, now)
	srv := httptest.NewServer(connector.Handler(connector.Deps{Service: svc, Hub: hub, Planner: plans, Loc: loc, Now: now}))
	t.Cleanup(srv.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: srv.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return &fixture{svc: svc, hub: hub, session: session}
}

// call runs a tool and decodes its structured result into out. It fails the test on a tool error.
func (f *fixture) call(t *testing.T, name string, args any, out any) {
	t.Helper()
	res := f.try(t, name, args)
	if res.IsError {
		t.Fatalf("%s: %s", name, text(res))
	}
	if out == nil {
		return
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: decode %s: %v", name, b, err)
	}
}

func (f *fixture) try(t *testing.T, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestOffersTheTools(t *testing.T) {
	f := newFixture(t)
	res, err := f.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	want := []string{"add_items", "delete_event", "delete_idea", "get_overview", "idea_to_task", "list_ideas",
		"list_tasks", "push_task", "set_plan", "set_task_status", "undo_last", "update_task"}
	if !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

func TestAddItemsFilesADumpInOneUndoableBatch(t *testing.T) {
	f := newFixture(t)
	msgs, unsub := f.hub.Subscribe("batch.applied")
	defer unsub()
	ctx := context.Background()
	dump := "read ch 4 for eco2000 by wednesday, dentist thursday at 3, essay idea rawls vs nozick"

	var added connector.Added
	f.call(t, "add_items", map[string]any{
		"raw_text": dump,
		"tasks":    []map[string]any{{"title": "Read chapter 4 for ECO2000", "area": "uoft", "due_on": "2026-10-28"}},
		"ideas":    []map[string]any{{"title": "Essay: Rawls vs Nozick on luck", "area": "huji"}},
		"events":   []map[string]any{{"title": "Dentist", "starts_at": "2026-10-29T15:00", "ends_at": "2026-10-29T16:00"}},
	}, &added)

	if len(added.Tasks) != 1 || len(added.Ideas) != 1 || len(added.Events) != 1 {
		t.Fatalf("added = %+v", added)
	}
	task, _ := f.svc.GetTask(ctx, added.Tasks[0].ID)
	if task.Area == nil || *task.Area != domain.AreaUofT || task.DueOn == nil || *task.DueOn != "2026-10-28" ||
		task.RawText != dump || task.CreatedBy != domain.ActorAgent {
		t.Errorf("task = %+v", task)
	}
	event, _ := f.svc.GetEvent(ctx, added.Events[0].ID)
	if !event.StartsAt.Equal(time.Date(2026, 10, 29, 19, 0, 0, 0, time.UTC)) {
		t.Errorf("event starts at %v, want 15:00 local", event.StartsAt)
	}
	if added.Events[0].StartsAt != "2026-10-29T15:00" {
		t.Errorf("event read back as %q, want local time", added.Events[0].StartsAt)
	}
	m := <-msgs
	if applied, ok := m.Data.(stream.BatchApplied); !ok || applied.Source != stream.SourceClaude || len(applied.Activity) != 3 {
		t.Errorf("published %+v", m)
	}

	f.call(t, "undo_last", map[string]any{}, nil)
	if tasks, _ := f.svc.ListTasks(ctx); len(tasks) != 0 {
		t.Errorf("after undo, tasks = %+v", tasks)
	}
	if ideas, _ := f.svc.ListIdeas(ctx); len(ideas) != 0 {
		t.Errorf("after undo, ideas = %+v", ideas)
	}
}

func TestABadItemChangesNothing(t *testing.T) {
	f := newFixture(t)
	res := f.try(t, "add_items", map[string]any{
		"tasks":  []map[string]any{{"title": "Pay the phone bill", "area": "home"}},
		"events": []map[string]any{{"title": "Dentist", "starts_at": "thursday at 3"}},
	})
	if !res.IsError || !strings.Contains(text(res), "YYYY-MM-DDTHH:MM") {
		t.Fatalf("result = %+v", res)
	}
	if tasks, _ := f.svc.ListTasks(context.Background()); len(tasks) != 0 {
		t.Errorf("a failed call left tasks behind: %+v", tasks)
	}

	res = f.try(t, "add_items", map[string]any{"tasks": []map[string]any{{"title": "x", "area": "work"}}})
	if !res.IsError {
		t.Error("an unknown area was accepted")
	}
}

func TestOverviewPlanAndTaskChanges(t *testing.T) {
	f := newFixture(t)
	var added connector.Added
	f.call(t, "add_items", map[string]any{"tasks": []map[string]any{
		{"title": "Email HUJI about the special exam date", "area": "huji"},
		{"title": "Pay the phone bill", "area": "home"},
		{"title": "Book flights", "area": "personal"},
	}}, &added)
	email, bill, flights := added.Tasks[0].ID, added.Tasks[1].ID, added.Tasks[2].ID

	var before connector.Overview
	f.call(t, "get_overview", map[string]any{}, &before)
	if before.Started || before.Today != "2026-10-26" || len(before.Unplanned) != 3 || len(before.Areas) != 5 {
		t.Errorf("overview before planning = %+v", before)
	}

	var set connector.PlanSet
	f.call(t, "set_plan", map[string]any{
		"must":   []map[string]any{{"task_id": email, "why": "the exam date can't wait"}},
		"should": []map[string]any{{"task_id": bill}},
		"note":   "One email, then the bill.",
	}, &set)
	if set.Tasks != 2 {
		t.Errorf("plan set = %+v", set)
	}

	var after connector.Overview
	f.call(t, "get_overview", map[string]any{}, &after)
	if !after.Started || after.Opening != "One email, then the bill." || len(after.Plan["must"]) != 1 ||
		after.Plan["must"][0].ID != email || after.Plan["must"][0].Why != "the exam date can't wait" ||
		len(after.Unplanned) != 1 || after.Unplanned[0].ID != flights {
		t.Errorf("overview after planning = %+v", after)
	}

	if res := f.try(t, "set_plan", map[string]any{"must": []map[string]any{{"task_id": 999}}}); !res.IsError {
		t.Error("a plan with an unknown task was accepted")
	}

	var updated connector.TaskOut
	f.call(t, "update_task", map[string]any{"task_id": bill, "area": "", "due_on": "2026-10-30"}, &updated)
	if updated.Area != nil || updated.DueOn == nil || *updated.DueOn != "2026-10-30" || updated.Title != "Pay the phone bill" {
		t.Errorf("updated = %+v", updated)
	}

	f.call(t, "set_task_status", map[string]any{"task_id": email, "status": "done"}, nil)
	var open connector.TaskList
	f.call(t, "list_tasks", map[string]any{"area": "huji"}, &open)
	if len(open.Tasks) != 0 {
		t.Errorf("open HUJI tasks = %+v, want none", open.Tasks)
	}

	if res := f.try(t, "push_task", map[string]any{"task_id": flights, "until": "2026-10-26"}); !res.IsError {
		t.Error("a push to today was accepted")
	}
	f.call(t, "push_task", map[string]any{"task_id": flights, "until": "2026-11-02"}, nil)
	var later connector.Overview
	f.call(t, "get_overview", map[string]any{}, &later)
	if later.Later != 1 || len(later.Unplanned) != 0 {
		t.Errorf("after pushing, overview = %+v", later)
	}
}

func TestIdeasThroughTheConnector(t *testing.T) {
	f := newFixture(t)
	f.call(t, "add_items", map[string]any{"ideas": []map[string]any{
		{"title": "Podcast about exchange semesters", "area": "personal"},
		{"title": "Look up how Matter bridges work", "area": "projects"},
	}}, nil)

	var ideas connector.IdeaList
	f.call(t, "list_ideas", map[string]any{"area": "projects"}, &ideas)
	if len(ideas.Ideas) != 1 || ideas.Ideas[0].Title != "Look up how Matter bridges work" || ideas.Ideas[0].AddedOn != "2026-10-26" {
		t.Fatalf("ideas = %+v", ideas)
	}

	var task connector.TaskOut
	f.call(t, "idea_to_task", map[string]any{"idea_id": ideas.Ideas[0].ID}, &task)
	if task.Title != "Look up how Matter bridges work" || task.Area == nil || *task.Area != domain.AreaProjects {
		t.Errorf("task = %+v", task)
	}
	f.call(t, "list_ideas", map[string]any{}, &ideas)
	if len(ideas.Ideas) != 1 {
		t.Fatalf("ideas left = %+v", ideas)
	}
	f.call(t, "delete_idea", map[string]any{"idea_id": ideas.Ideas[0].ID}, nil)
	f.call(t, "list_ideas", map[string]any{}, &ideas)
	if len(ideas.Ideas) != 0 {
		t.Errorf("ideas after delete = %+v", ideas)
	}
}

func TestUndoWithNothingToUndo(t *testing.T) {
	f := newFixture(t)
	if res := f.try(t, "undo_last", map[string]any{}); !res.IsError || !strings.Contains(text(res), "nothing to undo") {
		t.Errorf("result = %+v", res)
	}
}
