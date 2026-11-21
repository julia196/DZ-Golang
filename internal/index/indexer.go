package index

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode"

	"mygo/internal/domain"

	"github.com/redis/go-redis/v9"
)

// BuildIndex строит обратный индекс в Redis и сбрасывает старый кэш.
func BuildIndex(ctx context.Context, rdb *redis.Client, movies []domain.Movie) error {
	if err := clearCache(ctx, rdb); err != nil {
		return fmt.Errorf("clear cache: %w", err)
	}

	pipe := rdb.Pipeline()
	count := 0

	for _, m := range movies {
		text := m.Title + " " + m.Director
		words := extractWords(text)

		for _, word := range words {
			key := "index:" + word
			pipe.SAdd(ctx, key, m.ID)
			count++
		}
	}

	data, err := json.Marshal(movies)
	if err == nil {
		pipe.Set(ctx, "cache:movies:all", data, 0)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("exec pipeline: %w", err)
	}

	log.Printf("index built: %d movies, %d word-relations", len(movies), count)
	return nil
}

// extractWords разбивает текст на слова, приводя к нижнему регистру.
func extractWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// clearCache удаляет старый кэш ответов.
func clearCache(ctx context.Context, rdb *redis.Client) error {
	var cursor uint64
	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, "cache:search:*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	rdb.Del(ctx, "cache:movies:all")
	return nil
}
