package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultStateFile = "state.json"
	defaultPostsFile = "posts.json"
)

type config struct {
	FeedURL   string
	PostsFile string
	StateFile string
	Timeout   time.Duration
}

type bskyFeedResponse struct {
	Feed []struct {
		Post bskyPostView `json:"post"`
	} `json:"feed"`
}

type bskyPostView struct {
	URI    string `json:"uri"`
	Record struct {
		Text   string `json:"text"`
		Facets []struct {
			Features []struct {
				Type string `json:"$type"`
				URI  string `json:"uri"`
			} `json:"features"`
		} `json:"facets"`
	} `json:"record"`
	Embed bskyEmbed `json:"embed"`
}

type bskyEmbed struct {
	Type     string        `json:"$type"`
	Images   []bskyImage   `json:"images"`
	External *bskyExternal `json:"external"`
	Media    *bskyEmbed    `json:"media"`
}

type bskyImage struct {
	Alt      string `json:"alt"`
	Fullsize string `json:"fullsize"`
	Thumb    string `json:"thumb"`
}

type bskyExternal struct {
	URI         string `json:"uri"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type post struct {
	URI      string      `json:"uri"`
	Text     string      `json:"text"`
	Links    []link      `json:"links,omitempty"`
	Images   []postImage `json:"images,omitempty"`
	External *external   `json:"external,omitempty"`
}

type link struct {
	URI string `json:"uri"`
}

type postImage struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

type external struct {
	URI         string `json:"uri"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type state struct {
	LatestPostURI string `json:"latest_post_uri"`
	UpdatedAt     string `json:"updated_at"`
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	prev, err := readState(cfg.StateFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if prev.LatestPostURI == "" {
		posts, err := fetchFeed(ctx, cfg.FeedURL)
		if err != nil {
			return err
		}
		if err := writePosts(cfg.PostsFile, posts); err != nil {
			return err
		}
		if len(posts) == 0 {
			fmt.Println("no posts found")
			return nil
		}

		latest := state{
			LatestPostURI: posts[0].URI,
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeState(cfg.StateFile, latest); err != nil {
			return err
		}

		fmt.Printf("initialized state with latest post uri %q\n", latest.LatestPostURI)
		return nil
	}

	posts, err := fetchFeed(ctx, cfg.FeedURL)
	if err != nil {
		return err
	}
	if err := writePosts(cfg.PostsFile, posts); err != nil {
		return err
	}
	if len(posts) == 0 {
		fmt.Println("no posts found")
		return nil
	}

	latest := state{
		LatestPostURI: posts[0].URI,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	newPosts := newPostsSince(posts, prev.LatestPostURI)

	for i := len(newPosts) - 1; i >= 0; i-- {
		fmt.Printf("new post: %s\n", newPosts[i].URI)
	}

	if len(newPosts) == 0 {
		fmt.Println("no new posts")
		return nil
	}

	if err := writeState(cfg.StateFile, latest); err != nil {
		return err
	}

	fmt.Printf("saved latest post uri %q to %s\n", latest.LatestPostURI, cfg.StateFile)
	return nil
}

func loadConfig() (config, error) {
	feedURL := strings.TrimSpace(os.Getenv("BSKY_FEED_URL"))
	if feedURL == "" {
		return config{}, errors.New("BSKY_FEED_URL is required")
	}

	parsed, err := url.Parse(feedURL)
	if err != nil {
		return config{}, fmt.Errorf("parse BSKY_FEED_URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return config{}, errors.New("BSKY_FEED_URL must be an absolute URL")
	}

	stateFile := strings.TrimSpace(os.Getenv("STATE_FILE"))
	if stateFile == "" {
		stateFile = defaultStateFile
	}

	postsFile := strings.TrimSpace(os.Getenv("POSTS_FILE"))
	if postsFile == "" {
		postsFile = defaultPostsFile
	}

	return config{
		FeedURL:   feedURL,
		PostsFile: postsFile,
		StateFile: stateFile,
		Timeout:   30 * time.Second,
	}, nil
}

func fetchFeed(ctx context.Context, feedURL string) ([]post, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bsky2other/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch feed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var feed bskyFeedResponse
	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode feed response: %w", err)
	}

	posts := make([]post, 0, len(feed.Feed))
	for _, item := range feed.Feed {
		if item.Post.URI == "" {
			continue
		}

		posts = append(posts, postFromView(item.Post))
	}

	return posts, nil
}

func postFromView(view bskyPostView) post {
	p := post{
		URI:  view.URI,
		Text: view.Record.Text,
	}

	for _, facet := range view.Record.Facets {
		for _, feature := range facet.Features {
			if feature.URI != "" {
				p.Links = appendUniqueLink(p.Links, link{URI: feature.URI})
			}
		}
	}

	collectEmbed(view.Embed, &p)

	return p
}

func collectEmbed(embed bskyEmbed, p *post) {
	for _, bskyImage := range embed.Images {
		imageURL := bskyImage.Fullsize
		if imageURL == "" {
			imageURL = bskyImage.Thumb
		}
		if imageURL != "" {
			p.Images = appendUniqueImage(p.Images, postImage{URL: imageURL, Alt: bskyImage.Alt})
		}
	}

	if p.External == nil && embed.External != nil && embed.External.URI != "" {
		p.External = &external{
			URI:         embed.External.URI,
			Title:       embed.External.Title,
			Description: embed.External.Description,
		}
	}

	if embed.Media != nil {
		collectEmbed(*embed.Media, p)
	}
}

func appendUniqueLink(links []link, value link) []link {
	for _, existing := range links {
		if existing.URI == value.URI {
			return links
		}
	}

	return append(links, value)
}

func appendUniqueImage(images []postImage, value postImage) []postImage {
	for _, existing := range images {
		if existing.URL == value.URL {
			return images
		}
	}

	return append(images, value)
}

func indexPostURI(posts []post, uri string) int {
	for i, post := range posts {
		if post.URI == uri {
			return i
		}
	}

	return -1
}

func newPostsSince(posts []post, previousURI string) []post {
	foundAt := indexPostURI(posts, previousURI)
	if foundAt < 0 {
		return posts
	}

	return posts[:foundAt]
}

func readState(filename string) (state, error) {
	f, err := os.Open(filename)
	if err != nil {
		return state{}, err
	}
	defer f.Close()

	var s state
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return state{}, fmt.Errorf("read state file: %w", err)
	}

	return s, nil
}

func writeState(filename string, s state) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create state file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

func writePosts(filename string, posts []post) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create posts file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(posts); err != nil {
		return fmt.Errorf("write posts file: %w", err)
	}

	return nil
}
