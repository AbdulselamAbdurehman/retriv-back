package matching

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/reports"
)

// DetailProvider yields a finder report's decrypted verification details.
// Implemented by reports.Service.
type DetailProvider interface {
	FinderDetails(ctx context.Context, reportID uuid.UUID) ([]reports.Detail, error)
}

// Verifier prepares verification questions for a freshly opened match.
// Implemented by verification.Service.
type Verifier interface {
	Prepare(ctx context.Context, matchID uuid.UUID, category string, details []reports.Detail) error
}

// Notifier delivers an in-app + email notification. Implemented by notify.Service.
type Notifier interface {
	Notify(ctx context.Context, userID uuid.UUID, entryType string, entryID uuid.UUID, title, message string) error
}

type Config struct {
	RadiusMeters   float64
	TimeWindow     time.Duration
	MaxCosineDist  float64
	CandidateLimit int
}

type Service struct {
	repo     *Repo
	details  DetailProvider
	verifier Verifier
	notifier Notifier
	cfg      Config
}

func NewService(repo *Repo, details DetailProvider, verifier Verifier, notifier Notifier, cfg Config) *Service {
	return &Service{repo: repo, details: details, verifier: verifier, notifier: notifier, cfg: cfg}
}

// Result is the binary outcome exposed to the reporter (FR-9: no hints).
type Result struct {
	Matched bool // a candidate cleared the threshold; verification is now pending
	MatchID uuid.UUID
}

// FindAndOpen checks a newly submitted report against the opposite-type pool and,
// if the best candidate clears the similarity threshold, opens a match and kicks
// off verification. It never returns any detail about the candidate.
func (s *Service) FindAndOpen(ctx context.Context, reportID uuid.UUID) (Result, error) {
	f, err := s.repo.loadFacts(ctx, reportID)
	if err != nil {
		return Result{}, err
	}

	q := candidateQuery{
		oppositeKind: opposite(f.Kind),
		category:     f.Category,
		emb:          f.Embedding,
		hasLoc:       f.HasLocation,
		lng:          f.Lng,
		lat:          f.Lat,
		radius:       s.cfg.RadiusMeters,
		maxDistance:  s.cfg.MaxCosineDist,
		limit:        s.cfg.CandidateLimit,
	}
	if f.EventAt != nil {
		lo := f.EventAt.Add(-s.cfg.TimeWindow)
		hi := f.EventAt.Add(s.cfg.TimeWindow)
		q.eventLo, q.eventHi = &lo, &hi
	}

	candidates, err := s.repo.findCandidates(ctx, q)
	if err != nil {
		return Result{}, err
	}
	if len(candidates) == 0 {
		return Result{Matched: false}, nil
	}
	best := candidates[0]

	// Resolve which side is the lost report (its owner is the loser who answers).
	var lostID, foundID, loserID uuid.UUID
	if f.Kind == reports.KindLost {
		lostID, foundID, loserID = reportID, best.ReportID, f.UserID
	} else {
		lostID, foundID, loserID = best.ReportID, reportID, best.UserID
	}

	matchID, err := s.repo.createMatch(ctx, lostID, foundID)
	if err != nil {
		return Result{}, err
	}

	details, err := s.details.FinderDetails(ctx, foundID)
	if err != nil {
		return Result{}, err
	}
	if err := s.verifier.Prepare(ctx, matchID, f.Category, details); err != nil {
		return Result{}, err
	}

	_ = s.notifier.Notify(ctx, loserID, "match", matchID,
		"Verify your item",
		"A possible match was found. Answer a couple of questions to confirm it's yours.")

	return Result{Matched: true, MatchID: matchID}, nil
}

func opposite(kind string) string {
	if kind == reports.KindLost {
		return reports.KindFound
	}
	return reports.KindLost
}
