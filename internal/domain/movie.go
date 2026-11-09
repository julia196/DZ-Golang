package domain

// Movie - модель фильма со стенда.
type Movie struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Year        int      `json:"year"`
	Director    string   `json:"director"`
	Description string   `json:"description"`
	Actors      []string `json:"actors"`
	Genres      []string `json:"genres"`
}
