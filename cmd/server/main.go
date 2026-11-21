// @title           Movie Service API
// @version         1.0
// @description     API для загрузки фильмов с внешнего стенда.
// @host            localhost:8080
// @BasePath        /

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"

	httpSwagger "github.com/swaggo/http-swagger"

	"mygo/internal/adapter"
	"mygo/internal/cache"
	grpchandler "mygo/internal/handler/grpc"
	httphandler "mygo/internal/handler/http"
	"mygo/internal/index"
	"mygo/internal/repository"
	"mygo/internal/syncer"
	"mygo/internal/usecase"
	"mygo/migrations"
	"mygo/movie"

	_ "mygo/docs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func initDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute

	var pool *pgxpool.Pool
	var pingErr error

	for i := 0; i < 5; i++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			pingErr = pool.Ping(pingCtx)
			cancel()

			if pingErr == nil {
				break
			}
			pool.Close()
		} else {
			pingErr = err
		}

		if i < 4 {
			log.Printf("waiting for db (attempt %d/60): %v", i+1, pingErr)
			time.Sleep(1 * time.Second)
		}
	}

	if pingErr != nil {
		return nil, fmt.Errorf("ping db after retries: %w", pingErr)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	if err := goose.Up(sqlDB, "."); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return pool, nil
}

func main() {
	baseURL := os.Getenv("MOVIE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://homeworksite.site"
	}

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Дефолт для локальной разработки
	}

	ctx := context.Background()

	// 1. Подключение к БД + миграции
	pool, err := initDB(ctx, dsn)
	if err != nil {
		// ВАЖНО: Мы не делаем log.Fatalf, а просто пишем предупреждение!
		log.Printf("WARNING: init db failed: %v. Starting server in degraded mode.", err)
	} else {
		defer pool.Close()
		log.Printf("db connected, max conns: %d", pool.Stat().MaxConns())
	}

	// 2. Подключение к Redis
	rdb, err := cache.NewRedisClient(ctx, redisAddr)
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	defer rdb.Close()
	log.Printf("redis connected at %s", redisAddr)

	// 3. Адаптер стенда и репозиторий
	client := adapter.NewMovieClient(baseURL)
	repo := repository.NewMoviesRepository(pool)

	// 4. Синхронизация и индекс (делаем только если БД доступна)
	if pool != nil {
		syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Minute)
		if err := syncer.Run(syncCtx, client, repo); err != nil {
			log.Printf("warning: sync failed: %v", err)
		}
		syncCancel()

		allMovies, err := repo.GetAll(ctx)
		if err != nil {
			log.Printf("failed to get all movies for index: %v", err)
		} else {
			indexCtx, indexCancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := index.BuildIndex(indexCtx, rdb, allMovies); err != nil {
				log.Printf("failed to build index: %v", err)
			}
			indexCancel()
		}
	} else {
		log.Printf("skipping sync and index: database is unavailable")
	}

	// 5. Usecase
	uc := usecase.NewMovieUseCase(repo, rdb)
	httpHandler := httphandler.NewMovieHandler(uc)
	grpcServer := grpchandler.NewMovieServer(uc)

	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":9090"
	}
	go startGRPCServer(grpcServer, grpcAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/load-movies", httpHandler.LoadMovies)
	mux.HandleFunc("/last-movie", httpHandler.LastMovie)
	mux.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	mux.HandleFunc("/search", httpHandler.Search)

	svr := &http.Server{
		Addr:         httpAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("HTTP server listening on %s", httpAddr)
	if err := svr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// startGRPCServer поднимает gRPC-сервер.
func startGRPCServer(srv movie.MovieServiceServer, addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("failed to listen gRPC on %s: %v", addr, err)
		return
	}
	s := grpc.NewServer()
	movie.RegisterMovieServiceServer(s, srv)
	log.Printf("gRPC server listening on %s", addr)
	if err := s.Serve(lis); err != nil {
		log.Printf("failed to serve gRPC: %v", err)
	}
}
