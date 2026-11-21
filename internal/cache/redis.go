package cache

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient создает клиент и проверяет, что Redis доступен.
func NewRedisClient(ctx context.Context, addr string) (*redis.Client, error) {
	var rdb *redis.Client
	var err error

	for i := 0; i < 60; i++ {
		rdb = redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     "",
			DB:           0,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
		})

		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = rdb.Ping(pingCtx).Err()
		cancel()

		if err == nil {
			return rdb, nil
		}

		rdb.Close()
		if i < 59 {
			log.Printf("waiting for redis (attempt %d/60): %v", i+1, err)
			time.Sleep(1 * time.Second)
		}
	}

	return nil, fmt.Errorf("ping redis after retries: %w", err)
}
