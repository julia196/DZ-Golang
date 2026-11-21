package usecase

import (
	"context"

	"log"
	"mygo/internal/cache"
	"mygo/internal/domain"
	"mygo/internal/repository"
	"strings"
	"unicode"

	"github.com/redis/go-redis/v9"
)

// MovieProvider — источник фильмов со стенда.
// Оставляем этот интерфейс, так как его использует syncer для синхронизации.
type MovieProvider interface {
	GetMovie(ctx context.Context, id int) (domain.Movie, error)
}

type MovieUseCase interface {
	LoadMovies(ctx context.Context, from, to int) (*LoadResult, error)
	GetLastMovie(ctx context.Context) (domain.Movie, error)
	Search(ctx context.Context, query string) ([]domain.Movie, error)
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
	repo repository.MoviesRepository
	rdb  *redis.Client
}

// NewMovieUseCase создаёт usecase с заданным репозиторием.
func NewMovieUseCase(repo repository.MoviesRepository, rdb *redis.Client) MovieUseCase {
	return &movieUseCase{repo: repo, rdb: rdb}
}

// LoadMovies загружает фильмы в диапазоне [from; to] из БД.
func (u *movieUseCase) LoadMovies(ctx context.Context, from, to int) (*LoadResult, error) {
	if from < 1 || to < 1 || from > to {
		return nil, domain.ErrInvalidRange
	}

	found, err := u.repo.GetRange(ctx, from, to)
	if err != nil {
		if err.Error() == "database is unavailable" {
			log.Printf("WARNING: database is unavailable, returning empty result for load-movies")
			return &LoadResult{Movies: []domain.Movie{}, MissingIDs: []int{}}, nil
		}
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

// GetLastMovie находит последний доступный фильм в БД.
func (u *movieUseCase) GetLastMovie(ctx context.Context) (domain.Movie, error) {
	m, err := u.repo.GetMax(ctx)
	if err != nil {
		if err.Error() == "database is unavailable" {
			return domain.Movie{}, domain.ErrMovieNotFound // Возвращаем 404
		}
		return domain.Movie{}, err
	}
	return m, nil
}

func (u *movieUseCase) Search(ctx context.Context, query string) ([]domain.Movie, error) {
	if movies, ok := cache.GetCachedSearch(ctx, u.rdb, query); ok {
		return movies, nil
	}

	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	ids, err := cache.SearchIDs(ctx, u.rdb, words)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		empty := make([]domain.Movie, 0)
		cache.SetCachedSearch(ctx, u.rdb, query, empty) // Кэшируем пустой ответ
		return empty, nil
	}

	movies, err := u.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	cache.SetCachedSearch(ctx, u.rdb, query, movies)

	return movies, nil
}
