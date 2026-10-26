package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/planner"
)

// glance is today at a glance in plain text, for a Shortcut to show as a notification: what's done,
// what's left in each tier, and the next event. It reads the plan and the calendar and calls no
// model, so it answers at once and costs nothing.
func (s *Server) glance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today, err := s.Planner.Today(ctx)
	if err != nil {
		log.Printf("glance: %v", err)
		writeText(w, http.StatusInternalServerError, "Couldn't read your planner just now.")
		return
	}
	tasks, err := s.Service.ListTasks(ctx)
	if err != nil {
		log.Printf("glance: %v", err)
		writeText(w, http.StatusInternalServerError, "Couldn't read your planner just now.")
		return
	}
	now := s.Now()
	events, err := s.Service.ListEvents(ctx, now, now.AddDate(0, 0, 7))
	if err != nil {
		log.Printf("glance: %v", err)
		writeText(w, http.StatusInternalServerError, "Couldn't read your planner just now.")
		return
	}
	writeText(w, http.StatusOK, glanceText(today, tasks, events, now, s.Loc))
}

// glanceText lays the day out in a few short lines. A tier shows the titles still open in it, the
// same cards Today shows; could, often long, shows its first task and how many more.
func glanceText(t planner.Today, tasks []domain.Task, events []domain.Event, now time.Time, loc *time.Location) string {
	byID := map[int64]domain.Task{}
	done := 0
	for _, task := range tasks {
		byID[task.ID] = task
		if task.Status == domain.StatusDone && domain.Date(task.UpdatedAt.In(loc).Format(time.DateOnly)) == t.Date {
			done++
		}
	}

	var lines []string
	switch {
	case t.Day == nil || t.Plan == nil:
		lines = append(lines, "Today isn't planned yet. Open the planner to start your day.")
	case t.Day.WrappedAt != nil:
		lines = append(lines, fmt.Sprintf("Day wrapped up with %s. Rest well.", count(done, "thing done", "things done")))
	default:
		tiers := map[domain.Tier][]string{}
		left := 0
		for _, e := range t.Plan.Entries {
			task, ok := byID[e.TaskID]
			if !ok || task.Status != domain.StatusOpen || (task.NotBefore != nil && *task.NotBefore > t.Date) {
				continue
			}
			tiers[e.Tier] = append(tiers[e.Tier], task.Title)
			left++
		}
		if left == 0 {
			lines = append(lines, fmt.Sprintf("%s today, and the plan is clear.", capitalize(count(done, "thing done", "things done"))))
		} else if done == 0 {
			lines = append(lines, fmt.Sprintf("Nothing done yet, %d to go.", left))
		} else {
			lines = append(lines, fmt.Sprintf("%d done, %d to go.", done, left))
		}
		for _, tier := range []domain.Tier{domain.TierMust, domain.TierShould, domain.TierCould} {
			titles := tiers[tier]
			if len(titles) == 0 {
				continue
			}
			if tier == domain.TierCould && len(titles) > 1 {
				titles = []string{fmt.Sprintf("%s, +%d more", titles[0], len(titles)-1)}
			}
			lines = append(lines, fmt.Sprintf("%s: %s", capitalize(string(tier)), strings.Join(titles, "; ")))
		}
	}
	if len(events) > 0 {
		lines = append(lines, "Next: "+events[0].Title+", "+when(events[0].StartsAt, now, loc))
	}
	return strings.Join(lines, "\n")
}

// when says when an event starts the way you'd say it: "today 3:00 PM", "tomorrow 9:30 AM", or
// "Thu 3:00 PM" later in the week.
func when(at, now time.Time, loc *time.Location) string {
	at, now = at.In(loc), now.In(loc)
	clock := at.Format("3:04 PM")
	y1, m1, d1 := now.Date()
	y2, m2, d2 := at.Date()
	days := int(time.Date(y2, m2, d2, 0, 0, 0, 0, loc).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, loc)).Hours()/24 + 0.5)
	switch days {
	case 0:
		return "today " + clock
	case 1:
		return "tomorrow " + clock
	}
	return at.Format("Mon") + " " + clock
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
