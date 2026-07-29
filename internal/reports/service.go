package reports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/AbdulselamAbdurehman/retriv-back/internal/crypto"
	"github.com/AbdulselamAbdurehman/retriv-back/internal/ollama"
)

var ErrQuotaExceeded = errors.New("reports: daily lost-report quota exceeded")

type Service struct {
	repo       *Repo
	embedder   ollama.Embedder
	cipher     *crypto.Cipher
	lostPerDay int
	ttl        time.Duration
}

func NewService(repo *Repo, embedder ollama.Embedder, cipher *crypto.Cipher, lostPerDay int, ttl time.Duration) *Service {
	return &Service{repo: repo, embedder: embedder, cipher: cipher, lostPerDay: lostPerDay, ttl: ttl}
}

// Submit validates, enforces the lost-report quota, embeds the description,
// encrypts finder details, and stores the report.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (Report, error) {
	if in.Kind != KindLost && in.Kind != KindFound {
		return Report{}, fmt.Errorf("reports: invalid kind %q", in.Kind)
	}
	// Only losers are rate-limited; found reporting is unlimited (FR-18).
	if in.Kind == KindLost {
		n, err := s.repo.CountLostSince(ctx, in.UserID, startOfDay(time.Now()))
		if err != nil {
			return Report{}, err
		}
		if n >= s.lostPerDay {
			return Report{}, ErrQuotaExceeded
		}
	}

	emb, err := s.embedder.Embed(ctx, in.Category+": "+in.Description)
	if err != nil {
		return Report{}, err
	}

	encrypted := make([]EncryptedDetail, 0, len(in.Details))
	if in.Kind == KindFound {
		for _, d := range in.Details {
			ct, nonce, err := s.cipher.Encrypt([]byte(d.Value))
			if err != nil {
				return Report{}, err
			}
			encrypted = append(encrypted, EncryptedDetail{Kind: d.Kind, Ciphertext: ct, Nonce: nonce})
		}
	}

	rep := Report{
		UserID:      in.UserID,
		Kind:        in.Kind,
		Category:    in.Category,
		Description: in.Description,
		Lat:         in.Lat,
		Lng:         in.Lng,
		HasLocation: in.HasLocation,
		EventAt:     in.EventAt,
		Status:      StatusActive,
		ExpiresAt:   time.Now().Add(s.ttl),
	}
	id, err := s.repo.Create(ctx, rep, emb, encrypted)
	if err != nil {
		return Report{}, err
	}
	rep.ID = id
	return rep, nil
}

// FinderDetails decrypts a finder report's stored verification details. Used by
// the verification flow to generate and grade questions.
func (s *Service) FinderDetails(ctx context.Context, reportID uuid.UUID) ([]Detail, error) {
	enc, err := s.repo.EncryptedDetails(ctx, reportID)
	if err != nil {
		return nil, err
	}
	out := make([]Detail, 0, len(enc))
	for _, e := range enc {
		pt, err := s.cipher.Decrypt(e.Ciphertext, e.Nonce)
		if err != nil {
			return nil, err
		}
		out = append(out, Detail{ID: e.ID, Kind: e.Kind, Value: string(pt)})
	}
	return out, nil
}

// DetailValue decrypts a single verification detail by id (used when grading).
func (s *Service) DetailValue(ctx context.Context, detailID uuid.UUID) (string, error) {
	e, err := s.repo.EncryptedDetailByID(ctx, detailID)
	if err != nil {
		return "", err
	}
	pt, err := s.cipher.Decrypt(e.Ciphertext, e.Nonce)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Report, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID) ([]Report, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	return s.repo.SetStatus(ctx, id, status)
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
