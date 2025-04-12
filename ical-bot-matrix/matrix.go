package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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

var logger *slog.Logger
var cfg matrixConfig
var programLevel = new(slog.LevelVar)

func logLevelFromString(toParse string) (slog.Level, error) {
	switch strings.ToUpper(toParse) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return slog.Level(-999), errors.New("Invalid log level provided")
	}
}

func sendMessage(client *mautrix.Client, ctx context.Context, roomID id.RoomID, message string) error {
	resMes, err := client.SendText(ctx, roomID, message)
	if err != nil {
		slog.Warn("Could not send Matrix message", "error", err, "room", roomID, "message", message)
		return err
	}

	slog.Debug("Successfully sent Matrix message", "result", resMes, "roomID", roomID, "message", message)
	return nil
}

// TODO: Implement me
func loginIcal() {}

// TODO: Implement me

func fetchIcal() {}

func main() {
	if err := env.Parse(&cfg); err != nil {
		slog.Error("Could not parse necessary environment variables for config", "error", err)
		os.Exit(1)
	}

	programLevel, err := logLevelFromString(cfg.LogLevel)
	if err != nil {
		slog.Error("Could not set log level", "error", err)
		os.Exit(1)
	}

	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel}))
	slog.SetDefault(logger)
	slog.Info("Starting program with log level", "level", programLevel)

	client, err := mautrix.NewClient(cfg.HomeserverUrl, "", "")
	if err != nil {
		slog.Error("Could not reach Homeserver", "error", err)
		os.Exit(1)
	}
	// I'm not sure if this is the correct way to set the store. The docs are a bit ambiguous
	client.StateStore = mautrix.NewMemoryStateStore()
	resLogin, err := client.Login(context.TODO(), &mautrix.ReqLogin{
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
		slog.Error("Could not login to homeserver", "error", err)
		os.Exit(1)
	}
	slog.Debug("Successfully logged into homeserver", "result", resLogin)

	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {
		if isEncrypted, err := client.StateStore.IsEncrypted(ctx, evt.RoomID); !isEncrypted || err != nil {
			slog.Info("Room not encrypted yet.", "error", err, "roomID", evt.RoomID)
      // if err = client.StateStore.SetEncryptionEvent(ctx, evt.RoomID, &event.EncryptionEventContent{
      // 	// Must be according to docs(https://github.com/mautrix/go/blob/826089e020fb838951df813138d89ab47b07b6b1/event/encryption.go#L19)
      // 	Algorithm:              id.AlgorithmMegolmV1,
      //   // Recommended defaults
      // 	RotationPeriodMillis:   7 * 24 * 60 * 60 *1000,
      // 	RotationPeriodMessages: 100,
      // }); err != nil {
      //   slog.Warn("Could not upgrade room to encrypted", "roomID", evt.RoomID, "error", err)
      // } else {
      //   slog.Info("Successfully upgraded room to encrypted", "roomID", evt.RoomID)
      // }
		}

		slog.Debug("Received a new message", "sender", evt.Sender.String(), "body", evt.Content.AsMessage().Body)
		if evt.Sender != client.UserID {
			resSend, err := client.SendText(ctx, evt.RoomID, "Yes, I heard you. Your message was "+evt.Content.AsMessage().Body)
			if err != nil {
				slog.Warn("Could not send message", "error", err)
			}
			slog.Debug("Sent message", "result", resSend)
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
					slog.Error("Failed when attempting to join invited room", "roomID", evt.RoomID, "event", evt)
				}

				slog.Info("Successfully joined invited room", "result", resJoin)
				sendMessage(client, ctx, evt.RoomID, "Hello, I'm the Ical Test Bot! It's nice to meet you :3")
			default:
				slog.Warn("Received unimplemented Membership change event", "type", membership, "event", evt)
			}
		}
	})

  cryptoStore := crypto.NewMemoryStore(nil)
	// TODO: The docs are not entirely clear if the pickleKey is a secret or just a key
	cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte("awawawaaawa"), cryptoStore)
	if err != nil {
		slog.Error("Failed to setup CryptoHelper", "error", err)
		os.Exit(1)
	}
	// TODO: Maybe setup cryptoHelper.Machine().log via slogzerolog
	if err := cryptoHelper.Init(context.TODO()); err != nil {
		slog.Error("Could not initialize cryptoHelper", "error", err)
		os.Exit(1)
	}
	// De- and Encryption
	client.Crypto = cryptoHelper
	slog.Debug("Successfully initialized cryptohelper")

	loginIcal()

	var syncWait sync.WaitGroup
	matrixSyncCtx, closeMatrixSyncContext := context.WithCancel(context.Background())

	go func() {
		// What does this do internally?
		if err := client.SyncWithContext(matrixSyncCtx); err != nil {
			if errors.Is(err, context.Canceled) {
				slog.Error("Received cancel event. Shutting down...")
			} else {
				slog.Error("Received unknown Sync error", "error_type", err)
			}
		}
	}()

	syncWait.Add(1)
	go func() {
		defer syncWait.Done()
		slog.Debug("Hiiii, I'm the Ical backend and I'm non-existent for meow")
		fetchIcal()
	}()

	syncWait.Add(1)
	go func() {
		defer syncWait.Done()
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, syscall.SIGTERM)
		signal.Notify(osSignals, syscall.SIGINT)
		signal.Notify(osSignals, syscall.SIGQUIT)

		sig := <-osSignals
		slog.Info("Received OS Signal for termination. Initiating shutdown procedure.", "signal", sig)
	}()

	syncWait.Wait()
	slog.Info("Starting shutdown procedure.")
	// Implicitly terminates the event loop
	closeMatrixSyncContext()
	if err := cryptoHelper.Close(); err != nil {
		slog.Error("Error while shuting down CryptoHelper", "error", err)
	}
	slog.Info("Finished shutdown. Terminating program.")
}
