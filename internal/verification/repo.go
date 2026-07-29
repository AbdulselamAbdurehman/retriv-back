package verification

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("verification: not found")

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// MatchInfo is the match plus the two parties' user ids.
type MatchInfo struct {
	ID            uuid.UUID
	LostReportID  uuid.UUID
	FoundReportID uuid.UUID
	LoserUserID   uuid.UUID // owner of the lost report; the one who answers
	FinderUserID  uuid.UUID
	Status        string
	AttemptsUsed  int
}

func (r *Repo) LoadMatch(ctx context.Context, matchID uuid.UUID) (MatchInfo, error) {
	var m MatchInfo
	err := r.pool.QueryRow(ctx,
		`SELECT m.id, m.lost_report_id, m.found_report_id, lr.user_id, fr.user_id, m.status, m.attempts_used
		 FROM matches m
		 JOIN reports lr ON lr.id = m.lost_report_id
		 JOIN reports fr ON fr.id = m.found_report_id
		 WHERE m.id = $1`, matchID,
	).Scan(&m.ID, &m.LostReportID, &m.FoundReportID, &m.LoserUserID, &m.FinderUserID, &m.Status, &m.AttemptsUsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return MatchInfo{}, ErrNotFound
	}
	if err != nil {
		return MatchInfo{}, fmt.Errorf("load match: %w", err)
	}
	return m, nil
}

// Question is a stored verification question. DetailID points at the encrypted
// finder detail it grades against; the expected answer is never stored here.
type Question struct {
	ID       uuid.UUID
	Prompt   string
	DetailID uuid.UUID
}

func (r *Repo) InsertQuestion(ctx context.Context, matchID uuid.UUID, prompt string, detailID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO verification_questions (match_id, prompt, detail_id) VALUES ($1, $2, $3)`,
		matchID, prompt, detailID)
	if err != nil {
		return fmt.Errorf("insert question: %w", err)
	}
	return nil
}

func (r *Repo) Questions(ctx context.Context, matchID uuid.UUID) ([]Question, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, prompt, detail_id FROM verification_questions WHERE match_id = $1 ORDER BY created_at`,
		matchID)
	if err != nil {
		return nil, fmt.Errorf("query questions: %w", err)
	}
	defer rows.Close()

	var out []Question
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.ID, &q.Prompt, &q.DetailID); err != nil {
			return nil, fmt.Errorf("scan question: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *Repo) InsertAnswer(ctx context.Context, questionID uuid.UUID, answer string, score float64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO verification_answers (question_id, answer_text, score) VALUES ($1, $2, $3)`,
		questionID, answer, score)
	if err != nil {
		return fmt.Errorf("insert answer: %w", err)
	}
	return nil
}

// IncrementAttempts bumps and returns the match's attempt counter.
func (r *Repo) IncrementAttempts(ctx context.Context, matchID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`UPDATE matches SET attempts_used = attempts_used + 1 WHERE id = $1 RETURNING attempts_used`,
		matchID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("increment attempts: %w", err)
	}
	return n, nil
}

func (r *Repo) SetOutcome(ctx context.Context, matchID uuid.UUID, status string, confidence float64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE matches SET status = $2, confidence = $3 WHERE id = $1`, matchID, status, confidence)
	if err != nil {
		return fmt.Errorf("set outcome: %w", err)
	}
	return nil
}
