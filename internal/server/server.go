package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

// Post describes a Bluesky post
type Post struct {
	ID        int
	RKey      string
	PostURI   string
	UserDID   string
	CreatedAt int64
}

// PostStore defines the interactions with a store
type PostStore interface {
	GetFeedPosts(cursor, limit int) ([]Post, error)
	CreatePost(post Post) error
}

// Server is the feed server that will be called when a user requests to view a feed
type Server struct {
	httpsrv   *http.Server
	postStore PostStore
	feedHost  string
	feedName  string
}

// NewServer builds a server - call the Run function to start the server
func NewServer(port int, feedHost, feedName string, postStore PostStore) (*Server, error) {
	srv := &Server{
		feedHost:  feedHost,
		feedName:  feedName,
		postStore: postStore,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/xrpc/app.bsky.feed.getFeedSkeleton", srv.HandleGetFeedSkeleton)
	mux.HandleFunc("/xrpc/app.bsky.feed.describeFeedGenerator", srv.HandleDescribeFeedGenerator)
	mux.HandleFunc("POST /xrpc/app.bsky.feed.sendInteractions", srv.HandleFeedInteractions)
	mux.HandleFunc("/.well-known/did.json", srv.HandleWellKnown)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	srv.httpsrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return srv, nil
}

// Run will start the server - it is a blocking function
func (s *Server) Run() {
	err := s.httpsrv.ListenAndServe()
	if err != nil {
		slog.Error("listen and serve", "error", err)
	}
}

// Stop will shutdown the server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpsrv.Shutdown(ctx)
}
