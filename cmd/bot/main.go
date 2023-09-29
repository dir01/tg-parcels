package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dir01/tg-parcels/bot"
	"github.com/dir01/tg-parcels/core"
	"github.com/dir01/tg-parcels/core/storage"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load()

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		panic("BOT_TOKEN is not set")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		panic("DB_PATH is not set")
	}

	parcelsAPIURL := os.Getenv("PARCELS_SERVICE_URL")
	if parcelsAPIURL == "" {
		panic("PARCELS_SERVICE_URL is not set")
	}

	pollingDurationStr := os.Getenv("POLLING_DURATION")
	if pollingDurationStr == "" {
		pollingDurationStr = "10m"
	}
	pollingDuration, err := time.ParseDuration(pollingDurationStr)

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	db := sqlx.MustOpen("sqlite3", dbPath)
	stor := storage.NewStorage(db)
	svc := core.NewService(stor, parcelsAPIURL, pollingDuration, logger)
	botStor := bot.NewStorage(db)
	b, err := bot.New(svc, botStor, token, logger)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		cancel()
	}()

	b.Start(ctx)
}
