package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchFeedParsesBskyFeedShape(t *testing.T) {
	fixture, err := os.ReadFile("testdata/feed.golden.json")
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	posts, err := fetchFeed(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchFeed() error = %v", err)
	}

	want := []string{
		"at://did:plc:abc/app.bsky.feed.post/post-005",
		"at://did:plc:abc/app.bsky.feed.post/post-004",
		"at://did:plc:abc/app.bsky.feed.post/post-003",
		"at://did:plc:abc/app.bsky.feed.post/post-002",
		"at://did:plc:abc/app.bsky.feed.post/post-001",
	}
	assertPostURIs(t, posts, want)

	if posts[0].Text != "new post with link and images" {
		t.Fatalf("posts[0].Text = %q", posts[0].Text)
	}
	assertLinks(t, posts[0].Links, []string{"https://example.com/link"})
	assertImages(t, posts[0].Images, []postImage{
		{URL: "https://cdn.example.com/image-001-full.jpg", Alt: "image one"},
		{URL: "https://cdn.example.com/image-002-full.jpg", Alt: "image two"},
	})

	if posts[1].External == nil {
		t.Fatal("posts[1].External is nil")
	}
	if posts[1].External.URI != "https://example.com/card" {
		t.Fatalf("posts[1].External.URI = %q", posts[1].External.URI)
	}
	if posts[1].External.Title != "Example Card" {
		t.Fatalf("posts[1].External.Title = %q", posts[1].External.Title)
	}
}

func TestNewPostsSinceReturnsFourPostsFromActualBskyOrder(t *testing.T) {
	posts := []post{
		{URI: "at://did:plc:abc/app.bsky.feed.post/post-005"},
		{URI: "at://did:plc:abc/app.bsky.feed.post/post-004"},
		{URI: "at://did:plc:abc/app.bsky.feed.post/post-003"},
		{URI: "at://did:plc:abc/app.bsky.feed.post/post-002"},
		{URI: "at://did:plc:abc/app.bsky.feed.post/post-001"},
	}

	got := newPostsSince(posts, "at://did:plc:abc/app.bsky.feed.post/post-001")

	want := []string{
		"at://did:plc:abc/app.bsky.feed.post/post-005",
		"at://did:plc:abc/app.bsky.feed.post/post-004",
		"at://did:plc:abc/app.bsky.feed.post/post-003",
		"at://did:plc:abc/app.bsky.feed.post/post-002",
	}
	assertPostURIs(t, got, want)
}

func TestNewPostsSinceReturnsAllPostsBeforePreviousURI(t *testing.T) {
	posts := []post{
		{URI: "at://example/new-4"},
		{URI: "at://example/new-3"},
		{URI: "at://example/new-2"},
		{URI: "at://example/new-1"},
		{URI: "at://example/previous"},
		{URI: "at://example/old"},
	}

	got := newPostsSince(posts, "at://example/previous")
	if len(got) != 4 {
		t.Fatalf("len(new posts) = %d, want 4", len(got))
	}

	want := []string{
		"at://example/new-4",
		"at://example/new-3",
		"at://example/new-2",
		"at://example/new-1",
	}
	for i := range want {
		if got[i].URI != want[i] {
			t.Fatalf("got[%d].URI = %q, want %q", i, got[i].URI, want[i])
		}
	}
}

func TestNewPostsSinceReturnsAllPostsWhenPreviousURIIsMissing(t *testing.T) {
	posts := []post{
		{URI: "at://example/new-2"},
		{URI: "at://example/new-1"},
	}

	got := newPostsSince(posts, "at://example/missing")
	if len(got) != len(posts) {
		t.Fatalf("len(new posts) = %d, want %d", len(got), len(posts))
	}
}

func assertPostURIs(t *testing.T, got []post, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(posts) = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].URI != want[i] {
			t.Fatalf("got[%d].URI = %q, want %q", i, got[i].URI, want[i])
		}
	}
}

func assertLinks(t *testing.T, got []link, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(links) = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].URI != want[i] {
			t.Fatalf("got[%d].URI = %q, want %q", i, got[i].URI, want[i])
		}
	}
}

func assertImages(t *testing.T, got []postImage, want []postImage) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(images) = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
