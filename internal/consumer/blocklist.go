package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/xrpc"
)

type Blocklist struct {
	client  *xrpc.Client
	listURI string

	handle   string
	password string
	host     string

	blocked map[string]struct{}
	mu      sync.RWMutex
}

func NewBlocklist(ctx context.Context, handle, password, host, listKey string) (*Blocklist, error) {
	client := &xrpc.Client{
		Host: host,
	}

	err := newAuth(ctx, client, handle, password)
	if err != nil {
		return nil, err
	}

	fullListURI := fmt.Sprintf("at://%s/app.bsky.graph.list/%s", client.Auth.Did, listKey)

	b := &Blocklist{
		client:   client,
		handle:   handle,
		password: password,
		host:     host,
		listURI:  fullListURI,
		blocked:  make(map[string]struct{}),
	}

	slog.Default().InfoContext(ctx, "Initial blocklist fetch...")
	if err := b.refreshList(ctx); err != nil {
		return nil, fmt.Errorf("failed initial blocklist fetch: %w", err)
	}

	// Start background updater
	go b.startBackgroundUpdater(ctx)

	return b, nil
}

func newAuth(ctx context.Context, client *xrpc.Client, handle string, password string) error {
	session, err := atproto.ServerCreateSession(ctx, client, &atproto.ServerCreateSession_Input{
		Identifier: handle,
		Password:   password,
	})
	if err != nil {
		return fmt.Errorf("failed to create bluesky session: %w", err)
	}
	client.Auth = &xrpc.AuthInfo{
		AccessJwt:  session.AccessJwt,
		RefreshJwt: session.RefreshJwt,
		Handle:     session.Handle,
		Did:        session.Did,
	}
	return nil
}

func (b *Blocklist) Contains(did string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, exists := b.blocked[did]
	return exists
}

func (b *Blocklist) GetAll() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	dids := make([]string, 0, len(b.blocked))
	for did := range b.blocked {
		dids = append(dids, did)
	}
	return dids
}

func (b *Blocklist) startBackgroundUpdater(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := b.refreshList(fetchCtx); err != nil {
				if strings.Contains(err.Error(), "ExpiredToken") {
					b.refreshSession(fetchCtx)
					if err := b.refreshList(fetchCtx); err != nil {
						slog.Default().ErrorContext(fetchCtx, "Error refreshing blocklist after session refresh", "error", err)
					}
				}
				slog.Default().ErrorContext(ctx, "Error refreshing blocklist", "error", err)
			}
			cancel()
		}
	}
}

func (b *Blocklist) refreshSession(ctx context.Context) error {
	slog.Default().InfoContext(ctx, "Refreshing session...")
	if err := newAuth(ctx, b.client, b.handle, b.password); err != nil {
		slog.Default().ErrorContext(ctx, "Failed to refresh session", "error", err)
		return err
	}
	return nil
}

func (b *Blocklist) refreshList(ctx context.Context) error {
	var cursor string
	newMap := make(map[string]struct{})

	for {
		resp, err := bsky.GraphGetList(ctx, b.client, cursor, 100, b.listURI)
		if err != nil {
			return err
		}

		for _, item := range resp.Items {
			newMap[item.Subject.Did] = struct{}{}
		}

		if resp.Cursor == nil || *resp.Cursor == "" {
			break
		}
		cursor = *resp.Cursor
	}

	// Hot swap: Lock only for the microsecond it takes to replace the map
	b.mu.Lock()
	b.blocked = newMap
	count := len(b.blocked)
	b.mu.Unlock()

	slog.Default().DebugContext(ctx, "Blocklist updated.", "blockedCount", count)
	return nil
}
