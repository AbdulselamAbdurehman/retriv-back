package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/reports"
)

type detailReq struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type createReportRequest struct {
	Kind        string      `json:"kind"`
	Category    string      `json:"category"`
	Description string      `json:"description"`
	Lat         *float64    `json:"lat"`
	Lng         *float64    `json:"lng"`
	EventAt     *time.Time  `json:"event_at"`
	Details     []detailReq `json:"details"`
}

func (a *API) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	var req createReportRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Kind != reports.KindLost && req.Kind != reports.KindFound {
		writeError(w, http.StatusBadRequest, "kind must be 'lost' or 'found'")
		return
	}
	if req.Category == "" || req.Description == "" {
		writeError(w, http.StatusBadRequest, "category and description are required")
		return
	}
	if req.Kind == reports.KindFound && len(req.Details) == 0 {
		writeError(w, http.StatusBadRequest, "found reports need at least one verification detail")
		return
	}

	in := reports.SubmitInput{
		UserID:      userID(r),
		Kind:        req.Kind,
		Category:    req.Category,
		Description: req.Description,
		EventAt:     req.EventAt,
	}
	if req.Lat != nil && req.Lng != nil {
		in.Lat, in.Lng, in.HasLocation = *req.Lat, *req.Lng, true
	}
	for _, d := range req.Details {
		in.Details = append(in.Details, reports.Detail{Kind: d.Kind, Value: d.Value})
	}

	rep, err := a.reports.Submit(r.Context(), in)
	if errors.Is(err, reports.ErrQuotaExceeded) {
		writeError(w, http.StatusTooManyRequests, "daily lost-report limit reached")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not submit report")
		return
	}

	// Immediately check the opposite pool. Only a binary status is returned so no
	// hint about any candidate can leak (FR-9).
	result, err := a.matching.FindAndOpen(r.Context(), rep.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "report saved, but matching failed")
		return
	}
	status := "no_match_yet"
	if result.Matched {
		status = "match_found"
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"report_id": rep.ID,
		"status":    status,
	})
}

func (a *API) handleListReports(w http.ResponseWriter, r *http.Request) {
	list, err := a.reports.ListByUser(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list reports")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, rep := range list {
		out = append(out, map[string]any{
			"id":          rep.ID,
			"kind":        rep.Kind,
			"category":    rep.Category,
			"description": rep.Description,
			"status":      rep.Status,
			"event_at":    rep.EventAt,
			"expires_at":  rep.ExpiresAt,
			"created_at":  rep.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": out})
}
