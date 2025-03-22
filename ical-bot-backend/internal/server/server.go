package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/patrick246/ical-bot/ical-bot-backend/internal/log"
)

type Job interface {
	Run(ctx context.Context) error
}

type Server struct {
	HTTPPort int
	GRPCPort int
	Logger   *slog.Logger
	Register func(*grpc.Server, *grpc.ClientConn, *runtime.ServeMux) error
	Jobs     []Job
}

func (s *Server) Run() error {
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.GRPCPort))
	if err != nil {
		return err
	}

	httpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.HTTPPort))
	if err != nil {
		return err
	}

	serveMux := runtime.NewServeMux()

	err = serveMux.HandlePath(
		http.MethodGet, "/.well-known/ready",
		func(w http.ResponseWriter, _ *http.Request, _ map[string]string) {
			w.WriteHeader(http.StatusOK)
		},
	)
	if err != nil {
		return err
	}

	httpServer := http.Server{
		Handler: serveMux,
	}

	server := grpc.NewServer()
	grpcClient, err := grpc.NewClient(fmt.Sprintf("localhost:%d", s.GRPCPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	err = s.Register(server, grpcClient, serveMux)
	if err != nil {
		return err
	}

	eg := errgroup.Group{}

	eg.Go(func() error {
		s.Logger.Info("serving gRPC", "addr", grpcListener.Addr().String())
		err := server.Serve(grpcListener)
		if err != nil {
			s.Logger.Error("error serving gRPC", log.Error(err))
		}

		return err
	})

	eg.Go(func() error {
		s.Logger.Info("serving http", "addr", httpListener.Addr().String())
		err := httpServer.Serve(httpListener)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		if err != nil {
			s.Logger.Error("error serving http", log.Error(err))
		}

		return err
	})

	for _, job := range s.Jobs {
		eg.Go(func() error {
			err := job.Run(context.Background())
			if err != nil {
				s.Logger.Error("job failure", log.Error(err))

				return err
			}

			return nil
		})
	}

	return eg.Wait()
}
