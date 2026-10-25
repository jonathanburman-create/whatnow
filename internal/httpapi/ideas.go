package httpapi

import (
	"net/http"

	"github.com/jameynakama/whatnow/internal/service"
)

func (s *Server) listIdeas(w http.ResponseWriter, r *http.Request) {
	ideas, err := s.Service.ListIdeas(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ideas)
}

func (s *Server) createIdea(w http.ResponseWriter, r *http.Request) {
	var f service.IdeaFields
	if err := decode(r, &f); err != nil {
		writeError(w, err)
		return
	}
	s.write(w, r, http.StatusCreated, func(batch int64) (any, error) {
		return s.Service.CreateIdea(r.Context(), batch, f)
	})
}

func (s *Server) updateIdea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var f service.IdeaFields
	if err := decode(r, &f); err != nil {
		writeError(w, err)
		return
	}
	s.write(w, r, http.StatusOK, func(batch int64) (any, error) {
		return s.Service.UpdateIdea(r.Context(), batch, id, f)
	})
}

func (s *Server) deleteIdea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	s.write(w, r, http.StatusOK, func(batch int64) (any, error) {
		return nil, s.Service.DeleteIdea(r.Context(), batch, id)
	})
}

// ideaToTask turns an idea into a task and takes it off the pile.
func (s *Server) ideaToTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	s.write(w, r, http.StatusOK, func(batch int64) (any, error) {
		return s.Service.IdeaToTask(r.Context(), batch, id)
	})
}
