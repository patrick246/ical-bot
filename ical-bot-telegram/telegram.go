package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	icalbot "github.com/patrick246/ical-bot/ical-bot-backend/pkg/api/pb/ical-bot-backend/v1"
)

// Send any text message to the bot after the bot has been started

const endpoint = "localhost:8081"
const error_message = "ERROR: Telegram-Bot:"
const info_message = "INFO: Telegram-Bot:"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	botOpts := []bot.Option{
		bot.WithDefaultHandler(defaultHandler),
	}

	b, err := bot.New(os.Getenv("EXAMPLE_TELEGRAM_BOT_TOKEN"), botOpts...)
	if nil != err {
		// panics for the sake of simplicity.
		// you should handle this error properly in your code.
		panic(err)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, "/hello", bot.MatchTypeExact, helloHandler)

	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	clientConnection, err := grpc.NewClient("", grpcOpts...)

	if err != nil {
		println("%s grpc client connection could not be established: %s", error_message, err.Error())
		defer clientConnection.Close()
		os.Exit(1)
	}
	telegramClientBot := icalbot.NewIcalBotServiceClient(clientConnection)

	var protoReq icalbot.ListChannelsRequest

	channelListResponse, err := telegramClientBot.ListChannels(context.Background(), &protoReq)

	if err != nil {
		println("could not fetch channel list!")
		os.Exit(1)
	}
	channel_list := channelListResponse.Channels

	stream, err := telegramClientBot.StreamEventNotifications(context.Background())
	if err != nil {
		println("%s telegram event stream notification could not be received: %s", error_message, err.Error())
		defer clientConnection.Close()
		os.Exit(1)
	}
	// grpc message waiter
	go func() {
		for {
			notification, err := stream.Recv()
			if err != nil {
				println("%s notification could not be received: %s", error_message, err.Error())
				return
			}
			ack := &icalbot.EventNotificationAcknowledge{}
			ack.Id = notification.Id

			for _, channel := range channel_list {
				telegramChannel := channel.GetTelegram()
				if telegramChannel == nil {
					continue
				}
				_, err = b.SendMessage(ctx,
					&bot.SendMessageParams{
						ChatID:    telegramChannel.Id,
						Text:      notification.Id,
						ParseMode: models.ParseModeMarkdown,
					})
				if err != nil {
					stream.Send(ack)
				}
			}

		}
	}()

	b.Start(ctx)
}

func helloHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "Hello, *" + bot.EscapeMarkdown(update.Message.From.FirstName) + "*",
		ParseMode: models.ParseModeMarkdown,
	})
}

func defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Say /hello",
	})
}
