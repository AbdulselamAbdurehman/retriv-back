package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("auth: not found")

// User is the account record. Names and phone are collected at registration.
type User struct {
	ID         uuid.UUID
	Email      string
	FirstName  string
	MiddleName string // optional
	LastName   string
	Phone      string
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) CreateUser(ctx context.Context, u User) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, first_name, middle_name, last_name, phone)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5)
		 RETURNING id`,
		u.Email, u.FirstName, u.MiddleName, u.LastName, u.Phone,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (r *Repo) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	var middle *string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, first_name, middle_name, last_name, phone
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.FirstName, &middle, &u.LastName, &u.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("user by email: %w", err)
	}
	if middle != nil {
		u.MiddleName = *middle
	}
	return u, nil
}

func (r *Repo) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	var middle *string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, first_name, middle_name, last_name, phone FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.FirstName, &middle, &u.LastName, &u.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("user by id: %w", err)
	}
	if middle != nil {
		u.MiddleName = *middle
	}
	return u, nil
}

func (r *Repo) EmailByID(ctx context.Context, userID uuid.UUID) (string, error) {
	var email string
	err := r.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("email by id: %w", err)
	}
	return email, nil
}

func (r *Repo) CreateMagicLink(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO magic_links (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create magic link: %w", err)
	}
	return nil
}

// ConsumeMagicLink atomically marks an unused, unexpired link as used and
// returns its user. A used or expired token yields ErrNotFound.
func (r *Repo) ConsumeMagicLink(ctx context.Context, tokenHash []byte) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx,
		`UPDATE magic_links SET used_at = now()
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		 RETURNING user_id`, tokenHash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("consume magic link: %w", err)
	}
	return userID, nil
}
