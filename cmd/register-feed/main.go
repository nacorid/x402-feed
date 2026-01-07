package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

const (
	httpClientTimeoutDuration        = time.Second * 5
	transportIdleConnTimeoutDuration = time.Second * 90
)

var baseurl string

type auth struct {
	AccessJwt string `json:"accessJwt"`
	Did       string `json:"did"`
}

type registerFeedGen struct {
	Repo       string         `json:"repo"`
	Collection string         `json:"collection"`
	Rkey       string         `json:"rkey"`
	Record     registerRecord `json:"record"`
}

type registerRecord struct {
	Did                 string    `json:"did"`
	DisplayName         string    `json:"displayName"`
	Description         string    `json:"description"`
	CreatedAt           time.Time `json:"createdAt"`
	AcceptsInteractions bool      `json:"acceptsInteractions"`
}

func main() {
	err := run()
	if err != nil {
		slog.Error("error registering feed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error loading .env file")
	}

	host := os.Getenv("BSKY_HOST")
	if host == "" {
		host = "https://bsky.social"
	}
	baseurl = host + "/xrpc"

	httpClient := http.Client{
		Timeout: httpClientTimeoutDuration,
		Transport: &http.Transport{
			IdleConnTimeout: transportIdleConnTimeoutDuration,
		},
	}
	auth, err := login(httpClient)
	if err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}

	err = Register(auth, httpClient)
	if err != nil {
		return err
	}

	return nil
}

func login(client http.Client) (*auth, error) {
	handle := os.Getenv("BSKY_HANDLE")
	appPass := os.Getenv("BSKY_PASS")

	url := fmt.Sprintf("%s/com.atproto.server.createsession", baseurl)

	requestData := map[string]interface{}{
		"identifier": handle,
		"password":   appPass,
	}

	data, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	r := bytes.NewReader(data)

	req, err := http.NewRequest("POST", url, r)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	defer func() {
		_ = res.Body.Close()
	}()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var loginResp auth
	err = json.Unmarshal(resBody, &loginResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &loginResp, nil
}

func Register(auth *auth, httpClient http.Client) error {
	feedName := os.Getenv("FEED_NAME")
	if feedName == "" {
		return fmt.Errorf("FEED_NAME env not set")
	}
	feedDisplayName := os.Getenv("FEED_DISPLAY_NAME")
	if feedDisplayName == "" {
		return fmt.Errorf("FEED_DISPLAY_NAME env not set")
	}
	feedDescription := os.Getenv("FEED_DESCRIPTION")
	if feedDescription == "" {
		return fmt.Errorf("FEED_DESCRIPTION env not set")
	}
	feedDID := os.Getenv("FEED_DID")
	if feedDID == "" {
		return fmt.Errorf("FEED_DID environment not set")
	}
	acceptsInteractions := false
	if os.Getenv("ACCEPTS_INTERACTIONS") == "true" {
		acceptsInteractions = true
	}

	reqData := registerFeedGen{
		Repo:       auth.Did,
		Collection: "app.bsky.feed.generator",
		Rkey:       feedName,
		Record: registerRecord{
			Did:                 feedDID,
			DisplayName:         feedDisplayName,
			Description:         feedDescription,
			CreatedAt:           time.Now(),
			AcceptsInteractions: acceptsInteractions,
		},
	}

	data, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	r := bytes.NewReader(data)

	url := fmt.Sprintf("%s/com.atproto.repo.putRecord", baseurl)
	req, err := http.NewRequest("POST", url, r)
	if err != nil {
		return fmt.Errorf("failed to create new post request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", auth.AccessJwt))

	res, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make create post request: %w", err)
	}

	defer func() {
		_ = res.Body.Close()
	}()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(b))
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("failed to create post: %v", res.StatusCode)
	}

	return nil
}
