package handlerhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"mygo/internal/domain"
	"mygo/internal/usecase"
)

// MovieHandler- HTTP-слой для работы с фильмами.
// Принимает HTTP-запросы, выполняет транспортную валидацию
type MovieHandler struct {
	uc usecase.MovieUseCase
}

// NewMovieHandler создаёт HTTP-слой с заданным usecase.
func NewMovieHandler(uc usecase.MovieUseCase) *MovieHandler {
	return &MovieHandler{uc: uc}
}

// writeJSONError- универсальный ответ с ошибкой в JSON.
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
// @Success      200  {object} usecase.LoadResult
// @Failure      400  {object} map[string]string "Неверные параметры from/to"
// @Failure      502  {object} map[string]string "Стенд недоступен"
// @Failure      504  {object} map[string]string "Таймаут стенда"
// @Router       /load-movies [get]
func (h *MovieHandler) LoadMovies(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.uc.LoadMovies(ctx, from, to)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidRange):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// LastMovie godoc
// @Summary      Последний фильм
// @Description  Находит последний доступный фильм на стенде (бинарный поиск по id).
// @Tags         movies
// @Accept       json
// @Produce      json
// @Success      200  {object} domain.Movie
// @Failure      404  {object} map[string]string "Фильмов на стенде нет"
// @Failure      502  {object} map[string]string "Стенд недоступен"
// @Failure      504  {object} map[string]string "Таймаут стенда"
// @Router       /last-movie [get]
func (h *MovieHandler) LastMovie(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	movie, err := h.uc.GetLastMovie(ctx)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrMovieNotFound):
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
