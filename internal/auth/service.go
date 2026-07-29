package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Mailer sends transactional email. Implemented by the notify package.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Service struct {
	repo       *Repo
	sessions   *SessionManager
	mailer     Mailer
	appBaseURL string
	linkTTL    time.Duration
}

func NewService(repo *Repo, sessions *SessionManager, mailer Mailer, appBaseURL string, linkTTL time.Duration) *Service {
	return &Service{repo: repo, sessions: sessions, mailer: mailer, appBaseURL: appBaseURL, linkTTL: linkTTL}
}

var ErrEmailTaken = errors.New("auth: email already registered")

type RegisterInput struct {
	Email      string
	FirstName  string
	MiddleName string
	LastName   string
	Phone      string
}

// Register creates an account and emails a sign-in link.
func (s *Service) Register(ctx context.Context, in RegisterInput) error {
	_, err := s.repo.CreateUser(ctx, User{
		Email:      strings.ToLower(strings.TrimSpace(in.Email)),
		FirstName:  in.FirstName,
		MiddleName: in.MiddleName,
		LastName:   in.LastName,
		Phone:      in.Phone,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrEmailTaken
		}
		return err
	}
	return s.sendLink(ctx, strings.ToLower(strings.TrimSpace(in.Email)))
}

// RequestLogin emails a sign-in link if the account exists. It never reveals
// whether the email is registered (prevents account enumeration).
func (s *Service) RequestLogin(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := s.repo.UserByEmail(ctx, email); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return s.sendLink(ctx, email)
}

// Verify consumes a raw magic-link token and returns a freshly issued session.
func (s *Service) Verify(ctx context.Context, rawToken string) (string, error) {
	hash := sha256.Sum256([]byte(rawToken))
	userID, err := s.repo.ConsumeMagicLink(ctx, hash[:])
	if err != nil {
		return "", err
	}
	return s.sessions.Issue(userID), nil
}

func (s *Service) sendLink(ctx context.Context, email string) error {
	user, err := s.repo.UserByEmail(ctx, email)
	if err != nil {
		return err
	}
	raw, err := randomToken()
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(raw))
	if err := s.repo.CreateMagicLink(ctx, user.ID, hash[:], time.Now().Add(s.linkTTL)); err != nil {
		return err
	}

	// Routed through the frontend origin, which proxies /api to the backend so
	// the session cookie is set on the origin the SPA runs on.
	link := fmt.Sprintf("%s/api/auth/verify?token=%s", s.appBaseURL, raw)
	body := fmt.Sprintf("Hi %s,\n\nSign in to Retriv:\n%s\n\nThis link expires in %s.",
		user.FirstName, link, s.linkTTL)
	return s.mailer.Send(ctx, email, "Your Retriv sign-in link", body)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// User returns the account for an authenticated request.
func (s *Service) User(ctx context.Context, id uuid.UUID) (User, error) {
	return s.repo.UserByID(ctx, id)
}

// Sessions exposes the session verifier to middleware.
func (s *Service) Sessions() *SessionManager { return s.sessions }
