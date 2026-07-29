package channel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("channel: match not found")

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

type Message struct {
	ID           uuid.UUID
	SenderUserID uuid.UUID
	Body         string
	CreatedAt    time.Time
}

// Participants returns the two user ids and the match status, used to authorize
// access to the masked channel.
func (r *Repo) Participants(ctx context.Context, matchID uuid.UUID) (loserID, finderID uuid.UUID, status string, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT lr.user_id, fr.user_id, m.status
		 FROM matches m
		 JOIN reports lr ON lr.id = m.lost_report_id
		 JOIN reports fr ON fr.id = m.found_report_id
		 WHERE m.id = $1`, matchID,
	).Scan(&loserID, &finderID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, "", ErrNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, "", fmt.Errorf("participants: %w", err)
	}
	return loserID, finderID, status, nil
}

func (r *Repo) Insert(ctx context.Context, matchID, sender uuid.UUID, body string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO messages (match_id, sender_user_id, body) VALUES ($1, $2, $3)`,
		matchID, sender, body)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (r *Repo) List(ctx context.Context, matchID uuid.UUID) ([]Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, sender_user_id, body, created_at FROM messages
		 WHERE match_id = $1 ORDER BY created_at`, matchID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SenderUserID, &m.Body, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Resolve marks the match and both reports resolved (FR-17).
func (r *Repo) Resolve(ctx context.Context, matchID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE matches SET status = 'resolved' WHERE id = $1`, matchID); err != nil {
		return fmt.Errorf("resolve match: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE reports SET status = 'resolved'
		 WHERE id IN (SELECT lost_report_id FROM matches WHERE id = $1
		              UNION SELECT found_report_id FROM matches WHERE id = $1)`, matchID); err != nil {
		return fmt.Errorf("resolve reports: %w", err)
	}
	return tx.Commit(ctx)
}
