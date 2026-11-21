package syncer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

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

	const workerCount = 20 // 20 параллельных запросов к стенду
	const chunkSize = 1000 // Сохраняем в БД пачками по 1000

	jobs := make(chan int, workerCount)
	results := make(chan domain.Movie, chunkSize*2)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				m, err := provider.GetMovie(ctx, id)
				if err != nil {
					if errors.Is(err, domain.ErrMovieNotFound) {
						continue // пропускаем 404
					}
					log.Printf("sync: fetch id=%d failed: %v", id, err)
					continue
				}
				results <- m
			}
		}()
	}

	go func() {
		defer close(jobs)
		for id := maxID + 1; id <= lastID; id++ {
			select {
			case <-ctx.Done():
				return
			case jobs <- id:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var movies []domain.Movie
	savedCount := 0
	for m := range results {
		movies = append(movies, m)
		if len(movies) >= chunkSize {
			if err := repo.UpsertMany(ctx, movies); err != nil {
				return fmt.Errorf("save chunk: %w", err)
			}
			savedCount += len(movies)
			log.Printf("sync: saved %d movies (total: %d)", len(movies), savedCount)
			movies = movies[:0]
		}
	}

	if len(movies) > 0 {
		if err := repo.UpsertMany(ctx, movies); err != nil {
			return fmt.Errorf("save final chunk: %w", err)
		}
		savedCount += len(movies)
		log.Printf("sync: saved %d final movies (total: %d)", len(movies), savedCount)
	}

	log.Printf("sync: completed up to id=%d", lastID)
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
