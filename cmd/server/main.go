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
	"github.com/redis/go-redis/v9"
)

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
		redisAddr = "localhost:6379"
	}

	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	rdb, err := cache.NewRedisClient(ctx, redisAddr)
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	defer rdb.Close()

	client := adapter.NewMovieClient(baseURL)
	repo := repository.NewMoviesRepository(pool)
	uc := usecase.NewMovieUseCase(repo, rdb)
	httpHandler := httphandler.NewMovieHandler(uc)
	grpcServer := grpchandler.NewMovieServer(uc)

	go func() {
		for {
			err := doInitAndSync(ctx, pool, client, repo, rdb)
			if err == nil {
				log.Println("background init and sync successful")
				break // Успех! Выходим из цикла
			}
			log.Printf("background init failed: %v, retrying in 5s", err)
			time.Sleep(5 * time.Second)
		}
	}()

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

// doInitAndSync выполняет пинг БД, миграции, синхронизацию и построение индекса
func doInitAndSync(ctx context.Context, pool *pgxpool.Pool, client *adapter.MovieClient, repo repository.MoviesRepository, rdb *redis.Client) error {
	// 1. Проверка подключения
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// 2. Миграции
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// 3. Синхронизация
	syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Minute)
	if err := syncer.Run(syncCtx, client, repo); err != nil {
		syncCancel()
		return fmt.Errorf("sync: %w", err)
	}
	syncCancel()

	// 4. Построение индекса
	allMovies, err := repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("get all movies for index: %w", err)
	}
	indexCtx, indexCancel := context.WithTimeout(ctx, 5*time.Minute)
	if err := index.BuildIndex(indexCtx, rdb, allMovies); err != nil {
		indexCancel()
		return fmt.Errorf("build index: %w", err)
	}
	indexCancel()

	return nil
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
