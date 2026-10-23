// @title           Movie Service API
// @version         1.0
// @description     API для загрузки фильмов с внешнего стенда.
// @host            localhost:8080
// @BasePath        /

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	httpSwagger "github.com/swaggo/http-swagger"

	_ "mygo/docs"
)

// Movie — фильм со стенда.
type Movie struct {
	ID    int    `json:"num"    example:"1"`
	Title string `json:"title"  example:"Barrel - Part 1"`
	Img   string `json:"img"    example:"https://imgs.xkcd.com/comics/barrel_cropped_(1).jpg"`
}

// LoadResult — результат загрузки диапазона с частичными ошибками.
type LoadResult struct {
	Movies     []Movie    `json:"movies"`
	MissingIDs []int      `json:"missing_ids,omitempty"`
	FailedIDs  []FailedID `json:"failed_ids,omitempty"`
}

type FailedID struct {
	ID     int    `json:"id"`
	Reason string `json:"reason"`
}

var ErrNotFound = errors.New("movie not found")

func fetchMovie(ctx context.Context, baseURL string, id int) (*Movie, error) {
	u := fmt.Sprintf("%s/%d/info.0.json", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request id=%d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("id=%d: unexpected status %d", id, resp.StatusCode)
	}

	var m Movie
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("id=%d: decode: %w", id, err)
	}
	return &m, nil
}

func GetMovies(ctx context.Context, baseURL string, from, to int) *LoadResult {
	var result LoadResult
	for id := from; id <= to; id++ {
		movie, err := fetchMovie(ctx, baseURL, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				result.MissingIDs = append(result.MissingIDs, id)
				continue
			}
			result.FailedIDs = append(result.FailedIDs, FailedID{ID: id, Reason: err.Error()})
			continue
		}
		result.Movies = append(result.Movies, *movie)
	}
	return &result
}

func GetLastMovie(ctx context.Context, baseURL string) (*Movie, error) {
	lo, hi := 1, 1

	for {
		_, err := fetchMovie(ctx, baseURL, hi)
		if errors.Is(err, ErrNotFound) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("id=%d: %w", hi, err)
		}
		lo = hi
		hi *= 2
		if hi > 1<<30 {
			return nil, fmt.Errorf("last movie not found: probe limit exceeded")
		}
	}

	for lo+1 < hi {
		mid := (lo + hi) / 2
		_, err := fetchMovie(ctx, baseURL, mid)
		switch {
		case errors.Is(err, ErrNotFound):
			hi = mid
		case err != nil:
			return nil, fmt.Errorf("id=%d: %w", mid, err)
		default:
			lo = mid
		}
	}
	return fetchMovie(ctx, baseURL, lo)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// LoadMovies godoc
// @Summary      Загрузка фильмов
// @Description  Загружает фильмы со стенда в диапазоне [from; to]. Частичные ошибки возвращаются в полях missing_ids и failed_ids.
// @Tags         movies
// @Accept       json
// @Produce      json
// @Param        from query int true  "Начальный ID (>= 1)"
// @Param        to   query int true  "Конечный ID (>= from)"
// @Success      200  {object} LoadResult
// @Failure      400  {object} map[string]string "Неверные параметры from/to"
// @Failure      502  {object} map[string]string "Стенд недоступен"
// @Failure      504  {object} map[string]string "Таймаут стенда"
// @Router       /load-movies [get]
func NewLoadMoviesHandler(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		fromStr := q.Get("from")
		toStr := q.Get("to")
		if fromStr == "" || toStr == "" {
			writeJSONError(w, http.StatusBadRequest, "parameters 'from' and 'to' are required")
			return
		}

		from, err := strconv.Atoi(fromStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "'from' must be integer")
			return
		}
		to, err := strconv.Atoi(toStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "'to' must be integer")
			return
		}
		if from < 1 || to < 1 || from > to {
			writeJSONError(w, http.StatusBadRequest, "invalid range: require 1 <= from <= to")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		movies := GetMovies(ctx, baseURL, from, to)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(movies)
	}
}

// LastMovie godoc
// @Summary      Последний фильм
// @Description  Находит последний доступный фильм на стенде (бинарный поиск по id).
// @Tags         movies
// @Accept       json
// @Produce      json
// @Success      200  {object} Movie
// @Failure      404  {object} map[string]string "Фильмов на стенде нет"
// @Failure      502  {object} map[string]string "Стенд недоступен"
// @Failure      504  {object} map[string]string "Таймаут стенда"
// @Router       /last-movie [get]
func NewLastMovieHandler(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		movie, err := GetLastMovie(ctx, baseURL)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "no movies on stand")
			case errors.Is(err, context.DeadlineExceeded):
				writeJSONError(w, http.StatusGatewayTimeout, "movie stand timeout")
			case errors.Is(err, context.Canceled):
				return
			default:
				writeJSONError(w, http.StatusBadGateway, "movie stand unavailable")
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(movie)
	}
}

func main() {
	baseURL := os.Getenv("MOVIE_BASE_URL")
	if baseURL == "" {
		baseURL = "https://homeworksite.site"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	go startGRPCServer(baseURL)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/load-movies", NewLoadMoviesHandler(baseURL))
	mux.HandleFunc("/last-movie", NewLastMovieHandler(baseURL))
	mux.HandleFunc("/swagger/", httpSwagger.WrapHandler)

	svr := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("HTTP server listening on %s", addr)
	if err := svr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
