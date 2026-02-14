package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/nacorid/x402-feed/internal/consumer"
	db "github.com/nacorid/x402-feed/internal/database"
	srv "github.com/nacorid/x402-feed/internal/server"

	"github.com/avast/retry-go/v4"
	"github.com/joho/godotenv"
)

const (
	defaultJetstreamAddr = "wss://jetstream2.us-east.bsky.network/subscribe"
	serverPort           = 11011 // this must be the port value used. See https://docs.bsky.app/docs/starter-templates/custom-feeds#deploying-your-feed
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error loading .env file")
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	feedHost := os.Getenv("FEED_HOST_NAME")
	if feedHost == "" {
		return fmt.Errorf("FEED_HOST_NAME not set")
	}
	feedName := os.Getenv("FEED_NAME")
	if feedName == "" {
		return fmt.Errorf("FEED_NAME not set")
	}
	handle := os.Getenv("BSKY_HANDLE")
	if handle == "" {
		return fmt.Errorf("BSKY_HANDLE not set")
	}
	appPass := os.Getenv("BSKY_PASS")
	if appPass == "" {
		return fmt.Errorf("BSKY_PASS not set")
	}

	blocklistKey := os.Getenv("BLOCKLIST_KEY")

	host := os.Getenv("BSKY_HOST")
	if host == "" {
		host = "https://bsky.social"
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./"
	}

	dbFilename := path.Join(dbPath, "database.db")
	database, err := db.NewDatabase(dbFilename)
	if err != nil {
		return fmt.Errorf("create new store: %w", err)
	}
	defer database.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var blocklist *consumer.Blocklist
	if blocklistKey != "" {
		blocklist, err = consumer.NewBlocklist(ctx, handle, appPass, host, blocklistKey)
		if err != nil {
			return fmt.Errorf("create blocklist: %w", err)
		}
	}

	go consumeLoop(ctx, database, blocklist)

	server, err := srv.NewServer(serverPort, feedHost, feedName, database)
	if err != nil {
		return fmt.Errorf("create new server: %w", err)
	}
	go func() {
		<-signals
		cancel()
		_ = server.Stop(context.Background())
	}()

	server.Run()
	return nil
}

func consumeLoop(ctx context.Context, database *db.Database, blocklist *consumer.Blocklist) {
	handler := consumer.NewFeedHandler(database, blocklist)

	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := handler.DeleteBlockedPosts(ctx)
				if err != nil {
					slog.Error("delete blocked posts", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	jsServerAddr := os.Getenv("JS_SERVER_ADDR")
	if jsServerAddr == "" {
		jsServerAddr = defaultJetstreamAddr
	}

	consumer := consumer.NewJetstreamConsumer(jsServerAddr, slog.Default(), handler)

	_ = retry.Do(func() error {
		err := consumer.Consume(ctx)
		if err != nil {
			// if the context has been cancelled then it's time to exit
			if errors.Is(err, context.Canceled) {
				return nil
			}
			slog.Error("consume loop", "error", err)
			return err
		}
		return nil
	}, retry.Attempts(0)) // retry indefinitly until context canceled

	slog.Warn("exiting consume loop")
}
