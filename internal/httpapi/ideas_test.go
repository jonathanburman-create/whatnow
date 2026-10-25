package httpapi_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jameynakama/whatnow/internal/domain"
)

type ideaMutation struct {
	BatchID int64       `json:"batch_id"`
	Data    domain.Idea `json:"data"`
}

func TestIdeaRoutes(t *testing.T) {
	api := newAPI(t)

	rec := api.do(http.MethodPost, "/api/v1/ideas", map[string]any{"title": "Essay: Rawls vs Nozick on luck", "area": "huji"})
	wantStatus(t, rec, http.StatusCreated)
	idea := decodeAs[ideaMutation](t, rec).Data
	if idea.ID == 0 || idea.Area == nil || *idea.Area != domain.AreaHUJI {
		t.Fatalf("created %+v", idea)
	}

	rec = api.do(http.MethodPut, fmt.Sprintf("/api/v1/ideas/%d", idea.ID), map[string]any{"title": "Essay on luck", "notes": "seminar", "area": nil})
	wantStatus(t, rec, http.StatusOK)
	if got := decodeAs[ideaMutation](t, rec).Data; got.Title != "Essay on luck" || got.Area != nil {
		t.Errorf("updated %+v", got)
	}

	rec = api.do(http.MethodGet, "/api/v1/ideas", nil)
	wantStatus(t, rec, http.StatusOK)
	if got := decodeAs[[]domain.Idea](t, rec); len(got) != 1 {
		t.Errorf("listed %+v", got)
	}

	rec = api.do(http.MethodPost, fmt.Sprintf("/api/v1/ideas/%d/task", idea.ID), nil)
	wantStatus(t, rec, http.StatusOK)
	if task := decodeAs[taskMutation](t, rec).Data; task.Title != "Essay on luck" || task.Notes != "seminar" {
		t.Errorf("task %+v", task)
	}
	rec = api.do(http.MethodGet, "/api/v1/ideas", nil)
	if got := decodeAs[[]domain.Idea](t, rec); len(got) != 0 {
		t.Errorf("idea still listed: %+v", got)
	}

	wantStatus(t, api.do(http.MethodDelete, fmt.Sprintf("/api/v1/ideas/%d", idea.ID), nil), http.StatusNotFound)
	wantStatus(t, api.do(http.MethodPost, "/api/v1/ideas", map[string]any{"title": "x", "area": "work"}), http.StatusBadRequest)
}

func TestAreasRoute(t *testing.T) {
	api := newAPI(t)
	rec := api.do(http.MethodGet, "/api/v1/areas", nil)
	wantStatus(t, rec, http.StatusOK)
	got := decodeAs[[]domain.AreaInfo](t, rec)
	if len(got) != len(domain.Areas) || got[0].ID != domain.AreaHUJI || got[0].Label != "HUJI" {
		t.Errorf("areas = %+v", got)
	}
}
