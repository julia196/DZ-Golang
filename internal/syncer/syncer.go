package syncer

import (
	"context"
	"errors"
	"fmt"
	"log"

	"mygo/internal/domain"
	"mygo/internal/repository"
	"mygo/internal/usecase"
)

// Run синхронизирует локальную БД с внешним стендом:
// находит последний id на стенде и докачивает отсутствующие фильмы.
// повторный запуск при актуальной БД ничего не делает.
func Run(ctx context.Context, provider usecase.MovieProvider, repo repository.MoviesRepository) error {
	lastID, err := findLastID(ctx, provider)
	if err != nil {
		return fmt.Errorf("find last id on stand: %w", err)
	}

	var maxID int
	if m, err := repo.GetMax(ctx); err == nil {
		maxID = m.ID
	} else if !errors.Is(err, domain.ErrMovieNotFound) {
		return fmt.Errorf("get max id in db: %w", err)
	}

	if maxID >= lastID {
		log.Printf("sync: db up to date (db_max_id=%d, stand_last_id=%d)", maxID, lastID)
		return nil
	}

	log.Printf("sync: loading movies %d..%d (%d total)", maxID+1, lastID, lastID-maxID)

	movies := make([]domain.Movie, 0, lastID-maxID)
	for id := maxID + 1; id <= lastID; id++ {
		m, err := provider.GetMovie(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrMovieNotFound) {
				log.Printf("sync: id=%d reported as last but not found, skipping", id)
				continue
			}
			return fmt.Errorf("fetch id=%d: %w", id, err)
		}
		movies = append(movies, m)
	}

	if err := repo.UpsertMany(ctx, movies); err != nil {
		return fmt.Errorf("save movies: %w", err)
	}

	log.Printf("sync: loaded %d movies", len(movies))
	return nil
}

// findLastID находит максимальный существующий id на стенде
func findLastID(ctx context.Context, provider usecase.MovieProvider) (int, error) {
	lo, hi := 1, 1

	for {
		_, err := provider.GetMovie(ctx, hi)
		if errors.Is(err, domain.ErrMovieNotFound) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("id=%d: %w", hi, err)
		}
		lo = hi
		hi *= 2
		if hi > 1<<30 {
			return 0, fmt.Errorf("last movie not found: probe limit exceeded")
		}
	}

	for lo+1 < hi {
		mid := (lo + hi) / 2
		_, err := provider.GetMovie(ctx, mid)
		switch {
		case errors.Is(err, domain.ErrMovieNotFound):
			hi = mid
		case err != nil:
			return 0, fmt.Errorf("id=%d: %w", mid, err)
		default:
			lo = mid
		}
	}
	return lo, nil
}
