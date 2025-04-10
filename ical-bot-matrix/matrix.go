package main

import (
  // "context"
	// "errors"
	"log/slog"
  // "github.com/caarlos0/env/v11"
	// "maunium.net/go/mautrix"
	// "maunium.net/go/mautrix/crypto/cryptohelper"
	// "maunium.net/go/mautrix/event"
	// "maunium.net/go/mautrix/id"
  // icalbot "github.com/patrick246/ical-bot/ical-bot-backend/pkg/api/pb/ical-bot-backend/v1"
)

var logger slog.Logger

func main() {
  logger = *slog.Default()

  logger.Info("Hello world, I am alive!", "lucky random number", 3)
}
