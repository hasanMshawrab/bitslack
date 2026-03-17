package slack_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hasanMshawrab/bitslack/internal/slack"
)

func TestPostMessage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"ts":"1234567890.123456"}`))
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	ts, err := c.PostMessage(context.Background(), "#general", "", "hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts != "1234567890.123456" {
		t.Fatalf("got ts=%q, want %q", ts, "1234567890.123456")
	}
}

func TestPostMessage_AsThreadReply(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"ts":"1234567890.999999"}`))
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	_, err := c.PostMessage(context.Background(), "#general", "1234567890.000001", "reply", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	threadTS, _ := captured["thread_ts"].(string)
	if threadTS != "1234567890.000001" {
		t.Fatalf("got thread_ts=%q, want %q", threadTS, "1234567890.000001")
	}
}

func TestPostMessage_SlackAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	_, err := c.PostMessage(context.Background(), "#nonexistent", "", "hello", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var slkErr *slack.Error
	if !errors.As(err, &slkErr) {
		t.Fatalf("expected *slack.Error, got %T: %v", err, err)
	}
	if slkErr.Code != "channel_not_found" {
		t.Fatalf("got code=%q, want %q", slkErr.Code, "channel_not_found")
	}
}

func TestPostMessage_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	_, err := c.PostMessage(context.Background(), "#general", "", "hello", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateMessage_Success(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"ts":"1234567890.123456"}`))
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	err := c.UpdateMessage(context.Background(), "#general", "1234567890.123456", "updated", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	channel, _ := captured["channel"].(string)
	if channel != "#general" {
		t.Fatalf("got channel=%q, want %q", channel, "#general")
	}
	ts, _ := captured["ts"].(string)
	if ts != "1234567890.123456" {
		t.Fatalf("got ts=%q, want %q", ts, "1234567890.123456")
	}
}

func TestUpdateMessage_SlackAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"error":"message_not_found"}`))
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	err := c.UpdateMessage(context.Background(), "#general", "1234567890.123456", "updated", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var slkErr *slack.Error
	if !errors.As(err, &slkErr) {
		t.Fatalf("expected *slack.Error, got %T: %v", err, err)
	}
	if slkErr.Code != "message_not_found" {
		t.Fatalf("got code=%q, want %q", slkErr.Code, "message_not_found")
	}
}

func TestClient_BearerToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"ts":"1234567890.123456"}`))
	}))
	defer srv.Close()

	c := slack.NewClient("xoxb-test", slack.WithBaseURL(srv.URL), slack.WithHTTPClient(srv.Client()))
	_, _ = c.PostMessage(context.Background(), "#general", "", "hello", nil)
	if capturedAuth != "Bearer xoxb-test" {
		t.Fatalf("got Authorization=%q, want %q", capturedAuth, "Bearer xoxb-test")
	}
}
