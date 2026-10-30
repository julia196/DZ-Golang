package domain

import "errors"

// ErrMovieNotFound - запрошенный фильм отсутствует на стенде(404).
var ErrMovieNotFound = errors.New("movie not found")

// ErrInvalidRange - некорректный диапазон ID:
// from < 0, to < 0 или from > to.
var ErrInvalidRange = errors.New("invalid range: require 0 <= from <= to")
