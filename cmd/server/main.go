// @title           Movie Service API
// @version         1.0
// @description     API для загрузки фильмов с внешнего стенда.
// @host            localhost:8080
// @BasePath        /

package main

import (
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

	client := adapter.NewMovieClient(baseURL)      // adapter → domain
	uc := usecase.NewMovieUseCase(client)          // usecase → MovieProvider (интерфейс)
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
