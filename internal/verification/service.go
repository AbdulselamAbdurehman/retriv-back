package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/matching"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/ollama"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/reports"
)

// DetailResolver decrypts a single finder detail by id, at grading time.
// Implemented by reports.Service.
type DetailResolver interface {
	DetailValue(ctx context.Context, detailID uuid.UUID) (string, error)
}

type Config struct {
	MinConfidence float64
	MaxAttempts   int
}

type Service struct {
	repo     *Repo
	chat     ollama.Chatter
	resolver DetailResolver
	cfg      Config
}

func NewService(repo *Repo, chat ollama.Chatter, resolver DetailResolver, cfg Config) *Service {
	return &Service{repo: repo, chat: chat, resolver: resolver, cfg: cfg}
}

var ErrNotPending = errors.New("verification: match is not awaiting verification")

// Prepare generates and stores one verification question per finder detail (up
// to two), each phrased to ask for the detail without revealing it (FR-10/11).
func (s *Service) Prepare(ctx context.Context, matchID uuid.UUID, category string, details []reports.Detail) error {
	n := len(details)
	if n > 2 {
		n = 2
	}
	for i := 0; i < n; i++ {
		prompt, err := s.generateQuestion(ctx, category, details[i])
		if err != nil {
			return err
		}
		if err := s.repo.InsertQuestion(ctx, matchID, prompt, details[i].ID); err != nil {
			return err
		}
	}
	return nil
}

// Match exposes the match record (and both parties) to handlers.
func (s *Service) Match(ctx context.Context, matchID uuid.UUID) (MatchInfo, error) {
	return s.repo.LoadMatch(ctx, matchID)
}

// Questions lists the questions for a match (prompts only; no expected answers).
func (s *Service) Questions(ctx context.Context, matchID uuid.UUID) ([]Question, error) {
	return s.repo.Questions(ctx, matchID)
}

// Outcome is the result of a verification attempt.
type Outcome struct {
	Match        MatchInfo
	Confidence   float64
	Passed       bool // cleared MinConfidence -> match verified
	Discarded    bool // ran out of attempts -> candidate discarded
	AttemptsUsed int
	MaxAttempts  int
}

// SubmitAnswers grades each answer against its stored detail, combines the
// scores, and updates the match. No per-question feedback is returned (FR-9).
func (s *Service) SubmitAnswers(ctx context.Context, matchID uuid.UUID, answers map[uuid.UUID]string) (Outcome, error) {
	m, err := s.repo.LoadMatch(ctx, matchID)
	if err != nil {
		return Outcome{}, err
	}
	if m.Status != matching.StatusPendingVerify {
		return Outcome{}, ErrNotPending
	}

	qs, err := s.repo.Questions(ctx, matchID)
	if err != nil {
		return Outcome{}, err
	}
	if len(qs) == 0 {
		return Outcome{}, fmt.Errorf("verification: match has no questions")
	}

	var total float64
	for _, q := range qs {
		answer := strings.TrimSpace(answers[q.ID])
		var score float64
		if answer != "" {
			expected, err := s.resolver.DetailValue(ctx, q.DetailID)
			if err != nil {
				return Outcome{}, err
			}
			if score, err = s.grade(ctx, expected, answer); err != nil {
				return Outcome{}, err
			}
		}
		if err := s.repo.InsertAnswer(ctx, q.ID, answer, score); err != nil {
			return Outcome{}, err
		}
		total += score
	}
	confidence := total / float64(len(qs))

	attempts, err := s.repo.IncrementAttempts(ctx, matchID)
	if err != nil {
		return Outcome{}, err
	}

	out := Outcome{Match: m, Confidence: confidence, AttemptsUsed: attempts, MaxAttempts: s.cfg.MaxAttempts}
	switch {
	case confidence >= s.cfg.MinConfidence:
		if err := s.repo.SetOutcome(ctx, matchID, matching.StatusVerified, confidence); err != nil {
			return Outcome{}, err
		}
		out.Passed = true
	case attempts >= s.cfg.MaxAttempts:
		if err := s.repo.SetOutcome(ctx, matchID, matching.StatusDiscarded, confidence); err != nil {
			return Outcome{}, err
		}
		out.Discarded = true
	}
	return out, nil
}

func (s *Service) generateQuestion(ctx context.Context, category string, d reports.Detail) (string, error) {
	system := "You write ONE short verification question for a lost-and-found service. " +
		"The claimant must state a private detail only the true owner would know. " +
		"Never reveal, quote, or hint at the answer. Output only the question text, nothing else."
	user := fmt.Sprintf("Item category: %s\nDetail type: %s\nThe true (secret) detail is: %s\n\nWrite the question.",
		category, d.Kind, d.Value)
	out, err := s.chat.Chat(ctx, system, user, false)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) grade(ctx context.Context, expected, answer string) (float64, error) {
	system := "You grade a lost-and-found verification answer. Compare the claimant's answer to the " +
		"true detail. Reply ONLY with JSON {\"score\": n} where n is 0..1 (1 = clearly correct, " +
		"0 = clearly wrong). Be strict; reject vague guesses."
	user := fmt.Sprintf("True detail: %s\nClaimant answer: %s", expected, answer)
	out, err := s.chat.Chat(ctx, system, user, true)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return 0, fmt.Errorf("verification: parse grade %q: %w", out, err)
	}
	switch {
	case parsed.Score < 0:
		return 0, nil
	case parsed.Score > 1:
		return 1, nil
	default:
		return parsed.Score, nil
	}
}
