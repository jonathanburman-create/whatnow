package httpapi

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jameynakama/whatnow/internal/agent"
	"github.com/jameynakama/whatnow/internal/service"
)

func (s *Server) chatHistory(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.Agent.History(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// chat sends a message and waits for the reply, which also streams over SSE as it's written.
// The turn finishes even if the client leaves, so its changes and its reply are never cut short.
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(r, &body); err != nil {
		writeError(w, err)
		return
	}
	reply, err := s.Agent.Send(context.WithoutCancel(r.Context()), body.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

// ask is chat for somewhere that can only show plain text, such as an iPhone Shortcut showing the
// reply as a notification. It takes {"text": "..."} like chat, and the message and its reply land
// in the app's chat as if typed there. The answer is the reply's text alone, or, when there is no
// reply, a plain line saying why. Like chat, the turn finishes even if the phone gives up waiting.
func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(r, &body); err != nil {
		writeText(w, http.StatusBadRequest, `Send {"text": "your message"} as JSON.`)
		return
	}
	reply, err := s.Agent.Send(context.WithoutCancel(r.Context()), body.Text)
	switch {
	case errors.Is(err, agent.ErrBusy):
		writeText(w, http.StatusConflict, "I'm still answering your last message. Try again in a moment.")
	case errors.Is(err, service.ErrInvalid):
		writeText(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), service.ErrInvalid.Error()+": "))
	case err != nil:
		log.Printf("ask: %v", err)
		writeText(w, http.StatusInternalServerError, "Something went wrong on the planner's side. Try again in a bit.")
	default:
		writeText(w, http.StatusOK, reply.Text)
	}
}

func writeText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	io.WriteString(w, text)
}
