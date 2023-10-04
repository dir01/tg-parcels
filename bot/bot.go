package bot

import (
	"context"
	"errors"
	"fmt"
	"github.com/dir01/parcels/parcels_api"
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
Hello! I'm a bot that can help you to track your parcels.

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
	handlers.Handle("/info", b.handleInfoCmd)
	handlers.Handle("/list", b.handleListCmd)
	handlers.Handle("/delete", b.handleDeleteCmd)

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

	if errors.Is(update.TrackingError, core.ErrNoTrackingInfo) {
		msg := "Tracking info not found at the moment, but we will keep trying to find it and will update of any changes"
		if _, err := b.bot.Send(tele.ChatID(chatID), msg); err != nil {
			zapFields := append(fields, zap.Int64("chat_id", chatID))
			b.logger.Error("failed to send message", zapFields...)
		}
		return
	}

	if update.TrackingError != nil {
		msg := "Failed to get tracking info"
		if _, err := b.bot.Send(tele.ChatID(chatID), msg); err != nil {
			zapFields := append(fields, zap.Int64("chat_id", chatID))
			b.logger.Error("failed to send message", zapFields...)
		}
		return
	}

	title := fmt.Sprintf("<code>%s</code>", update.TrackingNumber)
	if update.DisplayName != "" {
		title = fmt.Sprintf("%s - %s", title, update.DisplayName)
	}

	var lines []string
	lines = append(lines, title)
	for _, e := range update.NewTrackingEvents {
		l := fmt.Sprintf("%s - %s", e.Time, e.Description)
		lines = append(lines, l)
	}

	msg := strings.Join(lines, "\n")

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

func (b *Bot) handleInfoCmd(c tele.Context) error {
	userID := c.Message().Sender.ID
	args := c.Args()
	if len(args) == 0 {
		return c.Send(INFO_CMD_HELP, tele.ModeMarkdown)
	}

	trackingNumber := args[0]
	tracking, err := b.service.GetTracking(context.Background(), userID, trackingNumber)
	if err != nil {
		return c.Send("Failed to get tracking info")
	}

	title := fmt.Sprintf("<code>%s</code>", tracking.TrackingNumber)
	if tracking.DisplayName != "" {
		title = fmt.Sprintf("%s - %s", title, tracking.DisplayName)
	}

	lines := []string{title}
	events := b.collectAllEvents(tracking)
	for _, e := range events {
		l := fmt.Sprintf("%s - %s", e.Time, e.Description)
		lines = append(lines, l)
	}

	return c.Send(strings.Join(lines, "\n"), tele.ModeHTML)
}

func (b *Bot) handleListCmd(c tele.Context) error {
	userID := c.Message().Sender.ID
	trackings, err := b.service.ListTrackings(context.Background(), userID)
	if err != nil {
		return c.Send("Failed to list trackings")
	}

	var lines []string
	for _, tracking := range trackings {
		l := fmt.Sprintf("<code>%s</code>", tracking.TrackingNumber)
		if tracking.DisplayName != "" {
			l = fmt.Sprintf("%s - %s", l, tracking.DisplayName)
		}
		lines = append(lines, l)

		events := b.collectAllEvents(tracking)
		if len(events) > 0 {
			e := events[len(events)-1]
			l := fmt.Sprintf("%s - %s", e.Time, e.Description)
			lines = append(lines, l)
		}

		lines = append(lines, "")
	}

	return c.Send(strings.Join(lines, "\n"), tele.ModeHTML)
}

func (b *Bot) handleDeleteCmd(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Send("Please specify tracking number")
	}

	userID := c.Message().Sender.ID
	trackingNumber := args[0]

	if err := b.service.DeleteTracking(context.Background(), userID, trackingNumber); err == nil {
		return c.Send("Stopped tracking " + trackingNumber)
	} else {
		b.logger.Error("failed to stop tracking", zaperr.ToField(err))
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

func (b *Bot) collectAllEvents(tracking *core.Tracking) []parcels_api.TrackingEvent {
	var events []parcels_api.TrackingEvent
	for _, info := range tracking.TrackingInfos {
		events = append(events, info.Events...)
	}
	return events
}
