package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type mattermostConfig struct {
	BaseURL   string
	Token     string
	ChannelID string
}

// loadMattermostConfig returns ok=false when Mattermost is not configured,
// so the fetcher keeps working without it.
func loadMattermostConfig(getenv func(string) string) (mattermostConfig, bool, error) {
	baseURL := strings.TrimSpace(getenv("MM_URL"))
	token := strings.TrimSpace(getenv("MM_TOKEN"))
	channelID := strings.TrimSpace(getenv("MM_CID"))

	if baseURL == "" && token == "" && channelID == "" {
		return mattermostConfig{}, false, nil
	}
	if baseURL == "" || token == "" || channelID == "" {
		return mattermostConfig{}, false, fmt.Errorf("MM_URL, MM_TOKEN and MM_CID must all be set to post to Mattermost")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return mattermostConfig{}, false, fmt.Errorf("parse MM_URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return mattermostConfig{}, false, fmt.Errorf("MM_URL must be an absolute URL")
	}

	return mattermostConfig{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		ChannelID: channelID,
	}, true, nil
}

// postToMattermost sends posts oldest-first. posts is in feed order (newest first).
func postToMattermost(ctx context.Context, cfg mattermostConfig, posts []post) error {
	for i := len(posts) - 1; i >= 0; i-- {
		if err := sendMattermostMessage(ctx, cfg, formatMattermostMessage(posts[i])); err != nil {
			return fmt.Errorf("post %s: %w", posts[i].URI, err)
		}
	}

	return nil
}

func sendMattermostMessage(ctx context.Context, cfg mattermostConfig, message string) error {
	body, err := json.Marshal(map[string]string{
		"channel_id": cfg.ChannelID,
		"message":    message,
	})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/api/v4/posts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "bsky2other/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("mattermost status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func formatMattermostMessage(p post) string {
	var parts []string
	if p.Text != "" {
		parts = append(parts, p.Text)
	}

	for _, l := range p.Links {
		parts = append(parts, l.URI)
	}
	if p.External != nil && !hasLink(p.Links, p.External.URI) {
		parts = append(parts, p.External.URI)
	}
	if p.Quote != nil {
		if u := bskyWebURL(p.Quote.URI); u != "" {
			parts = append(parts, "引用: "+u)
		}
	}
	for _, img := range p.Images {
		parts = append(parts, fmt.Sprintf("![%s](%s)", strings.NewReplacer("[", "", "]", "").Replace(img.Alt), img.URL))
	}
	if u := bskyWebURL(p.URI); u != "" {
		parts = append(parts, u)
	}

	return strings.Join(parts, "\n\n")
}

func hasLink(links []link, uri string) bool {
	for _, l := range links {
		if l.URI == uri {
			return true
		}
	}

	return false
}

// bskyWebURL converts at://did/app.bsky.feed.post/rkey to a bsky.app URL.
func bskyWebURL(atURI string) string {
	rest, ok := strings.CutPrefix(atURI, "at://")
	if !ok {
		return ""
	}
	segments := strings.Split(rest, "/")
	if len(segments) != 3 || segments[1] != "app.bsky.feed.post" {
		return ""
	}

	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", segments[0], segments[2])
}
