package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nacorid/x402-feed/internal/auth"
)

const (
	defaultLimit = 50
)

// FeedSkeletonReponse describes a response that will contain a skeleton feed
type FeedSkeletonReponse struct {
	Cursor string             `json:"cursor"`
	Feed   []FeedSkeletonPost `json:"feed"`
}

// FeedSkeletonPost describes an individual post which is just the post URI
type FeedSkeletonPost struct {
	Post        string `json:"post"`
	FeedContext string `json:"feedContext"`
}

// HandleGetFeedSkeleton is the handler that will build up and return a feed response
func (s *Server) HandleGetFeedSkeleton(w http.ResponseWriter, r *http.Request) {
	slog.Debug("got request for feed skeleton", "host", r.RemoteAddr)

	// if you need to get a feed based on the user making the request you can use this to get the callers DID.
	// It's also a good idea to have this here incase you're getting spammed by non bluesky users - looking at you bots!
	_, err := auth.GetRequestUserDID(r)
	if err != nil {
		slog.Error("validate user auth", "error", err)
		http.Error(w, "validate auth", http.StatusUnauthorized)
		return
	}

	params := r.URL.Query()

	feed := params.Get("feed")
	if feed == "" {
		slog.Error("missing feed query param", "host", r.RemoteAddr)
		http.Error(w, "missing feed query param", http.StatusBadRequest)
		return
	}
	slog.Debug("request for feed", "feed", feed)

	limit, err := limitFromParams(params)
	if err != nil {
		slog.Error("get limit from params", "error", err)
		http.Error(w, "invalid limit query param", http.StatusBadRequest)
		return
	}
	if limit < 1 || limit > 100 {
		limit = defaultLimit
	}

	cursor := params.Get("cursor")

	resp, err := s.getFeed(r.Context(), feed, cursor, limit)
	if err != nil {
		slog.Error("get feed", "error", err, "feed", feed)
		http.Error(w, "error getting feed", http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(resp)
	if err != nil {
		slog.Error("marshall error", "error", err, "host", r.RemoteAddr)
		http.Error(w, "failed to encode resp", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	_, _ = w.Write(b)
}

// DescribeFeedResponse is what's returned when the 'app.bsky.feed.describeFeedGenerator' endpoint is called
type DescribeFeedResponse struct {
	DID   string `json:"did"`
	Feeds []Feed `json:"feeds"`
}

// Feed describes the feed URI
type Feed struct {
	URI string `json:"uri"`
}

// HandleDescribeFeedGenerator handles the describe feed generator endpoint
func (s *Server) HandleDescribeFeedGenerator(w http.ResponseWriter, r *http.Request) {
	slog.Debug("got request for describe feed", "host", r.RemoteAddr)
	resp := DescribeFeedResponse{
		DID: fmt.Sprintf("did:web:%s", s.feedHost),
		Feeds: []Feed{
			{
				URI: fmt.Sprintf("at://%s/app.bsky.feed.generator/%s", s.feedHost, s.feedName),
			},
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode resp", http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(b)
}

// FeedInteractions details the interactions that a user had with a feed when they viewed it
type FeedInteractions struct {
	Interactions []Interaction `json:"interactions"`
}

type Interaction struct {
	Item  string `json:"item"`
	Event string `json:"event"`
}

// HandleFeedInteractions will handle when the client sends back a feed interaction so you can improve
// the feed quality for the user
func (s *Server) HandleFeedInteractions(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handle feed interactions")
	userDID, err := auth.GetRequestUserDID(r)
	if err != nil {
		slog.Error("validate user auth", "error", err)
		http.Error(w, "validate auth", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("read feed interactions request body", "error", err)
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	var feedInteractions FeedInteractions
	err = json.Unmarshal(body, &feedInteractions)
	if err != nil {
		slog.Error("decode feed interactions request body", "error", err)
		http.Error(w, "decode body", http.StatusBadRequest)
		return
	}

	// here is where you would likely do something with the data that is sent to you such as improving the
	// data you store for a users feed
	for _, interaction := range feedInteractions.Interactions {
		slog.Info("interaction for user", "user", userDID, "item", interaction.Item, "interaction", interaction.Event)
	}
}

// WellKnownResponse is what's returned on a well-known endpoint
type WellKnownResponse struct {
	Context []string           `json:"@context"`
	Id      string             `json:"id"`
	Service []WellKnownService `json:"service"`
}

// WellKnownService describes the service returned on a well-known endpoint
type WellKnownService struct {
	Id              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// HandleWellKnown handles returning a well-known endpoint
func (s *Server) HandleWellKnown(w http.ResponseWriter, r *http.Request) {
	slog.Debug("got request for well known", "host", r.RemoteAddr)
	resp := WellKnownResponse{
		Context: []string{"https://www.w3.org/ns/did/v1"},
		Id:      fmt.Sprintf("did:web:%s", s.feedHost),
		Service: []WellKnownService{
			{
				Id:              "#bsky_fg",
				Type:            "BskyFeedGenerator",
				ServiceEndpoint: fmt.Sprintf("https://%s", s.feedHost),
			},
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode resp", http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(b)
}

func limitFromParams(params url.Values) (int, error) {
	limitStr := params.Get("limit")
	if limitStr == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return 0, fmt.Errorf("parsing limit param: %w", err)
	}
	return limit, nil
}

func (s *Server) getFeed(ctx context.Context, feed, cursor string, limit int) (FeedSkeletonReponse, error) {
	resp := FeedSkeletonReponse{
		Feed: make([]FeedSkeletonPost, 0),
	}

	cursorInt, err := strconv.Atoi(cursor)
	if err != nil && cursor != "" {
		slog.Error("convert cursor to int", "error", err, "cursor value", cursor)
	}
	if cursorInt == 0 {
		// if no cursor provided use a date waaaaay in the future to start the less than query
		cursorInt = 9999999999999
	}

	posts, err := s.postStore.GetFeedPosts(cursorInt, limit)
	if err != nil {
		return resp, fmt.Errorf("get feed from DB: %w", err)
	}

	usersFeed := make([]FeedSkeletonPost, 0, len(posts))
	for _, post := range posts {
		usersFeed = append(usersFeed, FeedSkeletonPost{
			Post: post.PostURI,
		})
	}

	resp.Feed = usersFeed

	// only set the return cursor if there was at least 1 record returned and that the len of records
	// being returned is the same as the limit
	if len(posts) > 0 && len(posts) == limit {
		lastPost := posts[len(posts)-1]
		resp.Cursor = fmt.Sprintf("%d", lastPost.CreatedAt)
	}
	return resp, nil
}
