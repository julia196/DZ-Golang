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
	grpchandler "mygo/internal/handler/grpc"
	httphandler "mygo/internal/handler/http"
	"mygo/internal/usecase"
	"mygo/movie"

	_ "mygo/docs"
	"mygo/internal/repository"
	"mygo/internal/syncer"
	"mygo/migrations"

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

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
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

	ctx := context.Background()

	pool, err := initDB(ctx, dsn)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer pool.Close()

	log.Printf("db connected, max conns: %d", pool.Stat().MaxConns())

	client := adapter.NewMovieClient(baseURL)
	repo := repository.NewMoviesRepository(pool)

	syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Minute)
	if err := syncer.Run(syncCtx, client, repo); err != nil {
		log.Printf("warning: sync failed: %v", err)
	}
	syncCancel()

	uc := usecase.NewMovieUseCase(repo)            // usecase → MovieProvider (интерфейс)
	httpHandler := httphandler.NewMovieHandler(uc) // handler/http → MovieUseCase (интерфейс)
	grpcServer := grpchandler.NewMovieServer(uc)   // handler/grpc → MovieUseCase (интерфейс)

	go startGRPCServer(grpcServer)

	mux := http.NewServeMux()
	mux.HandleFunc("/load-movies", httpHandler.LoadMovies)
	mux.HandleFunc("/last-movie", httpHandler.LastMovie)
	mux.HandleFunc("/swagger/", httpSwagger.WrapHandler)

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

// startGRPCServer поднимает gRPC-сервер на :9090.
func startGRPCServer(srv movie.MovieServiceServer) {
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	movie.RegisterMovieServiceServer(s, srv)
	log.Println("gRPC server listening on :9090")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
