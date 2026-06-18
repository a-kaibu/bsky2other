# bsky2other

`bsky2other` is a small command line tool that fetches a Bluesky feed JSON endpoint
and writes normalized post data to a local JSON file.

It keeps a state file with the latest seen post URI, so repeated runs can report
only posts that appeared since the previous run.

## Features

- Fetches a Bluesky feed endpoint as JSON
- Skips reposts
- Extracts post text, links, images, external cards, replies, and quotes
- Removes rich-text link facet display text from post text
- Writes normalized posts to `posts.json`
- Tracks the latest seen post in `state.json`

## Usage

Set `BSKY_FEED_URL` to an absolute Bluesky feed API URL and run the tool:

```sh
BSKY_FEED_URL="https://public.api.bsky.app/xrpc/app.bsky.feed.getAuthorFeed?actor=example.com" go run .
```

On the first run, the tool initializes `state.json` with the latest post URI and
writes the current feed items to `posts.json`.

On later runs, it prints newly seen post URIs from oldest to newest, refreshes
`posts.json`, and updates `state.json`.

## Configuration

Configuration is provided through environment variables.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `BSKY_FEED_URL` | yes | | Absolute URL for the Bluesky feed JSON endpoint. |
| `POSTS_FILE` | no | `posts.json` | Path for normalized post output. |
| `STATE_FILE` | no | `state.json` | Path for latest-seen post state. |

## Output

`posts.json` contains an array of normalized posts:

```json
[
  {
    "uri": "at://did:plc:example/app.bsky.feed.post/example",
    "text": "hello",
    "links": [{ "uri": "https://example.com" }],
    "images": [{ "url": "https://cdn.example.com/image.jpg", "alt": "example" }]
  }
]
```

`state.json` stores the latest feed item URI and update timestamp:

```json
{
  "latest_post_uri": "at://did:plc:example/app.bsky.feed.post/example",
  "updated_at": "2026-06-18T00:00:00Z"
}
```

## Development

Run tests with:

```sh
go test ./...
```

## License

MIT
