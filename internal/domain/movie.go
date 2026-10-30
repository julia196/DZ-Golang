package domain

// Movie - модель фильма со стенда.
type Movie struct {
	ID    int    `json:"num"`
	Title string `json:"title"`
	Img   string `json:"img"`
}
