package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailTaken = errors.New("user: email already registered")
	ErrNotFound   = errors.New("user: not found")
	ErrTokenNotFound = errors.New("user: refresh token not found")
)

// User is the in-memory shape of a row in auth.users.
// PasswordHash is included for login; never log it or return it over HTTP.
type User struct {
	ID           string
	Email        string
	PasswordHash string
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Create(ctx context.Context, email, passwordHash string) (*User, error) {
	const q = `
		INSERT INTO auth.users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, q, email, passwordHash).
		Scan(&u.ID, &u.Email, &u.PasswordHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("user: create: %w", err)
	}
	return u, nil
}

func (r *Repo) FindByEmail(ctx context.Context, email string) (*User, error) {
	const q = `
		SELECT id, email, password_hash
		FROM auth.users
		WHERE email = $1
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, q, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by email: %w", err)
	}
	return u, nil
}

func (r *Repo) FindById(ctx context.Context, id string) (*User, error) {
	const q = `
		SELECT id, email, password_hash
		FROM auth.users
		WHERE id = $1
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&u.ID, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user: find by id: %w", err)
	}
	return u, nil
}

// StoreRefreshToken persists a refresh token's hash for the given user.
// The raw token is never stored — only its SHA-256 hash.
func (r *Repo) StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	const q = `
		INSERT INTO auth.refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`

	if _, err := r.pool.Exec(ctx, q, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("user: store refresh token: %w", err)
	}
	return nil
}

// RefreshToken is the in-memory shape of a row in auth.refresh_tokens.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time // nil if not revoked
}

// FindRefreshToken looks up an active token by hash.
// Returns ErrTokenNotFound if the token doesn't exist, is revoked, or is expired.
func (r *Repo) FindRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, revoked_at
		FROM auth.refresh_tokens
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
	`

	t := &RefreshToken{}
	err := r.pool.QueryRow(ctx, q, tokenHash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user: find refresh token: %w", err)
	}
	return t, nil
}

// RevokeRefreshToken marks a token as revoked.
// Idempotent: revoking an already-revoked or missing token is not an error.
func (r *Repo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	const q = `
		UPDATE auth.refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1
		  AND revoked_at IS NULL
	`

	if _, err := r.pool.Exec(ctx, q, tokenHash); err != nil {
		return fmt.Errorf("user: revoke refresh token: %w", err)
	}
	return nil
}