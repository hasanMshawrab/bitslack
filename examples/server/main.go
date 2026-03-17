// Command server is a reference implementation showing how to wire up
// the bitslack library with simple in-memory adapters.
//
// It exposes a single endpoint:
//
//	POST /webhook  — receives Bitbucket webhook events (configure this URL in
//	                 Bitbucket repository settings → Webhooks)
//
// Environment variables:
//
//	SLACK_BOT_TOKEN      - Slack bot token (xoxb-...)
//	BITBUCKET_USERNAME   - Bitbucket account email used for API auth
//	BITBUCKET_TOKEN      - Bitbucket API access token
//	BITSLACK_CHANNEL_MAP - Repo-to-channel mapping: "workspace/repo=CHANNELID,..."
//	BITSLACK_USER_MAP    - User-to-Slack mapping: "bbuser=SLACKID,..."
//	PORT                 - HTTP listen port (default: 8080)
package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hasanMshawrab/bitslack"
)

const readHeaderTimeout = 10 * time.Second

func main() {
	logger := slog.Default()

	slackToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackToken == "" {
		logger.Error("SLACK_BOT_TOKEN is required")
		os.Exit(1)
	}

	bbUsername := os.Getenv("BITBUCKET_USERNAME")
	if bbUsername == "" {
		logger.Error("BITBUCKET_USERNAME is required")
		os.Exit(1)
	}

	bbToken := os.Getenv("BITBUCKET_TOKEN")
	if bbToken == "" {
		logger.Error("BITBUCKET_TOKEN is required")
		os.Exit(1)
	}

	channelMap := parseMap(os.Getenv("BITSLACK_CHANNEL_MAP"))
	userMap := parseMap(os.Getenv("BITSLACK_USER_MAP"))

	client, err := bitslack.New(bitslack.Config{
		SlackToken:        slackToken,
		BitbucketUsername: bbUsername,
		BitbucketToken:    bbToken,
		ThreadStore:       &memThreadStore{},
		ConfigStore:       &memConfigStore{channels: channelMap, users: userMap},
		Logger:            &slogLogger{l: logger},
	})
	if err != nil {
		logger.Error("failed to create bitslack client", "error", err)
		os.Exit(1)
	}

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		eventKey := r.Header.Get("X-Event-Key")

		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			logger.Error("failed to read request body", "error", readErr)
			w.WriteHeader(http.StatusOK)
			return
		}

		if handleErr := client.Handler(r.Context(), eventKey, body); handleErr != nil {
			logger.Error("handler error", "event", eventKey, "error", handleErr)
		}

		// Always return 200 to Bitbucket.
		w.WriteHeader(http.StatusOK)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	logger.Info("starting server", "port", port, "channels", len(channelMap), "users", len(userMap))
	listenErr := srv.ListenAndServe()
	if listenErr != nil {
		logger.Error("server stopped", "error", listenErr)
		os.Exit(1)
	}
}

// parseMap parses "key1=val1,key2=val2" into a map.
func parseMap(s string) map[string]string {
	m := make(map[string]string)
	if s == "" {
		return m
	}
	for pair := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// memThreadStore is an in-memory ThreadStore backed by [sync.Map].
type memThreadStore struct {
	m sync.Map
}

func (s *memThreadStore) Get(_ context.Context, prKey string) (string, bool, error) {
	v, ok := s.m.Load(prKey)
	if !ok {
		return "", false, nil
	}
	ts, _ := v.(string)
	return ts, true, nil
}

func (s *memThreadStore) Store(_ context.Context, prKey string, ts string) error {
	s.m.Store(prKey, ts)
	return nil
}

// memConfigStore is an in-memory ConfigStore backed by plain maps.
type memConfigStore struct {
	channels map[string]string
	users    map[string]string
}

func (c *memConfigStore) GetChannel(repo string) (string, bool) {
	ch, ok := c.channels[repo]
	return ch, ok
}

func (c *memConfigStore) GetSlackUserID(username string) (string, bool) {
	id, ok := c.users[username]
	return id, ok
}

// slogLogger wraps [slog.Logger] to satisfy bitslack.Logger.
type slogLogger struct {
	l *slog.Logger
}

func (sl *slogLogger) Info(msg string)  { sl.l.Info(msg) }
func (sl *slogLogger) Warn(msg string)  { sl.l.Warn(msg) }
func (sl *slogLogger) Error(msg string) { sl.l.Error(msg) }

// Compile-time interface checks.
var (
	_ bitslack.ThreadStore = (*memThreadStore)(nil)
	_ bitslack.ConfigStore = (*memConfigStore)(nil)
	_ bitslack.Logger      = (*slogLogger)(nil)
)
