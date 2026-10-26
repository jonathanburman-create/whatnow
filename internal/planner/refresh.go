package planner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jameynakama/whatnow/internal/domain"
	js "github.com/jameynakama/whatnow/internal/jsonschema"
	"github.com/jameynakama/whatnow/internal/service"
	"github.com/jameynakama/whatnow/internal/settings"
	"github.com/jameynakama/whatnow/internal/stream"
)

// refreshSchema asks only for the words: the cards' lines that need rewriting, then the note.
var refreshSchema = js.Object(
	js.Prop("entries", js.Array(js.Object(
		js.Prop("task_id", js.Integer("The card's task number, from #<id>.")),
		js.Prop("rationale", js.String("The card's new line, saying why the task sits in its tier.")),
	))),
	js.Prop("opening", js.String("The plan's note, two or three sentences to the user, in your voice.")),
)

type refreshReply struct {
	Entries []struct {
		TaskID    int64  `json:"task_id"`
		Rationale string `json:"rationale"`
	} `json:"entries"`
	Opening string `json:"opening"`
}

// Refresh brings the words of today's plan up to date after planned tasks changed, such as one
// being finished: the note and any card line that still talks about it as ahead. Every task keeps
// its tier and place. It saves a new plan version and publishes it as plan.updated, and reports
// whether it did. A plan code made, or a day with no model to write with, is left as it is;
// so is the plan when the model's reply won't do, since a stale line is better than a wrong one.
func (p *Planner) Refresh(ctx context.Context, changes string) (domain.Plan, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, err := p.gather(ctx, true)
	if err != nil {
		return domain.Plan{}, false, err
	}
	if st.day == nil || st.plan == nil {
		return domain.Plan{}, false, ErrNoDay
	}
	if st.day.WrappedAt != nil {
		return domain.Plan{}, false, fmt.Errorf("%w: %s is already wrapped", service.ErrConflict, st.day.Date)
	}
	if st.plan.Fallback || len(st.plan.Entries) == 0 {
		return *st.plan, false, nil
	}
	if st.model == nil {
		return *st.plan, false, st.modelErr
	}

	user, showing := refreshPrompt(st, changes)
	req := st.request(refreshInstructions, user, refreshSchema, "plan_refresh")
	req.Model = st.cfg.LightModel
	mctx, cancel := context.WithTimeout(ctx, modelTimeout)
	defer cancel()
	reply, err := ask(mctx, st.model, req, func(r refreshReply) error { return checkRefresh(r, showing) })
	if err != nil {
		return *st.plan, false, err
	}

	lines := map[int64]string{}
	for _, e := range reply.Entries {
		lines[e.TaskID] = strings.TrimSpace(e.Rationale)
	}
	entries := make([]domain.PlanEntry, len(st.plan.Entries))
	for i, e := range st.plan.Entries {
		if line, ok := lines[e.TaskID]; ok {
			e.Rationale = line
		}
		entries[i] = e
	}
	saved, err := p.svc.SavePlan(ctx, domain.Plan{
		DayID:   st.day.ID,
		Trigger: domain.TriggerRefresh,
		Reason:  changes,
		Opening: strings.TrimSpace(reply.Opening),
		Entries: entries,
	})
	if err != nil {
		return domain.Plan{}, false, err
	}
	p.hub.Publish(stream.Message{Type: "plan.updated", Data: saved})
	return saved, true, nil
}

// refreshPrompt lays out what changed, the note, and the cards still showing, by tier. It returns
// the showing cards' task numbers, the only ones a reply may rewrite.
func refreshPrompt(st state, changes string) (string, map[int64]bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "What changed: %s.\n\nThe plan's note now: %q\n\nCards still showing, as #<id> title: line:\n", changes, st.plan.Opening)
	showing := map[int64]bool{}
	for _, tier := range []domain.Tier{domain.TierMust, domain.TierShould, domain.TierCould} {
		fmt.Fprintf(&b, "%s:\n", tier)
		n := 0
		for _, e := range st.plan.Entries {
			tf, ok := st.facts.Task(e.TaskID)
			if e.Tier != tier || !ok {
				continue
			}
			showing[e.TaskID] = true
			fmt.Fprintf(&b, "#%d %s: %q\n", e.TaskID, tf.Task.Title, e.Rationale)
			n++
		}
		if n == 0 {
			b.WriteString("(none)\n")
		}
	}
	return b.String(), showing
}

// checkRefresh rejects a reply that rewrites a card that isn't showing, rewrites one twice, or
// leaves a line or the note empty.
func checkRefresh(r refreshReply, showing map[int64]bool) error {
	var errs []error
	seen := map[int64]bool{}
	for _, e := range r.Entries {
		if !showing[e.TaskID] {
			errs = append(errs, fmt.Errorf("#%d is not a card still showing", e.TaskID))
		}
		if seen[e.TaskID] {
			errs = append(errs, fmt.Errorf("#%d appears more than once", e.TaskID))
		}
		seen[e.TaskID] = true
		if strings.TrimSpace(e.Rationale) == "" {
			errs = append(errs, fmt.Errorf("#%d has an empty line", e.TaskID))
		}
	}
	if strings.TrimSpace(r.Opening) == "" {
		errs = append(errs, errors.New("the note is empty"))
	}
	return errors.Join(errs...)
}

// quietRefresh is a refresh that didn't happen for an ordinary reason, not worth a log line.
func quietRefresh(err error) bool {
	return errors.Is(err, ErrNoDay) || errors.Is(err, settings.ErrNotConfigured) || errors.Is(err, service.ErrConflict)
}
