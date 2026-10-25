// Package domain holds the types shared by the service and API layers.
package domain

import (
	"encoding/json"
	"time"
)

// Actor is who made a change.
type Actor string

const (
	ActorUser  Actor = "user"
	ActorAgent Actor = "agent"
)

func (a Actor) Valid() bool { return a == ActorUser || a == ActorAgent }

type Status string

const (
	StatusOpen    Status = "open"
	StatusDone    Status = "done"
	StatusDropped Status = "dropped"
)

func (s Status) Valid() bool { return s == StatusOpen || s == StatusDone || s == StatusDropped }

type Size string

const (
	SizeS Size = "S"
	SizeM Size = "M"
	SizeL Size = "L"
)

func (s Size) Valid() bool { return s == SizeS || s == SizeM || s == SizeL }

type Energy string

const (
	EnergyLow  Energy = "low"
	EnergyMed  Energy = "med"
	EnergyHigh Energy = "high"
)

func (e Energy) Valid() bool { return e == EnergyLow || e == EnergyMed || e == EnergyHigh }

// Area is the part of life a task belongs to. The set is fixed: Areas lists every one.
type Area string

const (
	AreaHUJI     Area = "huji"
	AreaUofT     Area = "uoft"
	AreaHome     Area = "home"
	AreaPersonal Area = "personal"
	AreaProjects Area = "projects"
)

// AreaInfo describes an area: its label for people and what belongs in it, which the model reads
// when it picks one.
type AreaInfo struct {
	ID     Area   `json:"id"`
	Label  string `json:"label"`
	Covers string `json:"covers"`
}

// Areas lists every area, in the order they're shown.
var Areas = []AreaInfo{
	{AreaHUJI, "HUJI", "the home degree at Hebrew University: exams, credit transfers, admin"},
	{AreaUofT, "UofT", "the exchange semester at the University of Toronto: courses, readings, assignments, admin"},
	{AreaHome, "Home / life admin", "errands, bills, bureaucracy, shopping, the apartment"},
	{AreaPersonal, "Personal & social", "friends, family, plans, trips, health, hobbies"},
	{AreaProjects, "Side projects", "tech setups, research help and other self-directed projects"},
}

func (a Area) Valid() bool {
	for _, info := range Areas {
		if info.ID == a {
			return true
		}
	}
	return false
}

// AreaIDs returns the IDs of every area, in order.
func AreaIDs() []string {
	ids := make([]string, len(Areas))
	for i, info := range Areas {
		ids[i] = string(info.ID)
	}
	return ids
}

// Date is a calendar date in YYYY-MM-DD form.
type Date string

func (d Date) Valid() bool {
	_, err := time.Parse(time.DateOnly, string(d))
	return err == nil
}

// Ref is a link or key pulled out of a task's text, such as a ticket key or a URL.
type Ref struct {
	Label string `json:"label"`
	URL   string `json:"url,omitempty"`
}

type Task struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Notes     string     `json:"notes"`
	RawText   string     `json:"raw_text"`
	Size      *Size      `json:"size"`
	Energy    *Energy    `json:"energy"`
	Area      *Area      `json:"area"`
	Feeling   string     `json:"feeling"`
	DueOn     *Date      `json:"due_on"`
	NotBefore *Date      `json:"not_before"`
	Status    Status     `json:"status"`
	Refs      []Ref      `json:"refs"`
	CreatedBy Actor      `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// Idea is a thought worth keeping that isn't something to do: an essay angle, a thing to look up
// someday, a project idea. Ideas never enter a plan. They wait in the idea pile until the user
// browses it, and one can be turned into a task.
type Idea struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Notes     string     `json:"notes"`
	Area      *Area      `json:"area"`
	RawText   string     `json:"raw_text"`
	CreatedBy Actor      `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// Blocker says TaskID cannot start until BlockedByID is finished.
type Blocker struct {
	TaskID      int64 `json:"task_id"`
	BlockedByID int64 `json:"blocked_by_id"`
}

// Event is anything at a fixed time.
type Event struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	StartsAt  time.Time  `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	RawText   string     `json:"raw_text"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
	// Source is the integration the event was pulled from, and SourceKey its identity there. Both
	// are empty for an event made in whatnow.
	Source    string `json:"source"`
	SourceKey string `json:"source_key"`
}

// Activity is one recorded change. Before is null for a creation and After is null for a removal.
type Activity struct {
	ID        int64           `json:"id"`
	BatchID   int64           `json:"batch_id"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  int64           `json:"entity_id"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	CreatedAt time.Time       `json:"created_at"`
}

// Pace is how hard the user wants to push today, from 1 to 5. The labels can change; there are
// always five levels.
type Pace int

func (p Pace) Valid() bool { return p >= 1 && p <= 5 }

var paceLabels = [...]string{"running on fumes", "gentle", "steady", "full steam", "go hard"}

func (p Pace) Label() string {
	if !p.Valid() {
		return ""
	}
	return paceLabels[p-1]
}

type Day struct {
	ID        int64      `json:"id"`
	Date      Date       `json:"date"`
	Pace      Pace       `json:"pace"`
	MoodNote  string     `json:"mood_note"`
	StartedAt time.Time  `json:"started_at"`
	WrappedAt *time.Time `json:"wrapped_at"`
	// TomorrowNote is what the user left for the next day when wrapping up.
	TomorrowNote string `json:"tomorrow_note"`
	// Summary is the light model's plain notes on the day, written when it's wrapped. The planner
	// reads it on later days; the user never sees it.
	Summary string `json:"summary"`
	// Reflection is the day told to the user, in their assistant's voice, written from Summary.
	Reflection string `json:"reflection"`
}

type Tier string

const (
	TierMust   Tier = "must"
	TierShould Tier = "should"
	TierCould  Tier = "could"
)

func (t Tier) Valid() bool { return t == TierMust || t == TierShould || t == TierCould }

// Trigger is what caused a plan to be made.
type Trigger string

const (
	TriggerMorning Trigger = "morning"
	TriggerPace    Trigger = "pace"
	TriggerEdit    Trigger = "edit"
	TriggerAgent   Trigger = "agent"
	// TriggerUser is a replan the user asked for from Today.
	TriggerUser Trigger = "user"
	// TriggerMove is the user moving one task to another tier, or out of the plan, by hand.
	TriggerMove Trigger = "move"
)

func (t Trigger) Valid() bool {
	return t == TriggerMorning || t == TriggerPace || t == TriggerEdit || t == TriggerAgent || t == TriggerUser || t == TriggerMove
}

type PlanEntry struct {
	TaskID    int64  `json:"task_id"`
	Tier      Tier   `json:"tier"`
	Rationale string `json:"rationale"`
}

// Plan is one version of a day's plan. Entries are in order within each tier. Fallback means
// code built the plan because the model couldn't.
type Plan struct {
	ID        int64       `json:"id"`
	DayID     int64       `json:"day_id"`
	Version   int         `json:"version"`
	Trigger   Trigger     `json:"trigger"`
	Reason    string      `json:"reason"`
	Opening   string      `json:"opening"`
	Entries   []PlanEntry `json:"entries"`
	Fallback  bool        `json:"fallback"`
	CreatedAt time.Time   `json:"created_at"`
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ToolUse is one tool the agent ran in a reply, shown as a chip. Error is set when it failed.
type ToolUse struct {
	Tool  string `json:"tool"`
	Label string `json:"label"`
	Error string `json:"error"`
}

// Message is one chat message. BatchID is the reply's changes, or 0 when it changed nothing. Error
// is set on a reply the provider couldn't give, and Text then says so in plain words.
type Message struct {
	ID        int64     `json:"id"`
	Date      Date      `json:"date"`
	Role      Role      `json:"role"`
	Text      string    `json:"text"`
	Tools     []ToolUse `json:"tools"`
	BatchID   int64     `json:"batch_id"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

// ChatFold is the running summary of a date's older chat. Messages up to ThroughID are in it.
type ChatFold struct {
	Summary   string `json:"summary"`
	ThroughID int64  `json:"through_id"`
}
