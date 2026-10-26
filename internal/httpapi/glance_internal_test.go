package httpapi

import (
	"testing"
	"time"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/planner"
)

func TestGlanceText(t *testing.T) {
	loc := time.FixedZone("UTC-4", -4*3600)
	now := time.Date(2026, 10, 25, 21, 0, 0, 0, loc) // Sunday evening
	const today = domain.Date("2026-10-25")
	d := func(s string) *domain.Date { v := domain.Date(s); return &v }
	task := func(id int64, title string, status domain.Status, updated time.Time) domain.Task {
		return domain.Task{ID: id, Title: title, Status: status, UpdatedAt: updated}
	}
	earlier := now.Add(-2 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	tasks := []domain.Task{
		task(1, "Take the quiz", domain.StatusDone, earlier),
		task(2, "Pack for tomorrow", domain.StatusOpen, earlier),
		task(3, "Read Rawls ch. 3", domain.StatusOpen, earlier),
		task(4, "Email HUJI", domain.StatusOpen, earlier),
		task(5, "Buy a jacket", domain.StatusOpen, earlier),
		task(6, "Pushed off", domain.StatusOpen, earlier),
		task(7, "Done yesterday", domain.StatusDone, yesterday),
	}
	tasks[5].NotBefore = d("2026-10-27")
	plan := &domain.Plan{Entries: []domain.PlanEntry{
		{TaskID: 1, Tier: domain.TierMust}, {TaskID: 2, Tier: domain.TierMust}, {TaskID: 3, Tier: domain.TierShould},
		{TaskID: 4, Tier: domain.TierCould}, {TaskID: 5, Tier: domain.TierCould}, {TaskID: 6, Tier: domain.TierCould},
	}}
	day := &domain.Day{Date: today}
	dentist := []domain.Event{{Title: "Dentist", StartsAt: time.Date(2026, 10, 29, 15, 0, 0, 0, loc)}}

	cases := []struct {
		name   string
		today  planner.Today
		tasks  []domain.Task
		events []domain.Event
		want   string
	}{
		{"planned", planner.Today{Date: today, Day: day, Plan: plan}, tasks, dentist,
			"1 done, 4 to go.\nMust: Pack for tomorrow\nShould: Read Rawls ch. 3\nCould: Email HUJI, +1 more\nNext: Dentist, Thu 3:00 PM"},
		{"nothing done, event tomorrow", planner.Today{Date: today, Day: day, Plan: plan}, tasks[1:3],
			[]domain.Event{{Title: "Seminar", StartsAt: time.Date(2026, 10, 26, 9, 30, 0, 0, loc)}},
			"Nothing done yet, 2 to go.\nMust: Pack for tomorrow\nShould: Read Rawls ch. 3\nNext: Seminar, tomorrow 9:30 AM"},
		{"all clear", planner.Today{Date: today, Day: day, Plan: plan}, tasks[:1], nil,
			"1 thing done today, and the plan is clear."},
		{"not started", planner.Today{Date: today}, tasks, nil,
			"Today isn't planned yet. Open the planner to start your day."},
		{"wrapped", planner.Today{Date: today, Day: &domain.Day{Date: today, WrappedAt: &now}, Plan: plan}, tasks, nil,
			"Day wrapped up with 1 thing done. Rest well."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := glanceText(c.today, c.tasks, c.events, now, loc); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
	if got := when(time.Date(2026, 10, 25, 22, 15, 0, 0, loc), now, loc); got != "today 10:15 PM" {
		t.Errorf("when today = %q", got)
	}
}
