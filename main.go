package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

type movie struct {
	ID       int
	Title    string
	Director string
}

func getMovie(ctx context.Context, client *http.Client, id int) (movie, error) {
	url := fmt.Sprintf("https://homeworksite.site/%d/info.0.json", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return movie{}, fmt.Errorf("Ошибка создания запроса для фильма с id = %d", id)
	}
	resp, err := client.Do(req)
	if err != nil {
		return movie{}, fmt.Errorf("Ошибка выполнения запроса для фильма с id=%d: %w", id, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return movie{}, fmt.Errorf("id=%d: Не найдено (404)", id)
	case http.StatusTooManyRequests:
		return movie{}, fmt.Errorf("id=%d: превышен лимит запросов (429)", id)
	case http.StatusInternalServerError:
		return movie{}, fmt.Errorf("id=%d: ошибка сервера (500)", id)
	default:
		return movie{}, fmt.Errorf("id=%d: неизвестная ошибка %s", id, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return movie{}, fmt.Errorf("id=%d: ошибка при чтении: %w", id, err)
	}

	var mov movie
	err = json.Unmarshal(body, &mov)
	if err != nil {
		return movie{}, fmt.Errorf("id=%d: ошибка при чтении json: %w", id, err)
	}
	return mov, nil
}

func main() {
	from := flag.Int("from", 5, "стартовый индекс")
	to := flag.Int("to", 0, "конечный индекс")
	workers := flag.Int("workers", 10, "количество воркеров")
	timeout := flag.Int("timeout", 5, "HTTP timeout")
	flag.Parse()

	if *from > *to {
		fmt.Fprintln(os.Stderr, "стартовый индекс не может быть больше конечного индекса: from > to")
		os.Exit(1)
	}
	if *workers <= 0 {
		fmt.Fprintln(os.Stderr, "количество воркеров должно быть больше нуля")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client := &http.Client{Timeout: time.Duration(*timeout) * time.Second}

	jobs := make(chan int, *workers)
	results := make(chan movie, *workers)
	var wg sync.WaitGroup

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
					mov, err := getMovie(ctx, client, id)
					if err != nil {
						fmt.Printf("id=%d: %v\n", id, err)
						continue
					}
					results <- mov
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for id := *from; id <= *to; id++ {
			select {
			case <-ctx.Done():
				return
			case jobs <- id:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for m := range results {
		fmt.Printf("%d — %s — %s\n", m.ID, m.Title, m.Director)
	}

}
