package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/facts"
	"github.com/jameynakama/whatnow/internal/stream"
)

// Run replans today whenever a change calls for it, until ctx is done or msgs closes. msgs is a
// hub subscription, so changes from the API, cleanup, and the agent are all seen, and so are
// undos. Changes that pile up while a replan runs are folded into one more replan. A change that
// doesn't call for a replan but leaves the plan's words stale, such as a planned task finished,
// refreshes the note and the cards' lines instead, keeping every task in its tier.
func (p *Planner) Run(ctx context.Context, msgs <-chan stream.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-msgs:
			if !ok {
				return
			}
			trigger, reasons, changes := p.reasons(ctx, m)
			for more := true; more; {
				select {
				case m, ok := <-msgs:
					if !ok {
						return
					}
					t, r, c := p.reasons(ctx, m)
					if t == domain.TriggerPace {
						trigger = t
					}
					reasons = append(reasons, r...)
					changes = append(changes, c...)
				default:
					more = false
				}
			}
			switch {
			case len(reasons) > 0:
				// A replan writes a new note and new lines anyway.
				if _, err := p.Replan(ctx, trigger, strings.Join(reasons, "; ")); err != nil && !errors.Is(err, ErrNoDay) {
					log.Printf("replan: %v", err)
				}
			case len(changes) > 0:
				if _, _, err := p.Refresh(ctx, strings.Join(changes, "; ")); err != nil && !quietRefresh(err) {
					log.Printf("refresh the plan's words: %v", err)
				}
			}
		}
	}
}

// reasons says why one hub message calls for a replan, and which planned tasks it took off or put
// back on today's list. Only a day that has a plan and isn't wrapped up is replanned or refreshed;
// before that, the morning makes the first plan.
func (p *Planner) reasons(ctx context.Context, m stream.Message) (domain.Trigger, []string, []string) {
	var acts []domain.Activity
	undone := false
	switch d := m.Data.(type) {
	case stream.BatchApplied:
		acts = d.Activity
	case stream.BatchUndone:
		var err error
		if acts, err = p.svc.BatchActivity(ctx, d.BatchID); err != nil {
			log.Printf("replan: read undone batch %d: %v", d.BatchID, err)
			return "", nil, nil
		}
		undone = true
	default:
		return "", nil, nil
	}
	today := facts.Today(p.now(), p.loc)
	day, err := p.svc.GetDay(ctx, today)
	if err != nil || day.WrappedAt != nil {
		return "", nil, nil
	}
	plan, err := p.svc.LatestPlan(ctx, day.ID)
	if err != nil {
		return "", nil, nil
	}
	musts, planned := map[int64]bool{}, map[int64]bool{}
	for _, e := range plan.Entries {
		planned[e.TaskID] = true
		if e.Tier == domain.TierMust {
			musts[e.TaskID] = true
		}
	}
	trigger, reasons := replanReasons(acts, musts, today, undone)
	return trigger, reasons, planChanges(acts, planned, today, undone)
}

// planChanges lists the planned tasks a batch took off today's list or put back on it: finished,
// dropped, deleted, pushed past today, or open again. Each leaves the plan's words out of date,
// such as a note that says "after the quiz" once the quiz is done. undone runs the batch backward.
func planChanges(acts []domain.Activity, planned map[int64]bool, today domain.Date, undone bool) []string {
	var changes []string
	for _, a := range acts {
		if a.Entity != "task" || !planned[a.EntityID] {
			continue
		}
		before, after := a.Before, a.After
		if undone {
			before, after = after, before
		}
		var b, af domain.Task
		// A task made in this batch isn't a change to the plan's words: the plan was made with it
		// in mind, such as the morning's dump being sorted after its plan was saved. Deleting a
		// task keeps it, marked deleted, so both sides are there for every change that matters.
		if !decode(before, &b) || !decode(after, &af) {
			continue
		}
		wasOn, isOn := onToday(b, today), onToday(af, today)
		if wasOn == isOn {
			continue
		}
		t := af
		var what string
		switch {
		case isOn:
			what = "is back on today's list"
		case af.DeletedAt != nil:
			what = "was deleted"
		case af.Status == domain.StatusDone:
			what = "was finished"
		case af.Status == domain.StatusDropped:
			what = "was dropped"
		default:
			what = "was pushed past today"
		}
		changes = append(changes, fmt.Sprintf("#%d %q %s", t.ID, t.Title, what))
	}
	return changes
}

// onToday is whether a task still shows on today's plan: open, and not put off past today.
func onToday(t domain.Task, today domain.Date) bool {
	return live(t) && (t.NotBefore == nil || *t.NotBefore <= today)
}

// replanReasons lists what in a batch calls for a replan: a pace change, a task becoming due
// today, a must being dropped or deleted, or a blocker added or removed on a must. undone runs
// the batch's changes backward, since an undo reverses each of them.
func replanReasons(acts []domain.Activity, musts map[int64]bool, today domain.Date, undone bool) (domain.Trigger, []string) {
	trigger := domain.TriggerEdit
	var reasons []string
	for _, a := range acts {
		before, after := a.Before, a.After
		if undone {
			before, after = after, before
		}
		switch a.Entity {
		case "day":
			var b, af domain.Day
			if decode(before, &b) && decode(after, &af) && b.Pace != af.Pace && af.Date == today {
				trigger = domain.TriggerPace
				reasons = append(reasons, fmt.Sprintf("pace changed to %q", af.Pace.Label()))
			}
		case "task":
			var b, af domain.Task
			hadB, hadA := decode(before, &b), decode(after, &af)
			switch {
			case hadA && dueNow(af, today) && !(hadB && dueNow(b, today)):
				reasons = append(reasons, fmt.Sprintf("#%d %q is due now", af.ID, af.Title))
			case musts[a.EntityID] && hadB && live(b) && (!hadA || af.Status == domain.StatusDropped || af.DeletedAt != nil):
				reasons = append(reasons, fmt.Sprintf("the must #%d %q was dropped", b.ID, b.Title))
			}
		case "blocker":
			var b domain.Blocker
			if !decode(before, &b) {
				decode(after, &b)
			}
			for _, id := range []int64{b.TaskID, b.BlockedByID} {
				if musts[id] {
					reasons = append(reasons, fmt.Sprintf("a blocker on the must #%d changed", id))
					break
				}
			}
		}
	}
	return trigger, reasons
}

func decode(raw json.RawMessage, v any) bool {
	return raw != nil && json.Unmarshal(raw, v) == nil
}

func live(t domain.Task) bool { return t.Status == domain.StatusOpen && t.DeletedAt == nil }

func dueNow(t domain.Task, today domain.Date) bool {
	return live(t) && t.DueOn != nil && *t.DueOn <= today
}
