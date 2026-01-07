package consumer

import (
	"context"
	"encoding/json"
	"strings"

	"fmt"
	"log/slog"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/client/schedulers/sequential"
	"github.com/bluesky-social/jetstream/pkg/models"

	"github.com/nacorid/x402-feed/internal/server"
)

// JetstreamConsumer is responsible for consuming from a jetstream instance
type JetstreamConsumer struct {
	cfg     *client.ClientConfig
	handler *Handler
	logger  *slog.Logger
}

// NewJetstreamConsumer configures a new jetstream consumer. To run or start you should call the Consume function
func NewJetstreamConsumer(jsAddr string, logger *slog.Logger, handler *Handler) *JetstreamConsumer {
	cfg := client.DefaultClientConfig()
	if jsAddr != "" {
		cfg.WebsocketURL = jsAddr
	}
	cfg.WantedCollections = []string{
		"app.bsky.feed.post",
	}
	cfg.WantedDids = []string{}

	return &JetstreamConsumer{
		cfg:     cfg,
		logger:  logger,
		handler: handler,
	}
}

// Consume will connect to a Jetstream client and start to consume and handle messages from it
func (c *JetstreamConsumer) Consume(ctx context.Context) error {
	scheduler := sequential.NewScheduler("jetstream", c.logger, c.handler.HandleEvent)
	defer scheduler.Shutdown()

	client, err := client.NewClient(c.cfg, c.logger, scheduler)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	cursor := time.Now().Add(1 * -time.Minute).UnixMicro()

	if err := client.ConnectAndRead(ctx, &cursor); err != nil {
		return fmt.Errorf("connect and read: %w", err)
	}

	slog.Info("stopping consume")
	return nil
}

// Handler is responsible for handling a message consumed from Jetstream
type Handler struct {
	store     server.PostStore
	blocklist *Blocklist
}

// NewFeedHandler returns a new handler
func NewFeedHandler(store server.PostStore, blocklist *Blocklist) *Handler {
	return &Handler{store: store, blocklist: blocklist}
}

// HandleEvent will handle an event based on the event's commit operation
func (h *Handler) HandleEvent(ctx context.Context, event *models.Event) error {
	if event.Commit == nil {
		return nil
	}

	switch event.Commit.Operation {
	case models.CommitOperationCreate:
		return h.handleCreateEvent(ctx, event)
		// TODO: handle deletes too
	default:
		return nil
	}
}

func (h *Handler) handleCreateEvent(_ context.Context, event *models.Event) error {
	if event.Commit.Collection != "app.bsky.feed.post" {
		return nil
	}

	var bskyPost apibsky.FeedPost
	if err := json.Unmarshal(event.Commit.Record, &bskyPost); err != nil {
		// ignore this
		return nil
	}

	// this is where logic goes for what posts you wish to store for a feed but for this example
	// just look for any post that contains the #golang hashtag
	if !strings.Contains(strings.ToLower(bskyPost.Text), "x402") {
		return nil
	}

	if h.blocklist != nil && h.blocklist.Contains(event.Did) {
		return nil
	}

	createdAt, err := time.Parse(time.RFC3339, bskyPost.CreatedAt)
	if err != nil {
		slog.Error("parsing createdAt time from post", "error", err, "timestamp", bskyPost.CreatedAt)
		createdAt = time.Now().UTC()
	}

	postURI := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", event.Did, event.Commit.RKey)
	post := server.Post{
		RKey:      event.Commit.RKey,
		PostURI:   postURI,
		CreatedAt: createdAt.UnixMilli(),
	}
	err = h.store.CreatePost(post)
	if err != nil {
		slog.Error("error creating post in store", "error", err)
		return nil
	}
	return nil
}
