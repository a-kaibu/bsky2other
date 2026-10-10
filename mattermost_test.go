package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadMattermostConfig(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	if _, ok, err := loadMattermostConfig(env(nil)); ok || err != nil {
		t.Fatalf("unset: ok=%v err=%v", ok, err)
	}
	if _, _, err := loadMattermostConfig(env(map[string]string{"MM_TOKEN": "t"})); err == nil {
		t.Fatal("partial config should error")
	}
	cfg, ok, err := loadMattermostConfig(env(map[string]string{
		"MM_URL": "https://mm.example.com/", "MM_TOKEN": "t", "MM_CID": "c",
	}))
	if err != nil || !ok || cfg.BaseURL != "https://mm.example.com" {
		t.Fatalf("full: cfg=%+v ok=%v err=%v", cfg, ok, err)
	}
}

func TestPostToMattermostSendsOldestFirst(t *testing.T) {
	var messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/posts" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected request: %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["channel_id"] != "chan" {
			t.Errorf("channel_id = %q", body["channel_id"])
		}
		messages = append(messages, body["message"])
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cfg := mattermostConfig{BaseURL: srv.URL, Token: "tok", ChannelID: "chan"}
	err := postToMattermost(context.Background(), cfg, []post{{Text: "new"}, {Text: "old"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0] != "old" || messages[1] != "new" {
		t.Fatalf("messages = %q", messages)
	}
}

func TestPostToMattermostReturnsErrorOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := mattermostConfig{BaseURL: srv.URL, Token: "bad", ChannelID: "chan"}
	err := postToMattermost(context.Background(), cfg, []post{{URI: "at://x", Text: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestFormatMattermostMessage(t *testing.T) {
	got := formatMattermostMessage(post{
		URI:      "at://did:plc:abc/app.bsky.feed.post/3k",
		Text:     "hello",
		Links:    []link{{URI: "https://example.com"}},
		External: &external{URI: "https://example.com"},
		Images:   []postImage{{URL: "https://img/1.jpg", Alt: "a [b]"}},
	})
	want := "hello\n\nhttps://example.com\n\n![a b](https://img/1.jpg)"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
