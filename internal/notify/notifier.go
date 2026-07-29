// Package notify persists in-app notifications and sends transactional email.
// Email is fire-and-forget and never stored.
package notify

import (
	"context"
	"log"

	"github.com/google/uuid"
)

// EmailResolver maps a user id to their email address. Implemented by auth.Repo.
type EmailResolver interface {
	EmailByID(ctx context.Context, userID uuid.UUID) (string, error)
}

type Service struct {
	repo   *Repo
	mailer *SMTPMailer
	emails EmailResolver
}

func NewService(repo *Repo, mailer *SMTPMailer, emails EmailResolver) *Service {
	return &Service{repo: repo, mailer: mailer, emails: emails}
}

// Notify stores an in-app notification and best-effort emails the user (FR-14).
// Email failures are logged, not returned: a missed email must not fail the flow.
func (s *Service) Notify(ctx context.Context, userID uuid.UUID, entryType string, entryID uuid.UUID, title, message string) error {
	if err := s.repo.Insert(ctx, userID, entryType, entryID, title, message); err != nil {
		return err
	}
	email, err := s.emails.EmailByID(ctx, userID)
	if err != nil || email == "" {
		return nil
	}
	if err := s.mailer.Send(ctx, email, title, message); err != nil {
		log.Printf("notify: email to %s failed: %v", email, err)
	}
	return nil
}

// Send emails a raw message (used for auth magic links). Implements auth.Mailer.
func (s *Service) Send(ctx context.Context, to, subject, body string) error {
	return s.mailer.Send(ctx, to, subject, body)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Notification, error) {
	return s.repo.ListByUser(ctx, userID)
}
