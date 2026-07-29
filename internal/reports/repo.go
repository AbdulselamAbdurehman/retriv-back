package reports

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

var ErrNotFound = errors.New("reports: not found")

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create inserts the report (with embedding + optional location) and its
// encrypted verification details in one transaction.
func (r *Repo) Create(ctx context.Context, rep Report, emb pgvector.Vector, details []EncryptedDetail) (uuid.UUID, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// location is built from lng/lat when present, else NULL.
	var lng, lat *float64
	if rep.HasLocation {
		lng, lat = &rep.Lng, &rep.Lat
	}

	var id uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO reports
		   (user_id, kind, category, description, description_embedding, location, event_at, expires_at)
		 VALUES
		   ($1, $2, $3, $4, $5,
		    CASE WHEN $6::float8 IS NULL THEN NULL
		         ELSE ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography END,
		    $8, $9)
		 RETURNING id`,
		rep.UserID, rep.Kind, rep.Category, rep.Description, emb,
		lng, lat, rep.EventAt, rep.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert report: %w", err)
	}

	for _, d := range details {
		if _, err := tx.Exec(ctx,
			`INSERT INTO verification_details (report_id, kind, ciphertext, nonce)
			 VALUES ($1, $2, $3, $4)`,
			id, d.Kind, d.Ciphertext, d.Nonce,
		); err != nil {
			return uuid.Nil, fmt.Errorf("insert detail: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// CountLostSince counts a user's lost-item reports created at/after ts (FR-18 quota).
func (r *Repo) CountLostSince(ctx context.Context, userID uuid.UUID, ts time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM reports
		 WHERE user_id = $1 AND kind = 'lost' AND created_at >= $2`,
		userID, ts,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count lost: %w", err)
	}
	return n, nil
}

func (r *Repo) Get(ctx context.Context, id uuid.UUID) (Report, error) {
	var rep Report
	var lat, lng *float64
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, kind, category, description,
		        ST_Y(location::geometry), ST_X(location::geometry),
		        event_at, status, expires_at, created_at
		 FROM reports WHERE id = $1`, id,
	).Scan(&rep.ID, &rep.UserID, &rep.Kind, &rep.Category, &rep.Description,
		&lat, &lng, &rep.EventAt, &rep.Status, &rep.ExpiresAt, &rep.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("get report: %w", err)
	}
	if lat != nil && lng != nil {
		rep.Lat, rep.Lng, rep.HasLocation = *lat, *lng, true
	}
	return rep, nil
}

// ListByUser returns a user's reports, newest first (for the "my reports" view).
func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]Report, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, kind, category, description, event_at, status, expires_at, created_at
		 FROM reports WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()

	var out []Report
	for rows.Next() {
		var rep Report
		if err := rows.Scan(&rep.ID, &rep.UserID, &rep.Kind, &rep.Category, &rep.Description,
			&rep.EventAt, &rep.Status, &rep.ExpiresAt, &rep.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// EncryptedDetails returns the stored ciphertext details for a report.
func (r *Repo) EncryptedDetails(ctx context.Context, reportID uuid.UUID) ([]EncryptedDetail, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, kind, ciphertext, nonce FROM verification_details WHERE report_id = $1 ORDER BY created_at`,
		reportID)
	if err != nil {
		return nil, fmt.Errorf("query details: %w", err)
	}
	defer rows.Close()

	var out []EncryptedDetail
	for rows.Next() {
		var d EncryptedDetail
		if err := rows.Scan(&d.ID, &d.Kind, &d.Ciphertext, &d.Nonce); err != nil {
			return nil, fmt.Errorf("scan detail: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// EncryptedDetailByID fetches a single detail (used when grading an answer).
func (r *Repo) EncryptedDetailByID(ctx context.Context, id uuid.UUID) (EncryptedDetail, error) {
	var d EncryptedDetail
	err := r.pool.QueryRow(ctx,
		`SELECT id, kind, ciphertext, nonce FROM verification_details WHERE id = $1`, id,
	).Scan(&d.ID, &d.Kind, &d.Ciphertext, &d.Nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return EncryptedDetail{}, ErrNotFound
	}
	if err != nil {
		return EncryptedDetail{}, fmt.Errorf("get detail: %w", err)
	}
	return d, nil
}

func (r *Repo) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE reports SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	return nil
}
