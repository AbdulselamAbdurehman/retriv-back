package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/api"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/auth"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/channel"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/config"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/crypto"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/db"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/matching"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/notify"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/ollama"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/reports"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/verification"
)

const (
	sessionTTL = 30 * 24 * time.Hour
	linkTTL    = 15 * time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Migrations run before the pool opens so the schema is ready on first query.
	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	cipher, err := crypto.New(cfg.EncKeyB64)
	if err != nil {
		return err
	}
	oll := ollama.New(cfg.OllamaBaseURL, cfg.OllamaEmbedModel, cfg.OllamaChatModel)

	// Repositories.
	authRepo := auth.NewRepo(pool)
	reportsRepo := reports.NewRepo(pool)
	matchingRepo := matching.NewRepo(pool)
	verifRepo := verification.NewRepo(pool)
	notifyRepo := notify.NewRepo(pool)
	channelRepo := channel.NewRepo(pool)

	// Services. Dependencies flow inward; cross-service wiring uses interfaces.
	mailer := notify.NewSMTPMailer(cfg.SMTPAddr, cfg.SMTPFrom)
	notifySvc := notify.NewService(notifyRepo, mailer, authRepo)

	sessions := auth.NewSessionManager(cfg.SessionSecret, sessionTTL)
	authSvc := auth.NewService(authRepo, sessions, notifySvc, cfg.AppBaseURL, linkTTL)

	reportsSvc := reports.NewService(reportsRepo, oll, cipher, cfg.LostReportsPerDay, cfg.ReportTTL)
	verifSvc := verification.NewService(verifRepo, oll, reportsSvc, verification.Config{
		MinConfidence: cfg.MinConfidence,
		MaxAttempts:   cfg.MaxVerifyAttempts,
	})
	matchingSvc := matching.NewService(matchingRepo, reportsSvc, verifSvc, notifySvc, matching.Config{
		RadiusMeters:   cfg.MatchRadiusMeters,
		TimeWindow:     cfg.MatchTimeWindow,
		MaxCosineDist:  cfg.MatchMaxCosineDistance,
		CandidateLimit: cfg.MatchCandidateLimit,
	})
	channelSvc := channel.NewService(channelRepo)

	api := api.New(api.Deps{
		Auth:         authSvc,
		Reports:      reportsSvc,
		Matching:     matchingSvc,
		Verification: verifSvc,
		Channel:      channelSvc,
		Notify:       notifySvc,
		AppBaseURL:   cfg.AppBaseURL,
		CookieSecure: strings.HasPrefix(cfg.AppBaseURL, "https"),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("retriv listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-shutdown
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
