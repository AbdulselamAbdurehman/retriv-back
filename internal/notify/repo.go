package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

type Notification struct {
	ID        uuid.UUID
	EntryType string
	EntryID   uuid.UUID
	Title     string
	Message   string
	CreatedAt time.Time
	ReadAt    *time.Time
}

func (r *Repo) Insert(ctx context.Context, userID uuid.UUID, entryType string, entryID uuid.UUID, title, message string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notifications (user_id, entry_type, entry_id, title, message)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, entryType, entryID, title, message)
	if err != nil {
		return fmt.Errorf("notify: insert: %w", err)
	}
	return nil
}

func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]Notification, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, entry_type, entry_id, title, message, created_at, read_at
		 FROM notifications WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("notify: list: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.EntryType, &n.EntryID, &n.Title, &n.Message, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, fmt.Errorf("notify: scan: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
