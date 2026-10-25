// Package connector is the planner as a remote MCP server, which Claude reaches as a custom
// connector in the Claude app. Claude does the thinking on the user's subscription: it sorts a
// dictated brain dump itself and files the result here, reads the day, and can set the plan. So
// nothing here calls a model.
package connector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/planner"
	"github.com/jameynakama/whatnow/internal/service"
	"github.com/jameynakama/whatnow/internal/stream"
)

// Deps are the parts of one user's App the connector works on.
type Deps struct {
	Service *service.Service
	Hub     *stream.Hub
	Planner *planner.Planner
	Loc     *time.Location
	Now     func() time.Time
}

const instructions = `This is the user's own planner, a web app for someone with ADHD. Tasks, ideas, and events each belong to an area: huji (the home degree at Hebrew University), uoft (the exchange semester at the University of Toronto), home (errands, bills, bureaucracy, shopping), personal (friends, family, plans, trips, health, hobbies), or projects (side projects).

When the user dictates a brain dump, sort it yourself and file everything with one add_items call:
- tasks are things to do. Give each an area, and a due_on date when they said one.
- ideas are thoughts worth keeping that aren't things to do: essay angles, project ideas, things to look up someday. They go to the idea pile and never into the plan.
- events are things at a fixed time.
Anything at a fixed time also belongs in the user's Apple Calendar, and anything they need an alert for at a set time belongs in Apple Reminders. If you can create calendar events and reminders on their device, do that too.

Notes the user typed into the app or sent from their phone wait unsorted in the inbox. When they ask you to tidy up, and before you plan their day, call get_inbox and sort each note with sort_note, the same way you sort a dump.

Call get_overview before planning or answering questions about their day. Every change can be undone with undo_last. Keep replies short; the user sees the result in the app.`

// Handler serves the connector over streamable HTTP. It keeps no sessions: each request stands
// alone, which suits a server that opens and closes the user's App as requests come and go.
func Handler(d Deps) http.Handler {
	s := newServer(d)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			// The SDK refuses a request that arrives on a loopback address with a public Host,
			// which is every request here: the server listens on 127.0.0.1 behind Caddy. The
			// hosted front door already checks the Host against the public URL, and the
			// connector key, before a request gets this far.
			DisableLocalhostProtection: true,
		})
}

func newServer(d Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "planner", Title: "Planner", Version: "1"},
		&mcp.ServerOptions{Instructions: instructions})
	c := &conn{Deps: d}
	mcp.AddTool(s, &mcp.Tool{Name: "get_overview",
		Description: "Today's date and time, today's plan by tier, open tasks outside the plan, this week's events, the areas, and how many ideas are on the pile."},
		c.overview)
	mcp.AddTool(s, &mcp.Tool{Name: "list_tasks",
		Description: "List tasks by status (open, done, or dropped; open if empty), optionally in one area."}, c.listTasks)
	mcp.AddTool(s, &mcp.Tool{Name: "add_items",
		Description: "File what you sorted out of a brain dump, all at once: tasks, ideas, and events. One call is one undo."},
		c.addItems)
	mcp.AddTool(s, &mcp.Tool{Name: "get_inbox",
		Description: "Unsorted notes the user typed into the app or sent from their phone, oldest first, each still a single raw task."},
		c.inbox)
	mcp.AddTool(s, &mcp.Tool{Name: "sort_note",
		Description: "Replace one unsorted inbox note with the tasks, ideas, and events you sorted out of it. The note itself goes away. One call is one undo."},
		c.sortNote)
	mcp.AddTool(s, &mcp.Tool{Name: "update_task",
		Description: "Change some of a task's fields. Leave out a field to keep it; send an empty string to clear it. To put a task off, use push_task."},
		c.updateTask)
	mcp.AddTool(s, &mcp.Tool{Name: "set_task_status",
		Description: "Mark a task done, drop it (stop carrying it without doing it), or reopen it."}, c.setStatus)
	mcp.AddTool(s, &mcp.Tool{Name: "push_task",
		Description: "Put a task off until a later date. Pushes are counted, and a task pushed again and again says something."},
		c.pushTask)
	mcp.AddTool(s, &mcp.Tool{Name: "set_plan",
		Description: "Set today's whole plan: open task ids in must, should, and could, most important first. Tasks left out stay unplanned. Starts the day if it hasn't started."},
		c.setPlan)
	mcp.AddTool(s, &mcp.Tool{Name: "list_ideas",
		Description: "List the idea pile, newest first, optionally in one area. Only when the user asks about their ideas."},
		c.listIdeas)
	mcp.AddTool(s, &mcp.Tool{Name: "idea_to_task",
		Description: "Turn an idea into an open task and take it off the pile."}, c.ideaToTask)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_idea", Description: "Remove an idea from the pile."}, c.deleteIdea)
	mcp.AddTool(s, &mcp.Tool{Name: "delete_event", Description: "Remove an event from the planner."}, c.deleteEvent)
	mcp.AddTool(s, &mcp.Tool{Name: "undo_last",
		Description: "Take back the most recent change made through this connector, as a whole."}, c.undoLast)
	return s
}

type conn struct{ Deps }

// write runs op in a new batch made by the assistant and announces it, so an open app updates.
func (c *conn) write(ctx context.Context, op func(batch int64) error) error {
	batch, err := c.Service.NewBatch(ctx, domain.ActorAgent)
	if err != nil {
		return err
	}
	if err := op(batch); err != nil {
		if acts, _ := c.Service.BatchActivity(ctx, batch); len(acts) > 0 {
			// A partly applied batch is taken back, so a failed call changes nothing.
			if undoErr := c.Service.Undo(ctx, batch); undoErr != nil {
				return errors.Join(err, undoErr)
			}
		}
		return err
	}
	acts, err := c.Service.BatchActivity(ctx, batch)
	if err != nil {
		return err
	}
	c.Hub.PublishBatch(batch, stream.SourceClaude, acts)
	return nil
}

// TaskOut is a task as Claude reads it.
type TaskOut struct {
	ID        int64          `json:"id"`
	Title     string         `json:"title"`
	Area      *domain.Area   `json:"area,omitempty"`
	Status    domain.Status  `json:"status"`
	Notes     string         `json:"notes,omitempty"`
	DueOn     *domain.Date   `json:"due_on,omitempty"`
	NotBefore *domain.Date   `json:"not_before,omitempty"`
	Size      *domain.Size   `json:"size,omitempty"`
	Energy    *domain.Energy `json:"energy,omitempty"`
	Feeling   string         `json:"feeling,omitempty"`
}

func taskOut(t domain.Task) TaskOut {
	return TaskOut{ID: t.ID, Title: t.Title, Area: t.Area, Status: t.Status, Notes: t.Notes, DueOn: t.DueOn,
		NotBefore: t.NotBefore, Size: t.Size, Energy: t.Energy, Feeling: t.Feeling}
}

// EventOut is an event as Claude reads it, in the user's local time.
type EventOut struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at,omitempty"`
}

// IdeaOut is an idea as Claude reads it.
type IdeaOut struct {
	ID      int64        `json:"id"`
	Title   string       `json:"title"`
	Area    *domain.Area `json:"area,omitempty"`
	Notes   string       `json:"notes,omitempty"`
	AddedOn string       `json:"added_on"`
}

const localTime = "2006-01-02T15:04"

func (c *conn) eventOut(e domain.Event) EventOut {
	out := EventOut{ID: e.ID, Title: e.Title, StartsAt: e.StartsAt.In(c.Loc).Format(localTime)}
	if e.EndsAt != nil {
		out.EndsAt = e.EndsAt.In(c.Loc).Format(localTime)
	}
	return out
}

func (c *conn) ideaOut(i domain.Idea) IdeaOut {
	return IdeaOut{ID: i.ID, Title: i.Title, Area: i.Area, Notes: i.Notes, AddedOn: i.CreatedAt.In(c.Loc).Format(time.DateOnly)}
}

// Empty is the input of a tool that takes none.
type Empty struct{}

// PlannedTask is a task in a tier of today's plan.
type PlannedTask struct {
	TaskOut
	Why string `json:"why,omitempty"`
}

// Overview is get_overview's result.
type Overview struct {
	Now       string                   `json:"now"`
	Today     domain.Date              `json:"today"`
	Started   bool                     `json:"day_started"`
	Pace      string                   `json:"pace,omitempty"`
	Opening   string                   `json:"plan_note,omitempty"`
	Plan      map[string][]PlannedTask `json:"plan"`
	Unplanned []TaskOut                `json:"open_tasks_not_in_plan"`
	Later     int                      `json:"tasks_put_off_until_later"`
	Events    []EventOut               `json:"events_next_7_days"`
	Areas     []domain.AreaInfo        `json:"areas"`
	Ideas     int                      `json:"ideas_on_pile"`
}

func (c *conn) overview(ctx context.Context, _ *mcp.CallToolRequest, _ Empty) (*mcp.CallToolResult, Overview, error) {
	today, err := c.Planner.Today(ctx)
	if err != nil {
		return nil, Overview{}, err
	}
	tasks, err := c.Service.ListTasks(ctx)
	if err != nil {
		return nil, Overview{}, err
	}
	byID := map[int64]domain.Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	now := c.Now().In(c.Loc)
	out := Overview{Now: now.Format("Monday " + localTime), Today: today.Date, Started: today.Day != nil,
		Plan: map[string][]PlannedTask{}, Unplanned: []TaskOut{}, Events: []EventOut{}, Areas: domain.Areas}
	if today.Day != nil {
		out.Pace = today.Day.Pace.Label()
	}
	planned := map[int64]bool{}
	if today.Plan != nil {
		out.Opening = today.Plan.Opening
		for _, e := range today.Plan.Entries {
			if t, ok := byID[e.TaskID]; ok && t.Status == domain.StatusOpen {
				out.Plan[string(e.Tier)] = append(out.Plan[string(e.Tier)], PlannedTask{TaskOut: taskOut(t), Why: e.Rationale})
				planned[e.TaskID] = true
			}
		}
	}
	eligible := map[int64]bool{}
	for _, id := range today.Unplanned {
		eligible[id] = true
		if t, ok := byID[id]; ok {
			out.Unplanned = append(out.Unplanned, taskOut(t))
		}
	}
	for _, t := range tasks {
		if t.Status == domain.StatusOpen && !planned[t.ID] && !eligible[t.ID] {
			out.Later++
		}
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, c.Loc)
	events, err := c.Service.ListEvents(ctx, start, start.AddDate(0, 0, 7))
	if err != nil {
		return nil, Overview{}, err
	}
	for _, e := range events {
		out.Events = append(out.Events, c.eventOut(e))
	}
	ideas, err := c.Service.ListIdeas(ctx)
	if err != nil {
		return nil, Overview{}, err
	}
	out.Ideas = len(ideas)
	return nil, out, nil
}

// ListTasksIn is list_tasks' input.
type ListTasksIn struct {
	Status string `json:"status,omitempty" jsonschema:"open, done, or dropped; open if empty"`
	Area   string `json:"area,omitempty" jsonschema:"only tasks in this area: huji, uoft, home, personal, or projects"`
}

// TaskList wraps a list, since a tool's result must be an object.
type TaskList struct {
	Tasks []TaskOut `json:"tasks"`
}

func (c *conn) listTasks(ctx context.Context, _ *mcp.CallToolRequest, in ListTasksIn) (*mcp.CallToolResult, TaskList, error) {
	status := domain.Status(in.Status)
	if status == "" {
		status = domain.StatusOpen
	}
	if !status.Valid() {
		return nil, TaskList{}, fmt.Errorf("status %q is not open, done, or dropped", in.Status)
	}
	area, err := areaFilter(in.Area)
	if err != nil {
		return nil, TaskList{}, err
	}
	tasks, err := c.Service.ListTasks(ctx)
	if err != nil {
		return nil, TaskList{}, err
	}
	out := TaskList{Tasks: []TaskOut{}}
	for _, t := range tasks {
		if t.Status == status && (area == "" || (t.Area != nil && *t.Area == area)) {
			out.Tasks = append(out.Tasks, taskOut(t))
		}
	}
	return nil, out, nil
}

func areaFilter(s string) (domain.Area, error) {
	a := domain.Area(strings.TrimSpace(s))
	if a != "" && !a.Valid() {
		return "", fmt.Errorf("area %q is not one of %s", s, strings.Join(domain.AreaIDs(), ", "))
	}
	return a, nil
}

// optional turns an empty string into an unset field.
func optional[T ~string](s string) *T {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	v := T(s)
	return &v
}

// NewTask is a task in add_items.
type NewTask struct {
	Title     string `json:"title" jsonschema:"short and concrete, in the user's words where you can"`
	Notes     string `json:"notes,omitempty" jsonschema:"useful detail that doesn't fit the title"`
	Area      string `json:"area,omitempty" jsonschema:"huji, uoft, home, personal, or projects"`
	DueOn     string `json:"due_on,omitempty" jsonschema:"when it's due, as YYYY-MM-DD"`
	NotBefore string `json:"not_before,omitempty" jsonschema:"the first day it can be done, as YYYY-MM-DD"`
	Size      string `json:"size,omitempty" jsonschema:"S under an hour, M one to three hours, L more"`
	Energy    string `json:"energy,omitempty" jsonschema:"mental effort: low, med, or high"`
	Feeling   string `json:"feeling,omitempty" jsonschema:"how they feel about it, in their words, only if they said"`
}

// NewIdea is an idea in add_items.
type NewIdea struct {
	Title string `json:"title" jsonschema:"the idea, short, in the user's words where you can"`
	Notes string `json:"notes,omitempty" jsonschema:"useful detail that doesn't fit the title"`
	Area  string `json:"area,omitempty" jsonschema:"huji, uoft, home, personal, or projects"`
}

// NewEvent is an event in add_items.
type NewEvent struct {
	Title    string `json:"title" jsonschema:"what it is"`
	StartsAt string `json:"starts_at" jsonschema:"local start time as YYYY-MM-DDTHH:MM"`
	EndsAt   string `json:"ends_at,omitempty" jsonschema:"local end time as YYYY-MM-DDTHH:MM, if it has a set end"`
}

// AddItemsIn is add_items' input.
type AddItemsIn struct {
	Tasks   []NewTask  `json:"tasks,omitempty"`
	Ideas   []NewIdea  `json:"ideas,omitempty"`
	Events  []NewEvent `json:"events,omitempty"`
	RawText string     `json:"raw_text,omitempty" jsonschema:"the user's words as dictated, kept with each item for reference"`
}

// Added is what add_items made.
type Added struct {
	Tasks  []TaskOut  `json:"tasks"`
	Ideas  []IdeaOut  `json:"ideas"`
	Events []EventOut `json:"events"`
}

func (c *conn) addItems(ctx context.Context, _ *mcp.CallToolRequest, in AddItemsIn) (*mcp.CallToolResult, Added, error) {
	if len(in.Tasks)+len(in.Ideas)+len(in.Events) == 0 {
		return nil, Added{}, errors.New("nothing to add: send at least one task, idea, or event")
	}
	var out Added
	err := c.write(ctx, func(batch int64) error {
		var err error
		out, err = c.file(ctx, batch, in)
		return err
	})
	if err != nil {
		return nil, Added{}, err
	}
	return nil, out, nil
}

// file adds a dump's sorted items in batch, each keeping the user's words as its raw text.
func (c *conn) file(ctx context.Context, batch int64, in AddItemsIn) (Added, error) {
	out := Added{Tasks: []TaskOut{}, Ideas: []IdeaOut{}, Events: []EventOut{}}
	for _, n := range in.Tasks {
		t, err := c.Service.CreateTask(ctx, batch, service.TaskFields{
			Title: n.Title, Notes: n.Notes, RawText: in.RawText, Area: optional[domain.Area](n.Area),
			DueOn: optional[domain.Date](n.DueOn), NotBefore: optional[domain.Date](n.NotBefore),
			Size: optional[domain.Size](n.Size), Energy: optional[domain.Energy](n.Energy), Feeling: n.Feeling,
		})
		if err != nil {
			return out, fmt.Errorf("task %q: %w", n.Title, err)
		}
		out.Tasks = append(out.Tasks, taskOut(t))
	}
	for _, n := range in.Ideas {
		i, err := c.Service.CreateIdea(ctx, batch, service.IdeaFields{Title: n.Title, Notes: n.Notes,
			Area: optional[domain.Area](n.Area), RawText: in.RawText})
		if err != nil {
			return out, fmt.Errorf("idea %q: %w", n.Title, err)
		}
		out.Ideas = append(out.Ideas, c.ideaOut(i))
	}
	for _, n := range in.Events {
		f, err := c.eventFields(n)
		if err != nil {
			return out, err
		}
		f.RawText = in.RawText
		e, err := c.Service.CreateEvent(ctx, batch, f)
		if err != nil {
			return out, fmt.Errorf("event %q: %w", n.Title, err)
		}
		out.Events = append(out.Events, c.eventOut(e))
	}
	return out, nil
}

func (c *conn) eventFields(n NewEvent) (service.EventFields, error) {
	start, err := time.ParseInLocation(localTime, strings.TrimSpace(n.StartsAt), c.Loc)
	if err != nil {
		return service.EventFields{}, fmt.Errorf("event %q: starts_at %q is not YYYY-MM-DDTHH:MM", n.Title, n.StartsAt)
	}
	f := service.EventFields{Title: n.Title, StartsAt: start}
	if strings.TrimSpace(n.EndsAt) != "" {
		end, err := time.ParseInLocation(localTime, strings.TrimSpace(n.EndsAt), c.Loc)
		if err != nil {
			return service.EventFields{}, fmt.Errorf("event %q: ends_at %q is not YYYY-MM-DDTHH:MM", n.Title, n.EndsAt)
		}
		f.EndsAt = &end
	}
	return f, nil
}

// UpdateTaskIn is update_task's input. A field left out stays as it is.
type UpdateTaskIn struct {
	TaskID    int64   `json:"task_id"`
	Title     *string `json:"title,omitempty"`
	Notes     *string `json:"notes,omitempty"`
	Area      *string `json:"area,omitempty" jsonschema:"huji, uoft, home, personal, or projects; empty clears it"`
	DueOn     *string `json:"due_on,omitempty" jsonschema:"YYYY-MM-DD; empty clears it"`
	NotBefore *string `json:"not_before,omitempty" jsonschema:"YYYY-MM-DD; empty clears it"`
	Size      *string `json:"size,omitempty" jsonschema:"S, M, or L; empty clears it"`
	Energy    *string `json:"energy,omitempty" jsonschema:"low, med, or high; empty clears it"`
	Feeling   *string `json:"feeling,omitempty"`
}

func (c *conn) updateTask(ctx context.Context, _ *mcp.CallToolRequest, in UpdateTaskIn) (*mcp.CallToolResult, TaskOut, error) {
	t, err := c.Service.GetTask(ctx, in.TaskID)
	if err != nil {
		return nil, TaskOut{}, err
	}
	f := service.TaskFields{Title: t.Title, Notes: t.Notes, Size: t.Size, Energy: t.Energy, Area: t.Area,
		Feeling: t.Feeling, DueOn: t.DueOn, NotBefore: t.NotBefore, Refs: t.Refs}
	if in.Title != nil {
		f.Title = *in.Title
	}
	if in.Notes != nil {
		f.Notes = *in.Notes
	}
	if in.Area != nil {
		f.Area = optional[domain.Area](*in.Area)
	}
	if in.DueOn != nil {
		f.DueOn = optional[domain.Date](*in.DueOn)
	}
	if in.NotBefore != nil {
		f.NotBefore = optional[domain.Date](*in.NotBefore)
	}
	if in.Size != nil {
		f.Size = optional[domain.Size](*in.Size)
	}
	if in.Energy != nil {
		f.Energy = optional[domain.Energy](*in.Energy)
	}
	if in.Feeling != nil {
		f.Feeling = *in.Feeling
	}
	err = c.write(ctx, func(batch int64) error {
		t, err = c.Service.UpdateTask(ctx, batch, in.TaskID, f)
		return err
	})
	return nil, taskOut(t), err
}

// SetStatusIn is set_task_status' input.
type SetStatusIn struct {
	TaskID int64  `json:"task_id"`
	Status string `json:"status" jsonschema:"done, dropped, or open"`
}

func (c *conn) setStatus(ctx context.Context, _ *mcp.CallToolRequest, in SetStatusIn) (*mcp.CallToolResult, TaskOut, error) {
	var t domain.Task
	err := c.write(ctx, func(batch int64) error {
		var err error
		t, err = c.Service.SetTaskStatus(ctx, batch, in.TaskID, domain.Status(in.Status))
		return err
	})
	return nil, taskOut(t), err
}

// PushTaskIn is push_task's input.
type PushTaskIn struct {
	TaskID int64  `json:"task_id"`
	Until  string `json:"until" jsonschema:"the first day to see it again, as YYYY-MM-DD, after today"`
}

func (c *conn) pushTask(ctx context.Context, _ *mcp.CallToolRequest, in PushTaskIn) (*mcp.CallToolResult, TaskOut, error) {
	until := domain.Date(in.Until)
	if until <= c.Planner.Date() {
		return nil, TaskOut{}, fmt.Errorf("until %q must be after today, %s", in.Until, c.Planner.Date())
	}
	var t domain.Task
	err := c.write(ctx, func(batch int64) error {
		var err error
		t, err = c.Service.PushTask(ctx, batch, in.TaskID, until)
		return err
	})
	return nil, taskOut(t), err
}

// PlanEntryIn is one task in a tier of set_plan.
type PlanEntryIn struct {
	TaskID int64  `json:"task_id"`
	Why    string `json:"why,omitempty" jsonschema:"a few words on why it's here, which the user sees on the card"`
}

// SetPlanIn is set_plan's input.
type SetPlanIn struct {
	Must    []PlanEntryIn `json:"must,omitempty" jsonschema:"what has to happen today, most important first"`
	Should  []PlanEntryIn `json:"should,omitempty" jsonschema:"what would be good to get done"`
	Could   []PlanEntryIn `json:"could,omitempty" jsonschema:"if there's room"`
	Opening string        `json:"note,omitempty" jsonschema:"a sentence or two for the top of the plan"`
	Reason  string        `json:"reason,omitempty" jsonschema:"what prompted this plan, in a few words"`
}

// PlanSet is set_plan's result.
type PlanSet struct {
	Version int `json:"version"`
	Tasks   int `json:"tasks_planned"`
}

func (c *conn) setPlan(ctx context.Context, _ *mcp.CallToolRequest, in SetPlanIn) (*mcp.CallToolResult, PlanSet, error) {
	var entries []domain.PlanEntry
	for _, tier := range []struct {
		tier    domain.Tier
		entries []PlanEntryIn
	}{{domain.TierMust, in.Must}, {domain.TierShould, in.Should}, {domain.TierCould, in.Could}} {
		for _, e := range tier.entries {
			entries = append(entries, domain.PlanEntry{TaskID: e.TaskID, Tier: tier.tier, Rationale: e.Why})
		}
	}
	reason := in.Reason
	if reason == "" {
		reason = "planned with Claude"
	}
	plan, err := c.Planner.SetPlan(ctx, entries, in.Opening, reason)
	if err != nil {
		return nil, PlanSet{}, err
	}
	return nil, PlanSet{Version: plan.Version, Tasks: len(plan.Entries)}, nil
}

// ListIdeasIn is list_ideas' input.
type ListIdeasIn struct {
	Area string `json:"area,omitempty" jsonschema:"only ideas in this area: huji, uoft, home, personal, or projects"`
}

// IdeaList wraps a list, since a tool's result must be an object.
type IdeaList struct {
	Ideas []IdeaOut `json:"ideas"`
}

func (c *conn) listIdeas(ctx context.Context, _ *mcp.CallToolRequest, in ListIdeasIn) (*mcp.CallToolResult, IdeaList, error) {
	area, err := areaFilter(in.Area)
	if err != nil {
		return nil, IdeaList{}, err
	}
	ideas, err := c.Service.ListIdeas(ctx)
	if err != nil {
		return nil, IdeaList{}, err
	}
	out := IdeaList{Ideas: []IdeaOut{}}
	for _, i := range ideas {
		if area == "" || (i.Area != nil && *i.Area == area) {
			out.Ideas = append(out.Ideas, c.ideaOut(i))
		}
	}
	return nil, out, nil
}

// IdeaIn names one idea.
type IdeaIn struct {
	IdeaID int64 `json:"idea_id"`
}

func (c *conn) ideaToTask(ctx context.Context, _ *mcp.CallToolRequest, in IdeaIn) (*mcp.CallToolResult, TaskOut, error) {
	var t domain.Task
	err := c.write(ctx, func(batch int64) error {
		var err error
		t, err = c.Service.IdeaToTask(ctx, batch, in.IdeaID)
		return err
	})
	return nil, taskOut(t), err
}

// Done says a removal or undo happened.
type Done struct {
	Done bool `json:"done"`
}

func (c *conn) deleteIdea(ctx context.Context, _ *mcp.CallToolRequest, in IdeaIn) (*mcp.CallToolResult, Done, error) {
	err := c.write(ctx, func(batch int64) error { return c.Service.DeleteIdea(ctx, batch, in.IdeaID) })
	return nil, Done{Done: err == nil}, err
}

// EventIn names one event.
type EventIn struct {
	EventID int64 `json:"event_id"`
}

func (c *conn) deleteEvent(ctx context.Context, _ *mcp.CallToolRequest, in EventIn) (*mcp.CallToolResult, Done, error) {
	err := c.write(ctx, func(batch int64) error { return c.Service.DeleteEvent(ctx, batch, in.EventID) })
	return nil, Done{Done: err == nil}, err
}

func (c *conn) undoLast(ctx context.Context, _ *mcp.CallToolRequest, _ Empty) (*mcp.CallToolResult, Done, error) {
	batch, err := c.Service.LastUndoableBatch(ctx, domain.ActorAgent, 0)
	if err != nil {
		return nil, Done{}, errors.New("there's nothing to undo")
	}
	if err := c.Service.Undo(ctx, batch); err != nil {
		return nil, Done{}, err
	}
	c.Hub.PublishUndone(batch)
	return nil, Done{Done: true}, nil
}

// Note is an unsorted note in the inbox.
type Note struct {
	ID         int64  `json:"id"`
	Text       string `json:"text"`
	CapturedAt string `json:"captured_at"`
}

// Inbox is get_inbox's result.
type Inbox struct {
	Notes []Note `json:"notes"`
}

// unsorted says whether a task is a note captured as raw text that nothing has tidied: the user
// made it, it has no area, and its title is still the raw text's first line.
func unsorted(t domain.Task) bool {
	return t.Status == domain.StatusOpen && t.CreatedBy == domain.ActorUser && t.RawText != "" && t.Area == nil &&
		t.Title == service.RawTitle(t.RawText)
}

func (c *conn) inbox(ctx context.Context, _ *mcp.CallToolRequest, _ Empty) (*mcp.CallToolResult, Inbox, error) {
	tasks, err := c.Service.ListTasks(ctx)
	if err != nil {
		return nil, Inbox{}, err
	}
	out := Inbox{Notes: []Note{}}
	for _, t := range tasks {
		if unsorted(t) {
			out.Notes = append(out.Notes, Note{ID: t.ID, Text: t.RawText, CapturedAt: t.CreatedAt.In(c.Loc).Format(localTime)})
		}
	}
	return nil, out, nil
}

// SortNoteIn is sort_note's input.
type SortNoteIn struct {
	NoteID int64      `json:"note_id"`
	Tasks  []NewTask  `json:"tasks,omitempty"`
	Ideas  []NewIdea  `json:"ideas,omitempty"`
	Events []NewEvent `json:"events,omitempty"`
}

func (c *conn) sortNote(ctx context.Context, _ *mcp.CallToolRequest, in SortNoteIn) (*mcp.CallToolResult, Added, error) {
	if len(in.Tasks)+len(in.Ideas)+len(in.Events) == 0 {
		return nil, Added{}, errors.New("sort the note into at least one task, idea, or event; to drop it instead, use set_task_status")
	}
	note, err := c.Service.GetTask(ctx, in.NoteID)
	if err != nil {
		return nil, Added{}, err
	}
	if !unsorted(note) {
		return nil, Added{}, fmt.Errorf("#%d isn't an unsorted note in the inbox", in.NoteID)
	}
	var out Added
	err = c.write(ctx, func(batch int64) error {
		var err error
		if out, err = c.file(ctx, batch, AddItemsIn{Tasks: in.Tasks, Ideas: in.Ideas, Events: in.Events, RawText: note.RawText}); err != nil {
			return err
		}
		return c.Service.DeleteTask(ctx, batch, note.ID)
	})
	if err != nil {
		return nil, Added{}, err
	}
	return nil, out, nil
}
