package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"mygo/internal/domain"
	"net/http"
	"strings"
)

// MovieClient — HTTP-клиент к внешнему стенду с фильмами.
// Возвращает domain.Movie
type MovieClient struct {
	baseURL string
	http    *http.Client
}

// NewMovieClient создаёт клиент к стенду по адресу baseURL.
func NewMovieClient(baseURL string) *MovieClient {
	return &MovieClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    http.DefaultClient,
	}
}

// movieDTO — внешний JSON стенда.
type movieDTO struct {
	Num   int    `json:"id"`
	Title string `json:"title"`
	Img   string `json:"img"`
}

// GetMovie запрашивает фильм по id и возвращает его в виде domain.Movie.
func (c *MovieClient) GetMovie(ctx context.Context, id int) (domain.Movie, error) {
	u := fmt.Sprintf("%s/%d/info.0.json", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return domain.Movie{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Movie{}, fmt.Errorf("request id=%d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domain.Movie{}, domain.ErrMovieNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return domain.Movie{}, fmt.Errorf("id=%d: unexpected status %d", id, resp.StatusCode)
	}

	var dto movieDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return domain.Movie{}, fmt.Errorf("id=%d: decode: %w", id, err)
	}

	return domain.Movie{
		ID:    dto.Num,
		Title: dto.Title,
		Img:   dto.Img,
	}, nil
}
