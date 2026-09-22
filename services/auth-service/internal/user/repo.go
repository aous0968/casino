package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailTaken = errors.New("user: email already registered")
	ErrNotFound   = errors.New("user: not found")
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