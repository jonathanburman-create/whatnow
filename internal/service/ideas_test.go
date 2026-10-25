package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/jameynakama/whatnow/internal/domain"
	"github.com/jameynakama/whatnow/internal/service"
)

func TestCreateIdeaFromRawText(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	raw := "essay angle: rawls vs nozick on luck\nmaybe for the political theory seminar"

	got, err := svc.CreateIdea(ctx, newBatch(t, svc, domain.ActorAgent),
		service.IdeaFields{RawText: raw, Area: ptr(domain.AreaHUJI)})
	if err != nil {
		t.Fatal(err)
	}

	if got.Title != "essay angle: rawls vs nozick on luck" || got.RawText != raw || got.CreatedBy != domain.ActorAgent {
		t.Errorf("idea = %+v", got)
	}
	listed, err := svc.ListIdeas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]domain.Idea{got}, listed); diff != "" {
		t.Errorf("stored idea differs from returned idea (-returned +stored):\n%s", diff)
	}
}

func TestListIdeasNewestFirst(t *testing.T) {
	svc, clock := newService(t)
	ctx := context.Background()
	for _, title := range []string{"first", "second", "third"} {
		if _, err := svc.CreateIdea(ctx, newBatch(t, svc, domain.ActorUser), service.IdeaFields{Title: title}); err != nil {
			t.Fatal(err)
		}
		clock.advance(time.Minute)
	}

	ideas, err := svc.ListIdeas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, i := range ideas {
		titles = append(titles, i.Title)
	}
	if diff := cmp.Diff([]string{"third", "second", "first"}, titles); diff != "" {
		t.Errorf("order (-want +got):\n%s", diff)
	}
}

func TestCreateIdeaRejectsInvalidFields(t *testing.T) {
	cases := map[string]service.IdeaFields{
		"no title or raw text": {Title: "  "},
		"bad area":             {Title: "x", Area: ptr(domain.Area("work"))},
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _ := newService(t)
			_, err := svc.CreateIdea(context.Background(), newBatch(t, svc, domain.ActorUser), fields)
			if !errors.Is(err, service.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestDeleteIdeaHidesItAndUndoBringsItBack(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	idea, err := svc.CreateIdea(ctx, newBatch(t, svc, domain.ActorUser), service.IdeaFields{Title: "look up Matter bridges"})
	if err != nil {
		t.Fatal(err)
	}
	batch := newBatch(t, svc, domain.ActorUser)
	if err := svc.DeleteIdea(ctx, batch, idea.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetIdea(ctx, idea.ID); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("deleted idea: err = %v, want ErrNotFound", err)
	}

	if err := svc.Undo(ctx, batch); err != nil {
		t.Fatal(err)
	}
	back, err := svc.GetIdea(ctx, idea.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Title != idea.Title {
		t.Errorf("after undo: %+v", back)
	}
}

func TestUpdateIdeaKeepsRawText(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	idea, err := svc.CreateIdea(ctx, newBatch(t, svc, domain.ActorUser), service.IdeaFields{RawText: "podcast idea"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdateIdea(ctx, newBatch(t, svc, domain.ActorUser), idea.ID,
		service.IdeaFields{Title: "Podcast about exchange semesters", Notes: "interview friends", RawText: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Podcast about exchange semesters" || got.Notes != "interview friends" || got.RawText != "podcast idea" {
		t.Errorf("updated idea = %+v", got)
	}
}

func TestIdeaToTaskMovesItOffThePileInOneUndoableBatch(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	idea, err := svc.CreateIdea(ctx, newBatch(t, svc, domain.ActorUser),
		service.IdeaFields{Title: "Write up the Matter setup", Notes: "for the blog", Area: ptr(domain.AreaProjects), RawText: "raw"})
	if err != nil {
		t.Fatal(err)
	}
	batch := newBatch(t, svc, domain.ActorUser)

	task, err := svc.IdeaToTask(ctx, batch, idea.ID)
	if err != nil {
		t.Fatal(err)
	}

	if task.Title != idea.Title || task.Notes != idea.Notes || task.Area == nil || *task.Area != domain.AreaProjects ||
		task.RawText != "raw" || task.Status != domain.StatusOpen {
		t.Errorf("task = %+v", task)
	}
	if ideas, _ := svc.ListIdeas(ctx); len(ideas) != 0 {
		t.Errorf("idea still on the pile: %+v", ideas)
	}

	if err := svc.Undo(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if ideas, _ := svc.ListIdeas(ctx); len(ideas) != 1 {
		t.Errorf("after undo, ideas = %+v, want the idea back", ideas)
	}
	if tasks, _ := svc.ListTasks(ctx); len(tasks) != 0 {
		t.Errorf("after undo, tasks = %+v, want none", tasks)
	}
}
