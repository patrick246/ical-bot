package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/caarlos0/env/v11"
	//"github.com/rs/zerolog"
	//slogzerolog "github.com/samber/slog-zerolog/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	_ "github.com/patrick246/ical-bot/ical-bot-backend/pkg/api/pb/ical-bot-backend/v1"
)

type matrixConfig struct {
	HomeserverUrl string `env:"MATRIX_HOMESERVER_URL,notEmpty"`
	Username      string `env:"MATRIX_USERNAME,notEmpty"`
	Password      string `env:"MATRIX_PASSWORD,notEmpty"`
	LogLevel      string `env:"MATRIX_LOG_LEVEL" envDefault:"INFO"`
}

func sendMessage(ctx context.Context, client *mautrix.Client, roomID id.RoomID, message string) error {
	resMes, err := client.SendText(ctx, roomID, message)
	if err != nil {
		slog.WarnContext(ctx, "could not send Matrix message", slog.Any("error", err), slog.String("room_id", roomID.String()), slog.String("message", message))
		return err
	}

	slog.DebugContext(ctx, "successfully sent Matrix message", slog.Any("result", resMes), slog.String("room_id", roomID.String()), slog.String("message", message))
	return nil
}

// TODO: Implement me
func loginIcal() {}

// TODO: Implement me
func fetchIcal() {}

func main() {
	var cfg matrixConfig
	if err := env.Parse(&cfg); err != nil {
		slog.Error("could not parse necessary environment variables for config", slog.Any("error", err))
		os.Exit(1)
	}

	var programLevel = new(slog.LevelVar)
	if err := programLevel.UnmarshalText(([]byte)(cfg.LogLevel)); err != nil {
		slog.Error("could not set log level", slog.Any("error", err))
		os.Exit(1)
	}

	var logger *slog.Logger
	logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel}))
	logger.Info("starting program with log level", slog.Any("level", programLevel))

	client, err := mautrix.NewClient(cfg.HomeserverUrl, "", "")
	if err != nil {
		logger.Error("could not reach homeserver", slog.Any("error", err))
		os.Exit(1)
	}
	// I'm not sure if this is the correct way to set the store. The docs are a bit ambiguous
	client.StateStore = mautrix.NewMemoryStateStore()
	resLogin, err := client.Login(context.Background(), &mautrix.ReqLogin{
		Type:     mautrix.AuthTypePassword,
		Password: cfg.Password,
		Identifier: mautrix.UserIdentifier{

			Type: mautrix.IdentifierTypeUser,
			User: cfg.Username,
		},
		StoreCredentials:   true,
		StoreHomeserverURL: true,
	})
	if err != nil {
		logger.Error("could not login to homeserver", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Debug("successfully logged into homeserver", slog.Any("result", resLogin))

	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {
		if isEncrypted, err := client.StateStore.IsEncrypted(ctx, evt.RoomID); !isEncrypted || err != nil {
			logger.InfoContext(ctx, "room not encrypted yet", slog.Any("error", err), slog.String("room_id", evt.RoomID.String()))
			// if err = client.StateStore.SetEncryptionEvent(ctx, evt.RoomID, &event.EncryptionEventContent{
			// 	// Must be according to docs(https://github.com/mautrix/go/blob/826089e020fb838951df813138d89ab47b07b6b1/event/encryption.go#L19)
			// 	Algorithm:              id.AlgorithmMegolmV1,
			//   // Recommended defaults
			// 	RotationPeriodMillis:   7 * 24 * 60 * 60 *1000,
			// 	RotationPeriodMessages: 100,
			// }); err != nil {
			//   logger.Warn("could not upgrade room to encrypted", "roomID", evt.RoomID, "error", err)
			// } else {
			//   logger.Info("successfully upgraded room to encrypted", "roomID", evt.RoomID)
			// }
		}

		logger.DebugContext(ctx, "received a new message", slog.String("sender", evt.Sender.String()), slog.String("body", evt.Content.AsMessage().Body))
		if evt.Sender != client.UserID {
			resSend, err := client.SendText(ctx, evt.RoomID, "Yes, I heard you. Your message was "+evt.Content.AsMessage().Body)
			if err != nil {
				logger.WarnContext(ctx, "could not send message", slog.Any("error", err))
			}
			logger.DebugContext(ctx, "sent message", slog.Any("result", resSend))
		}
	})
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		// StateKey is user's ID for member change events
		if evt.GetStateKey() == client.UserID.String() {
			membership := evt.Content.AsMember().Membership

			switch membership {
			case event.MembershipInvite:
				resJoin, err := client.JoinRoom(ctx, evt.RoomID.String(), nil)
				if err != nil {
					logger.Error("failed when attempting to join invited room", slog.Any("error", err), slog.String("room_id", evt.RoomID.String()), slog.Any("event", evt))
				}

				logger.InfoContext(ctx, "successfully joined invited room", slog.Any("result", resJoin))
				sendMessage(ctx, client, evt.RoomID, "Hello, I'm the Ical Test Bot! It's nice to meet you :3")
			default:
				logger.WarnContext(ctx, "received unimplemented membership change event", slog.Any("type", membership), slog.Any("event", evt))
			}
		}
	})

	cryptoStore := crypto.NewMemoryStore(nil)
	// TODO: The docs are not entirely clear if the pickleKey is a secret or just a key
	cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte("awawawaaawa"), cryptoStore)
	if err != nil {
		logger.Error("failed to setup cryptoHelper", slog.Any("error", err))
		os.Exit(1)
	}
	// TODO: Maybe setup cryptoHelper.Machine().log via loggerzerolog
	if err := cryptoHelper.Init(context.Background()); err != nil {
		logger.Error("could not initialize cryptoHelper", slog.Any("error", err))
		os.Exit(1)
	}
	// De- and Encryption
	client.Crypto = cryptoHelper
	logger.Debug("successfully initialized cryptoHelper")

	loginIcal()

	var syncWait sync.WaitGroup

	matrixSyncCtx, closeMatrixSyncContext := context.WithCancel(context.Background())
	syncWait.Add(1)
	go func() {
		terminated := false
		defer syncWait.Done()

		for !terminated {
			if err := client.SyncWithContext(matrixSyncCtx); err != nil {
				if errors.Is(err, context.Canceled) {
					logger.InfoContext(matrixSyncCtx, "received cancel event, shutting down matrix backend", slog.Any("error", err))
					terminated = true
				} else {
					logger.ErrorContext(matrixSyncCtx, "received unknown sync error", slog.Any("error_type", err))
				}
			}
		}
	}()

	icalSyncCtx, closeIcalSyncContext := context.WithCancel(context.Background())
	syncWait.Add(1)
	go func() {
		terminated := false
		defer syncWait.Done()

		logger.DebugContext(icalSyncCtx, "hiiii, I'm the ical backend and I'm non-existent for meow")

		for !terminated {
			select {
			case <-icalSyncCtx.Done():
				logger.InfoContext(icalSyncCtx, "shutting down ical backend")
				terminated = true
			}

			fetchIcal()
		}
	}()

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	sig := <-osSignals
	logger.Info("received os signal for termination, initiating shutdown procedure", slog.Any("signal", sig))
	closeMatrixSyncContext()
	closeIcalSyncContext()
	syncWait.Wait()

	if err := cryptoHelper.Close(); err != nil {
		logger.Error("error while shuting down cryptoHelper", slog.Any("error", err))
	}
	logger.Info("finished shutdown, terminating program")
}
