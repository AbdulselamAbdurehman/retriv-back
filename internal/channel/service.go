// Package channel is the masked in-app message channel between two matched
// parties (FR-16). Real contact details are never exchanged here.
package channel

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// match statuses that permit channel access
const (
	statusVerified = "verified"
	statusResolved = "resolved"
)

var (
	ErrForbidden    = errors.New("channel: not a participant")
	ErrNotConnected = errors.New("channel: match is not connected yet")
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// authorize checks the user is one of the two parties and the match is connected.
func (s *Service) authorize(ctx context.Context, matchID, userID uuid.UUID) error {
	loser, finder, status, err := s.repo.Participants(ctx, matchID)
	if err != nil {
		return err
	}
	if userID != loser && userID != finder {
		return ErrForbidden
	}
	if status != statusVerified && status != statusResolved {
		return ErrNotConnected
	}
	return nil
}

func (s *Service) Post(ctx context.Context, matchID, userID uuid.UUID, body string) error {
	if err := s.authorize(ctx, matchID, userID); err != nil {
		return err
	}
	return s.repo.Insert(ctx, matchID, userID, body)
}

func (s *Service) List(ctx context.Context, matchID, userID uuid.UUID) ([]Message, error) {
	if err := s.authorize(ctx, matchID, userID); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, matchID)
}

func (s *Service) Resolve(ctx context.Context, matchID, userID uuid.UUID) error {
	if err := s.authorize(ctx, matchID, userID); err != nil {
		return err
	}
	return s.repo.Resolve(ctx, matchID)
}
