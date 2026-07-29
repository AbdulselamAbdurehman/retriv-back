package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/channel"
)

// writeChannelError maps channel authorization errors to HTTP statuses.
func writeChannelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, channel.ErrNotFound):
		writeError(w, http.StatusNotFound, "match not found")
	case errors.Is(err, channel.ErrForbidden):
		writeError(w, http.StatusForbidden, "not your match")
	case errors.Is(err, channel.ErrNotConnected):
		writeError(w, http.StatusConflict, "match is not connected yet")
	default:
		writeError(w, http.StatusInternalServerError, "channel error")
	}
}

func (a *API) handleListMessages(w http.ResponseWriter, r *http.Request) {
	matchID, ok := parsePathID(w, r)
	if !ok {
		return
	}
	me := userID(r)
	msgs, err := a.channel.List(r.Context(), matchID, me)
	if err != nil {
		writeChannelError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{
			"id":         m.ID,
			"body":       m.Body,
			"mine":       m.SenderUserID == me,
			"created_at": m.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

type postMessageRequest struct {
	Body string `json:"body"`
}

func (a *API) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	matchID, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var req postMessageRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "message body is required")
		return
	}
	if err := a.channel.Post(r.Context(), matchID, userID(r), strings.TrimSpace(req.Body)); err != nil {
		writeChannelError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "sent"})
}
