package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dir01/tg-parcels/core"
	"github.com/hori-ryota/zaperr"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

const TRACK_CMD_HELP = "/track <tracking number> [[name]] - start receiving updates about a parcel"
const INFO_CMD_HELP = "/info <tracking number> - get info about a parcel"
const STOP_CMD_HELP = "/stop <tracking number> - stop receiving updates about a parcel"
const LIST_CMD_HELP = "/list - list all tracked parcels"

var HELP = strings.Join([]string{`
Hello, I'm a bot that can help you to track your parcels.

**Commands:**`,
	TRACK_CMD_HELP,
	INFO_CMD_HELP,
	STOP_CMD_HELP,
	LIST_CMD_HELP,
	"/help - show this message",
}, "\n")

func New(service core.Service, storage Storage, token string, logger *zap.Logger) (*Bot, error) {
	b, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, err
	}
	return &Bot{
		service: service,
		storage: storage,
		bot:     b,
		logger:  logger,
	}, nil
}

type Storage interface {
	UserChatID(ctx context.Context, userID int64) (int64, error)
	SaveUserChatID(ctx context.Context, userID int64, chatID int64) error
}

type Bot struct {
	service core.Service
	bot     *tele.Bot
	logger  *zap.Logger
	storage Storage
}

func (b *Bot) Start(ctx context.Context) {
	handlers := b.bot.Group()
	handlers.Use(b.saveChatIDMiddleware)
	handlers.Handle("/start", b.handleHelpCmd)
	handlers.Handle("/help", b.handleHelpCmd)
	handlers.Handle("/track", b.handleTrackCmd)

	go func() {
		for {
			select {
			case <-ctx.Done():
				b.logger.Debug("context cancelled, stopping updates worker")
				return
			case update := <-b.service.Updates():
				b.notifyUserOfTrackingUpdate(update)
			}
		}
	}()

	b.service.Start(ctx)
	b.logger.Debug("service started")
	go func() {
		b.logger.Debug("starting bot termination watcher")
		<-ctx.Done()
		b.logger.Debug("context cancelled, stopping bot")
		b.bot.Stop()
		b.bot.Close()
		b.logger.Debug("bot stopped")
	}()
	b.logger.Debug("starting bot")
	b.bot.Start()
}

func (b *Bot) notifyUserOfTrackingUpdate(update core.TrackingUpdate) {
	fields := []zap.Field{
		zap.Any("update", update),
	}
	b.logger.Debug("handling tracking update", fields...)

	chatID, err := b.storage.UserChatID(context.Background(), update.UserID)
	if err != nil {
		b.logger.Error("failed to get chat id", fields...)
		return
	}
	if chatID == 0 {
		b.logger.Debug("no chat id found for user", fields...)
		return
	}

	bytes, err := json.MarshalIndent(update, "", "  ")
	if err != nil {
		fields := append(fields, zaperr.ToField(err))
		b.logger.Error("failed to marshal update", fields...)
		return
	}
	msg := fmt.Sprintf("Parcel %s (%s) has been updated:\n%s", update.TrackingNumber, update.DisplayName, string(bytes))

	if _, err := b.bot.Send(tele.ChatID(chatID), msg); err != nil {
		zapFields := append(fields, zap.Int64("chat_id", chatID))
		b.logger.Error("failed to send message", zapFields...)
	}
}

func (b *Bot) handleTrackCmd(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Send(TRACK_CMD_HELP, "Markdown")
	}

	userID := c.Message().Sender.ID
	trackingNumber := args[0]
	displayName := strings.Join(args[1:], " ")

	if err := b.service.Track(context.Background(), userID, trackingNumber, displayName); err == nil {
		return c.Send("Started tracking " + trackingNumber)
	} else {
		b.logger.Error("failed to track parcel", zaperr.ToField(err))
	}
	return nil
}

func (b *Bot) handleHelpCmd(c tele.Context) error {
	return c.Send(HELP, "Markdown")
}

func (b *Bot) saveChatIDMiddleware(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		chatID := c.Message().Chat.ID
		userID := c.Message().Sender.ID

		zapFields := []zap.Field{
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
		}
		b.logger.Debug("saving chat id", zapFields...)

		if err := b.storage.SaveUserChatID(context.Background(), userID, chatID); err != nil {
			zapFields := append(zapFields, zaperr.ToField(err))
			b.logger.Error("failed to save chat id", zapFields...)
		}
		return next(c)
	}
}
