package grpchandler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mygo/internal/domain"
	"mygo/internal/usecase"
	"mygo/movie"
)

var _ movie.MovieServiceServer = (*MovieServer)(nil)

// MovieServer - gRPC-слой для работы с фильмами.
type MovieServer struct {
	movie.UnimplementedMovieServiceServer

	uc usecase.MovieUseCase
}

// NewMovieServer создаёт gRPC-слой с заданным usecase.
func NewMovieServer(uc usecase.MovieUseCase) *MovieServer {
	return &MovieServer{uc: uc}
}

// LoadMovies — rpc LoadMovies.
func (s *MovieServer) LoadMovies(ctx context.Context, req *movie.LoadMoviesRequest) (*movie.LoadMoviesResponse, error) {
	from := int(req.From)
	to := int(req.To)

	result, err := s.uc.LoadMovies(ctx, from, to)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidRange):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			return nil, status.Error(codes.Internal, "internal error")
		}
	}

	resp := &movie.LoadMoviesResponse{
		Movies:     make([]*movie.Movie, 0, len(result.Movies)),
		MissingIds: make([]int32, 0, len(result.MissingIDs)),
		FailedIds:  make([]*movie.FailedID, 0, len(result.FailedIDs)),
	}

	for _, m := range result.Movies {
		resp.Movies = append(resp.Movies, &movie.Movie{
			Num:   int32(m.ID),
			Title: m.Title,
			Img:   m.Img,
		})
	}

	for _, id := range result.MissingIDs {
		resp.MissingIds = append(resp.MissingIds, int32(id))
	}

	for _, f := range result.FailedIDs {
		resp.FailedIds = append(resp.FailedIds, &movie.FailedID{
			Id:     int32(f.ID),
			Reason: f.Reason,
		})
	}

	return resp, nil
}

// GetLastMovie — rpc GetLastMovie.
func (s *MovieServer) GetLastMovie(
	ctx context.Context,
	req *movie.GetLastMovieRequest,
) (*movie.Movie, error) {
	m, err := s.uc.GetLastMovie(ctx)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrMovieNotFound):
			return nil, status.Error(codes.NotFound, "no movies on stand")
		case ctx.Err() != nil:
			return nil, status.Error(codes.DeadlineExceeded, "movie stand timeout")
		default:
			return nil, status.Errorf(codes.Unavailable, "movie stand unavailable: %v", err)
		}
	}

	return &movie.Movie{
		Num:   int32(m.ID),
		Title: m.Title,
		Img:   m.Img,
	}, nil
}
