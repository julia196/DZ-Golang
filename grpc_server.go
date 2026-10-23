package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mygo/movie"
)

type grpcServer struct {
	movie.UnimplementedMovieServiceServer
	baseURL string
}

func (s *grpcServer) LoadMovies(ctx context.Context, req *movie.LoadMoviesRequest) (*movie.LoadMoviesResponse, error) {
	from := int(req.From)
	to := int(req.To)

	if from < 1 || to < 1 || from > to {
		return nil, status.Errorf(codes.InvalidArgument, "invalid range: require 1 <= from <= to")
	}

	result := GetMovies(ctx, s.baseURL, from, to)

	resp := &movie.LoadMoviesResponse{
		Movies:     make([]*movie.Movie, 0, len(result.Movies)),
		MissingIds: make([]int32, 0, len(result.MissingIDs)),
		FailedIds:  make([]*movie.FailedID, 0, len(result.FailedIDs)), // ← новое
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

func (s *grpcServer) GetLastMovie(ctx context.Context, req *movie.GetLastMovieRequest) (*movie.Movie, error) {
	m, err := GetLastMovie(ctx, s.baseURL)
	if err != nil {
		switch {
		case err == ErrNotFound:
			return nil, status.Errorf(codes.NotFound, "no movies on stand")
		case ctx.Err() != nil:
			return nil, status.Errorf(codes.DeadlineExceeded, "movie stand timeout")
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

func startGRPCServer(baseURL string) {
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	movie.RegisterMovieServiceServer(s, &grpcServer{baseURL: baseURL})
	log.Println("gRPC server listening on :9090")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
