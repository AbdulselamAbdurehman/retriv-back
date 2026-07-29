package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/reports"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/verification"
)

func parsePathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func (a *API) handleQuestions(w http.ResponseWriter, r *http.Request) {
	matchID, ok := parsePathID(w, r)
	if !ok {
		return
	}
	m, err := a.verification.Match(r.Context(), matchID)
	if errors.Is(err, verification.ErrNotFound) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load match")
		return
	}
	if userID(r) != m.LoserUserID { // only the loser answers
		writeError(w, http.StatusForbidden, "not your match")
		return
	}
	qs, err := a.verification.Questions(r.Context(), matchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load questions")
		return
	}
	out := make([]map[string]any, 0, len(qs))
	for _, q := range qs {
		out = append(out, map[string]any{"id": q.ID, "prompt": q.Prompt}) // never expose detail_id
	}
	writeJSON(w, http.StatusOK, map[string]any{"questions": out})
}

type answersRequest struct {
	Answers []struct {
		QuestionID uuid.UUID `json:"question_id"`
		Answer     string    `json:"answer"`
	} `json:"answers"`
}

func (a *API) handleAnswers(w http.ResponseWriter, r *http.Request) {
	matchID, ok := parsePathID(w, r)
	if !ok {
		return
	}
	var req answersRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	m, err := a.verification.Match(r.Context(), matchID)
	if errors.Is(err, verification.ErrNotFound) {
		writeError(w, http.StatusNotFound, "match not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load match")
		return
	}
	if userID(r) != m.LoserUserID {
		writeError(w, http.StatusForbidden, "not your match")
		return
	}

	answers := make(map[uuid.UUID]string, len(req.Answers))
	for _, ans := range req.Answers {
		answers[ans.QuestionID] = ans.Answer
	}

	out, err := a.verification.SubmitAnswers(r.Context(), matchID, answers)
	if errors.Is(err, verification.ErrNotPending) {
		writeError(w, http.StatusConflict, "this match is no longer awaiting verification")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not grade answers")
		return
	}

	status := "failed"
	switch {
	case out.Passed:
		status = "verified"
		a.connect(r, out.Match)
	case out.Discarded:
		status = "discarded"
	}
	// No confidence or per-question feedback is returned (FR-9).
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        status,
		"attempts_used": out.AttemptsUsed,
		"max_attempts":  out.MaxAttempts,
	})
}

// connect marks both reports matched and notifies both parties (FR-16). Best
// effort: notification/status errors are logged inside the services, not fatal.
func (a *API) connect(r *http.Request, m verification.MatchInfo) {
	ctx := r.Context()
	_ = a.reports.SetStatus(ctx, m.LostReportID, reports.StatusMatched)
	_ = a.reports.SetStatus(ctx, m.FoundReportID, reports.StatusMatched)
	_ = a.notify.Notify(ctx, m.LoserUserID, "match", m.ID,
		"Match confirmed", "Verified! You can now message the finder in-app.")
	_ = a.notify.Notify(ctx, m.FinderUserID, "match", m.ID,
		"Match confirmed", "The owner verified your found item. You can now message them in-app.")
}

func (a *API) handleResolve(w http.ResponseWriter, r *http.Request) {
	matchID, ok := parsePathID(w, r)
	if !ok {
		return
	}
	if err := a.channel.Resolve(r.Context(), matchID, userID(r)); err != nil {
		writeChannelError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "resolved",
		"donation_url": a.appBaseURL + "/donate", // optional donation prompt (FP-2)
	})
}
