package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mygo/internal/domain"

	"github.com/redis/go-redis/v9"
)

// SearchIDs ищет ID фильмов по словам в индексе Redis.
func SearchIDs(ctx context.Context, rdb *redis.Client, words []string) ([]int, error) {
	if len(words) == 0 {
		return nil, nil
	}

	keys := make([]string, len(words))
	for i, w := range words {
		keys[i] = "index:" + w
	}

	ids, err := rdb.SInter(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis sinter: %w", err)
	}

	res := make([]int, 0, len(ids))
	for _, idStr := range ids {
		var id int
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			res = append(res, id)
		}
	}
	return res, nil
}

// GetCachedSearch пытается достать ответ из кэша.
func GetCachedSearch(ctx context.Context, rdb *redis.Client, query string) ([]domain.Movie, bool) {
	key := "cache:search:" + query
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}

	var movies []domain.Movie
	if err := json.Unmarshal([]byte(val), &movies); err != nil {
		return nil, false
	}
	return movies, true
}

// SetCachedSearch сохраняет ответ в кэш на 1 час
func SetCachedSearch(ctx context.Context, rdb *redis.Client, query string, movies []domain.Movie) {
	key := "cache:search:" + query
	data, err := json.Marshal(movies)
	if err != nil {
		return
	}
	rdb.Set(ctx, key, data, 3600*time.Second)
}
