package planner

import (
	"context"
	"fmt"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/service"
	"github.com/jameynakama/whatnow/internal/stream"
)

// defaultPace is the pace a day started from outside the morning ritual gets.
const defaultPace = domain.Pace(3)

// SetPlan saves a whole plan for today that someone else worked out, such as Claude through the
// connector, with no model call here. If today hasn't started, it starts at a steady pace first;
// an earlier day left unwrapped stays as it is, for the morning flow to reconcile. Every entry
// must be an eligible task, at most once.
func (p *Planner) SetPlan(ctx context.Context, entries []domain.PlanEntry, opening, reason string) (domain.Plan, error) {
	seen := map[int64]bool{}
	for _, e := range entries {
		if !e.Tier.Valid() {
			return domain.Plan{}, fmt.Errorf("%w: tier %q", service.ErrInvalid, e.Tier)
		}
		if seen[e.TaskID] {
			return domain.Plan{}, fmt.Errorf("%w: #%d is in the plan twice", service.ErrInvalid, e.TaskID)
		}
		seen[e.TaskID] = true
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	st, err := p.gather(ctx, false)
	if err != nil {
		return domain.Plan{}, err
	}
	if st.day == nil {
		day, err := p.svc.StartDay(ctx, st.facts.Today, defaultPace, "")
		if err != nil {
			return domain.Plan{}, err
		}
		st.day = &day
	}
	if st.day.WrappedAt != nil {
		return domain.Plan{}, fmt.Errorf("%w: %s is already wrapped", service.ErrConflict, st.day.Date)
	}
	for _, e := range entries {
		if _, ok := st.facts.Task(e.TaskID); !ok {
			return domain.Plan{}, fmt.Errorf("%w: #%d is not an open task that can be done today", service.ErrInvalid, e.TaskID)
		}
	}

	saved, err := p.svc.SavePlan(ctx, domain.Plan{
		DayID:   st.day.ID,
		Trigger: domain.TriggerAgent,
		Reason:  reason,
		Opening: opening,
		Entries: entries,
	})
	if err != nil {
		return domain.Plan{}, err
	}
	p.hub.Publish(stream.Message{Type: "plan.updated", Data: saved})
	return saved, nil
}
