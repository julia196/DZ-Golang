package usecase

import (
	"context"
	"errors"
	"fmt"

	"mygo/internal/domain"
)

type MovieProvider interface {
	GetMovie(ctx context.Context, id int) (domain.Movie, error)
}

type MovieUseCase interface {
	LoadMovies(ctx context.Context, from, to int) (*LoadResult, error)
	GetLastMovie(ctx context.Context) (domain.Movie, error)
}

// LoadResult — результат загрузки диапазона с частичными ошибками.
type LoadResult struct {
	Movies     []domain.Movie `json:"movies"`
	MissingIDs []int          `json:"missing_ids,omitempty"`
	FailedIDs  []FailedID     `json:"failed_ids,omitempty"`
}

// FailedID — ошибка загрузки конкретного id.
type FailedID struct {
	ID     int    `json:"id"`
	Reason string `json:"reason"`
}

// movieUseCase — приватная реализация MovieUseCase.
type movieUseCase struct {
	provider MovieProvider
}

// NewMovieUseCase создаёт usecase с заданным источником фильмов.
func NewMovieUseCase(provider MovieProvider) MovieUseCase {
	return &movieUseCase{provider: provider}
}

// LoadMovies загружает фильмы в диапазоне [from; to].
func (u *movieUseCase) LoadMovies(ctx context.Context, from, to int) (*LoadResult, error) {
	if from < 1 || to < 1 || from > to {
		return nil, domain.ErrInvalidRange
	}
	var result LoadResult

	for id := from; id <= to; id++ {
		m, err := u.provider.GetMovie(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrMovieNotFound) {
				result.MissingIDs = append(result.MissingIDs, id)
				continue
			}
			result.FailedIDs = append(result.FailedIDs, FailedID{
				ID:     id,
				Reason: err.Error(),
			})
			continue
		}
		result.Movies = append(result.Movies, m)
	}
	return &result, nil
}

// GetLastMovie находит последний доступный фильм на стенде
func (u *movieUseCase) GetLastMovie(ctx context.Context) (domain.Movie, error) {
	lo, hi := 1, 1

	for {
		_, err := u.provider.GetMovie(ctx, hi)
		if errors.Is(err, domain.ErrMovieNotFound) {
			break
		}
		if err != nil {
			return domain.Movie{}, fmt.Errorf("id=%d: %w", hi, err)
		}
		lo = hi
		hi *= 2
		if hi > 1<<30 {
			return domain.Movie{}, fmt.Errorf("last movie not found: probe limit exceeded")
		}
	}

	for lo+1 < hi {
		mid := (lo + hi) / 2
		_, err := u.provider.GetMovie(ctx, mid)
		switch {
		case errors.Is(err, domain.ErrMovieNotFound):
			hi = mid
		case err != nil:
			return domain.Movie{}, fmt.Errorf("id=%d: %w", mid, err)
		default:
			lo = mid
		}
	}
	return u.provider.GetMovie(ctx, lo)
}
