package planner_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/llm/fake"
	"github.com/jameynakama/whatnow/internal/service"
	"github.com/jameynakama/whatnow/internal/settings"
)

// quizDay is a started day whose plan is written around a quiz: study comes after it.
type quizDay struct {
	quiz, study, laundry domain.Task
	plan                 domain.Plan
}

func (f *fixture) quizDay(t *testing.T, fallback bool) quizDay {
	t.Helper()
	q := quizDay{
		quiz:    f.task(t, service.TaskFields{Title: "take the quiz"}),
		study:   f.task(t, service.TaskFields{Title: "study for the midterm"}),
		laundry: f.task(t, service.TaskFields{Title: "laundry"}),
	}
	day := f.start(t, 3, "")
	plan, err := f.svc.SavePlan(context.Background(), domain.Plan{
		DayID:   day.ID,
		Trigger: domain.TriggerMorning,
		Opening: "Get the quiz done first, then study after the quiz.",
		Entries: []domain.PlanEntry{
			{TaskID: q.quiz.ID, Tier: domain.TierMust, Rationale: "It closes at noon."},
			{TaskID: q.study.ID, Tier: domain.TierMust, Rationale: "Do it after the quiz."},
			{TaskID: q.laundry.ID, Tier: domain.TierCould, Rationale: "If there's room."},
		},
		Fallback: fallback,
	})
	if err != nil {
		t.Fatal(err)
	}
	q.plan = plan
	return q
}

func TestFinishingAPlannedTaskRefreshesThePlansWords(t *testing.T) {
	f := newFixture(t)
	q := f.quizDay(t, false)
	f.model.replies = []any{map[string]any{
		"entries": []any{map[string]any{"task_id": q.study.ID, "rationale": "The quiz is done, so this is next."}},
		"opening": "Quiz done. Studying is up next.",
	}}
	plans := f.run(t)

	f.finish(t, q.quiz)

	plan := nextPlan(t, plans)
	if plan.Version != 2 || plan.Trigger != domain.TriggerRefresh || plan.Reason != `#1 "take the quiz" was finished` {
		t.Errorf("plan = %+v", plan)
	}
	if plan.Opening != "Quiz done. Studying is up next." || plan.Fallback {
		t.Errorf("opening = %q, fallback = %v", plan.Opening, plan.Fallback)
	}
	// Same tasks, same tiers, same order; only the stale line changed.
	want := []domain.PlanEntry{
		{TaskID: q.quiz.ID, Tier: domain.TierMust, Rationale: "It closes at noon."},
		{TaskID: q.study.ID, Tier: domain.TierMust, Rationale: "The quiz is done, so this is next."},
		{TaskID: q.laundry.ID, Tier: domain.TierCould, Rationale: "If there's room."},
	}
	if diff := cmp.Diff(want, plan.Entries); diff != "" {
		t.Errorf("entries (-want +got):\n%s", diff)
	}

	if len(f.model.sent) != 1 {
		t.Fatalf("%d model calls", len(f.model.sent))
	}
	req := f.model.sent[0]
	if req.SchemaName != "plan_refresh" || req.Model != settings.Defaults().Anthropic.LightModel {
		t.Errorf("request schema %q, model %q", req.SchemaName, req.Model)
	}
	for _, s := range []string{
		`What changed: #1 "take the quiz" was finished.`,
		`The plan's note now: "Get the quiz done first, then study after the quiz."`,
		`#2 study for the midterm: "Do it after the quiz."`,
		"#3 laundry",
	} {
		if !strings.Contains(req.User, s) {
			t.Errorf("prompt is missing %q:\n%s", s, req.User)
		}
	}
	if strings.Contains(req.User, "#1 take the quiz") {
		t.Errorf("the finished task is still offered as a card:\n%s", req.User)
	}
}

func TestUndoingAFinishRefreshesTheWordsAgain(t *testing.T) {
	f := newFixture(t)
	q := f.quizDay(t, false)
	f.model.replies = []any{
		map[string]any{"entries": []any{}, "opening": "Quiz done."},
		map[string]any{"entries": []any{}, "opening": "The quiz is back on. Do it first."},
	}
	plans := f.run(t)

	batch := f.write(t, func(b int64) error {
		_, err := f.svc.SetTaskStatus(context.Background(), b, q.quiz.ID, domain.StatusDone)
		return err
	})
	if plan := nextPlan(t, plans); plan.Opening != "Quiz done." {
		t.Fatalf("after finishing: %+v", plan)
	}
	if err := f.svc.Undo(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	f.hub.PublishUndone(batch)
	plan := nextPlan(t, plans)
	if plan.Version != 3 || plan.Reason != `#1 "take the quiz" is back on today's list` || plan.Opening != "The quiz is back on. Do it first." {
		t.Errorf("after the undo: %+v", plan)
	}
	if !strings.Contains(f.model.sent[1].User, "#1 take the quiz") {
		t.Errorf("the reopened task isn't offered as a card:\n%s", f.model.sent[1].User)
	}
}

func TestARefreshThatWontDoLeavesThePlan(t *testing.T) {
	f := newFixture(t)
	q := f.quizDay(t, false)
	f.finish(t, q.quiz)
	// Both tries rewrite the finished task's card, which no longer shows.
	bad := map[string]any{"entries": []any{map[string]any{"task_id": q.quiz.ID, "rationale": "Done!"}}, "opening": "Nice."}
	f.model.replies = []any{bad, bad}

	_, changed, err := f.planner.Refresh(context.Background(), "a test")
	if err == nil || changed || !strings.Contains(err.Error(), "is not a card still showing") {
		t.Fatalf("changed = %v, err = %v", changed, err)
	}
	if !strings.Contains(f.model.sent[1].User, "It was rejected") {
		t.Errorf("no retry with the problem attached:\n%s", f.model.sent[1].User)
	}
	if latest, err := f.svc.LatestPlan(context.Background(), q.plan.DayID); err != nil || latest.Version != 1 {
		t.Errorf("latest = %+v, %v", latest, err)
	}
}

func TestACodeMadePlanOrNoProviderIsNotRefreshed(t *testing.T) {
	f := newFixture(t)
	q := f.quizDay(t, true)
	f.finish(t, q.quiz)
	// No replies are scripted: any model call fails the test.
	if _, changed, err := f.planner.Refresh(context.Background(), "a test"); err != nil || changed {
		t.Errorf("fallback plan: changed = %v, err = %v", changed, err)
	}

	f = newFixture(t)
	q = f.quizDay(t, false)
	f.configured = false
	f.finish(t, q.quiz)
	if _, changed, err := f.planner.Refresh(context.Background(), "a test"); !errors.Is(err, settings.ErrNotConfigured) || changed {
		t.Errorf("no provider: changed = %v, err = %v", changed, err)
	}
}

func TestADroppedMustReplansInsteadOfRefreshing(t *testing.T) {
	f := newFixture(t)
	f.configured = false
	q := f.quizDay(t, false)
	plans := f.run(t)

	f.write(t, func(b int64) error {
		_, err := f.svc.SetTaskStatus(context.Background(), b, q.quiz.ID, domain.StatusDropped)
		return err
	})
	if plan := nextPlan(t, plans); plan.Trigger != domain.TriggerEdit {
		t.Errorf("plan = %+v", plan)
	}
}

func TestTheFakeRefreshesTheNote(t *testing.T) {
	f := newFixture(t)
	f.slow = fake.Client{}
	q := f.quizDay(t, false)
	f.finish(t, q.quiz)

	plan, changed, err := f.planner.Refresh(context.Background(), "a test")
	if err != nil || !changed || plan.Opening != fake.Refreshed {
		t.Fatalf("plan = %+v, changed = %v, err = %v", plan, changed, err)
	}
	if plan.Entries[1].Rationale != "Do it after the quiz." {
		t.Errorf("entries = %+v", plan.Entries)
	}
}
