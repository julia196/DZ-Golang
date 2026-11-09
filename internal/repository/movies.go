package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mygo/internal/domain"
)

// MoviesRepository — доступ к таблице movies.
type MoviesRepository interface {
	Upsert(ctx context.Context, m domain.Movie) error
	UpsertMany(ctx context.Context, ms []domain.Movie) error
	GetRange(ctx context.Context, from, to int) ([]domain.Movie, error)
	GetMax(ctx context.Context) (domain.Movie, error)
}

type moviesRepo struct {
	pool *pgxpool.Pool
}

func NewMoviesRepository(pool *pgxpool.Pool) MoviesRepository {
	return &moviesRepo{pool: pool}
}

// Upsert вставляет фильм или обновляет, если id уже есть.
func (r *moviesRepo) Upsert(ctx context.Context, m domain.Movie) error {
	const q = `
		INSERT INTO movies (id, title, year, director, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE
		SET title      = EXCLUDED.title,
		    year       = EXCLUDED.year,
		    director   = EXCLUDED.director,
		    updated_at = now()
	`
	if _, err := r.pool.Exec(ctx, q, m.ID, m.Title, m.Year, m.Director); err != nil {
		return fmt.Errorf("upsert id=%d: %w", m.ID, err)
	}
	return nil
}

// UpsertMany — пачками по 500, чтобы не отправлять гигантский batch.
func (r *moviesRepo) UpsertMany(ctx context.Context, ms []domain.Movie) error {
	const chunkSize = 500
	for start := 0; start < len(ms); start += chunkSize {
		end := start + chunkSize
		if end > len(ms) {
			end = len(ms)
		}
		if err := r.upsertChunk(ctx, ms[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (r *moviesRepo) upsertChunk(ctx context.Context, ms []domain.Movie) error {
	if len(ms) == 0 {
		return nil
	}
	const q = `
		INSERT INTO movies (id, title, year, director, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE
		SET title      = EXCLUDED.title,
		    year       = EXCLUDED.year,
		    director   = EXCLUDED.director,
		    updated_at = now()
	`
	batch := &pgx.Batch{}
	for _, m := range ms {
		batch.Queue(q, m.ID, m.Title, m.Year, m.Director)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range ms {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("upsert batch: %w", err)
		}
	}
	return nil
}

// GetRange возвращает фильмы с id в [from; to], сортировка по id.
func (r *moviesRepo) GetRange(ctx context.Context, from, to int) ([]domain.Movie, error) {
	const q = `
		SELECT id, title, year, director
		FROM movies
		WHERE id BETWEEN $1 AND $2
		ORDER BY id
	`
	rows, err := r.pool.Query(ctx, q, from, to)
	if err != nil {
		return nil, fmt.Errorf("query range: %w", err)
	}
	defer rows.Close()

	var out []domain.Movie
	for rows.Next() {
		var m domain.Movie
		if err := rows.Scan(&m.ID, &m.Title, &m.Year, &m.Director); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}

// GetMax возвращает фильм с максимальным id.
func (r *moviesRepo) GetMax(ctx context.Context) (domain.Movie, error) {
	const q = `
		SELECT id, title, year, director
		FROM movies
		ORDER BY id DESC
		LIMIT 1
	`
	var m domain.Movie
	err := r.pool.QueryRow(ctx, q).Scan(&m.ID, &m.Title, &m.Year, &m.Director)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Movie{}, domain.ErrMovieNotFound
		}
		return domain.Movie{}, fmt.Errorf("query max: %w", err)
	}
	return m, nil
}
