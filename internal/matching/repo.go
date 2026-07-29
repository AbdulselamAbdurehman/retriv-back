package matching

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// status values for matches
const (
	StatusPendingVerify = "pending_verify"
	StatusVerified      = "verified"
	StatusResolved      = "resolved"
	StatusDiscarded     = "discarded"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// reportFacts is everything matching needs about the newly submitted report.
type reportFacts struct {
	UserID      uuid.UUID
	Kind        string
	Category    string
	Embedding   pgvector.Vector
	HasLocation bool
	Lng, Lat    float64
	EventAt     *time.Time
}

func (r *Repo) loadFacts(ctx context.Context, reportID uuid.UUID) (reportFacts, error) {
	var f reportFacts
	var lng, lat *float64
	err := r.pool.QueryRow(ctx,
		`SELECT user_id, kind, category, description_embedding,
		        location IS NOT NULL, ST_X(location::geometry), ST_Y(location::geometry), event_at
		 FROM reports WHERE id = $1`, reportID,
	).Scan(&f.UserID, &f.Kind, &f.Category, &f.Embedding, &f.HasLocation, &lng, &lat, &f.EventAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return reportFacts{}, fmt.Errorf("matching: report %s not found", reportID)
	}
	if err != nil {
		return reportFacts{}, fmt.Errorf("load facts: %w", err)
	}
	if lng != nil && lat != nil {
		f.Lng, f.Lat = *lng, *lat
	}
	return f, nil
}

// Candidate is an opposite-type report that passed the coarse filters, ranked by
// cosine distance (lower = more similar).
type Candidate struct {
	ReportID uuid.UUID
	UserID   uuid.UUID
	Distance float64
}

type candidateQuery struct {
	oppositeKind string
	category     string
	emb          pgvector.Vector
	hasLoc       bool
	lng, lat     float64
	radius       float64
	eventLo      *time.Time
	eventHi      *time.Time
	maxDistance  float64
	limit        int
}

// findCandidates runs the coarse filter (category + geo proximity + time window)
// then ranks survivors by embedding cosine distance, keeping only those within
// maxDistance.
func (r *Repo) findCandidates(ctx context.Context, q candidateQuery) ([]Candidate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, (description_embedding <=> $3) AS dist
		 FROM reports
		 WHERE kind = $1 AND category = $2 AND status = 'active'
		   AND (NOT $4 OR (location IS NOT NULL
		        AND ST_DWithin(location, ST_SetSRID(ST_MakePoint($5, $6), 4326)::geography, $7)))
		   AND ($8::timestamptz IS NULL OR event_at IS NULL OR event_at BETWEEN $8 AND $9)
		   AND (description_embedding <=> $3) <= $10
		 ORDER BY dist
		 LIMIT $11`,
		q.oppositeKind, q.category, q.emb, q.hasLoc, q.lng, q.lat, q.radius,
		q.eventLo, q.eventHi, q.maxDistance, q.limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find candidates: %w", err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ReportID, &c.UserID, &c.Distance); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// createMatch inserts a pending match for a lost/found pair. If the pair already
// has a match row, its id is returned unchanged.
func (r *Repo) createMatch(ctx context.Context, lostID, foundID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`INSERT INTO matches (lost_report_id, found_report_id)
		 VALUES ($1, $2)
		 ON CONFLICT (lost_report_id, found_report_id) DO UPDATE SET lost_report_id = EXCLUDED.lost_report_id
		 RETURNING id`,
		lostID, foundID,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create match: %w", err)
	}
	return id, nil
}
