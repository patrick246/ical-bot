package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	// This controls the maxprocs environment variable in container runtimes.
	// see https://martin.baillie.id/wrote/gotchas-in-the-go-network-packages-defaults/#bonus-gomaxprocs-containers-and-the-cfs
	"go.uber.org/automaxprocs/maxprocs"
	"google.golang.org/grpc"

	"github.com/patrick246/ical-bot/ical-bot-backend/internal/config"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/database"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/log"
	pb "github.com/patrick246/ical-bot/ical-bot-backend/internal/pkg/api/pb/ical-bot-backend/v1"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/server"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/service"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/service/calendar"
	"github.com/patrick246/ical-bot/ical-bot-backend/internal/service/events"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	logger := log.New(
		log.WithLevel(cfg.LogLevel),
		log.WithSource(),
	)

	_, err = maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		logger.DebugContext(ctx, fmt.Sprintf(s, i...))
	}))
	if err != nil {
		return fmt.Errorf("setting max procs: %w", err)
	}

	db, err := database.Connect(cfg.Database)
	if err != nil {
		return err
	}

	err = database.Migrate(context.Background(), db, logger)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	calendarRepo := calendar.NewCalendarRepository(db)
	eventRepo := events.NewRepository(db)
	svc := service.NewICalBackend(calendarRepo)

	srv := server.Server{
		HTTPPort: cfg.HTTPPort,
		GRPCPort: cfg.GRPCPort,
		Logger:   logger,
		Register: func(server *grpc.Server, conn *grpc.ClientConn, mux *runtime.ServeMux) error {
			pb.RegisterIcalBotServiceServer(server, svc)
			return pb.RegisterIcalBotServiceHandler(context.Background(), mux, conn)
		},
		Jobs: []server.Job{
			events.NewIcalImport(eventRepo, calendarRepo, httpClient, logger),
		},
	}

	err = srv.Run()
	if err != nil {
		return err
	}

	return nil
}
