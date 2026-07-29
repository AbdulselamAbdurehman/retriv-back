package api

import "net/http"

func (a *API) handleNotifications(w http.ResponseWriter, r *http.Request) {
	list, err := a.notify.List(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notifications")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, n := range list {
		out = append(out, map[string]any{
			"id":         n.ID,
			"entry_type": n.EntryType,
			"entry_id":   n.EntryID,
			"title":      n.Title,
			"message":    n.Message,
			"read_at":    n.ReadAt,
			"created_at": n.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}
