package usecase

import (
	"context"

	"mygo/internal/domain"
	"mygo/internal/repository"
)

// MovieProvider — источник фильмов со стенда.
type MovieProvider interface {
	GetMovie(ctx context.Context, id int) (domain.Movie, error)
}

// MovieUseCase — бизнес-логика над локальной БД.
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

type movieUseCase struct {
	repo repository.MoviesRepository
}

// NewMovieUseCase создаёт usecase поверх репозитория БД.
func NewMovieUseCase(repo repository.MoviesRepository) MovieUseCase {
	return &movieUseCase{repo: repo}
}

// LoadMovies возвращает фильмы в диапазоне [from; to] из БД.
// id, которых нет в БД, попадают в MissingIDs.
func (u *movieUseCase) LoadMovies(ctx context.Context, from, to int) (*LoadResult, error) {
	if from < 1 || to < 1 || from > to {
		return nil, domain.ErrInvalidRange
	}

	found, err := u.repo.GetRange(ctx, from, to)
	if err != nil {
		return nil, err
	}

	have := make(map[int]struct{}, len(found))
	for _, m := range found {
		have[m.ID] = struct{}{}
	}

	result := LoadResult{
		Movies:     make([]domain.Movie, 0, len(found)),
		MissingIDs: make([]int, 0),
	}

	for id := from; id <= to; id++ {
		if _, ok := have[id]; ok {
			continue
		}
		result.MissingIDs = append(result.MissingIDs, id)
	}
	result.Movies = found

	return &result, nil
}

// GetLastMovie возвращает фильм с максимальным id из БД.
func (u *movieUseCase) GetLastMovie(ctx context.Context) (domain.Movie, error) {
	return u.repo.GetMax(ctx)
}
