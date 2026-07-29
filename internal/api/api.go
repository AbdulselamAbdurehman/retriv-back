// Package api is the HTTP transport layer: it decodes requests, calls the
// domain services, and encodes responses. Business logic lives in the services.
package api

import (
	"net/http"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/auth"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/channel"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/matching"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/notify"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/reports"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/verification"
)

type API struct {
	auth         *auth.Service
	reports      *reports.Service
	matching     *matching.Service
	verification *verification.Service
	channel      *channel.Service
	notify       *notify.Service
	sessions     *auth.SessionManager
	appBaseURL   string
	cookieSecure bool
}

type Deps struct {
	Auth         *auth.Service
	Reports      *reports.Service
	Matching     *matching.Service
	Verification *verification.Service
	Channel      *channel.Service
	Notify       *notify.Service
	AppBaseURL   string
	CookieSecure bool
}

func New(d Deps) *API {
	return &API{
		auth:         d.Auth,
		reports:      d.Reports,
		matching:     d.Matching,
		verification: d.Verification,
		channel:      d.Channel,
		notify:       d.Notify,
		sessions:     d.Auth.Sessions(),
		appBaseURL:   d.AppBaseURL,
		cookieSecure: d.CookieSecure,
	}
}

// Handler builds the route table wrapped in the global middleware chain.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Auth
	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("GET /api/auth/verify", a.handleVerify)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))

	// Reports
	mux.HandleFunc("POST /api/reports", a.requireAuth(a.handleCreateReport))
	mux.HandleFunc("GET /api/reports", a.requireAuth(a.handleListReports))

	// Matches / verification
	mux.HandleFunc("GET /api/matches/{id}/questions", a.requireAuth(a.handleQuestions))
	mux.HandleFunc("POST /api/matches/{id}/answers", a.requireAuth(a.handleAnswers))
	mux.HandleFunc("POST /api/matches/{id}/resolve", a.requireAuth(a.handleResolve))

	// Masked channel
	mux.HandleFunc("GET /api/matches/{id}/messages", a.requireAuth(a.handleListMessages))
	mux.HandleFunc("POST /api/matches/{id}/messages", a.requireAuth(a.handlePostMessage))

	// Notifications
	mux.HandleFunc("GET /api/notifications", a.requireAuth(a.handleNotifications))

	return recoverPanic(logging(mux))
}
