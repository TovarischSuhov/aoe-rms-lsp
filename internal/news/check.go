package news

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxFeedPages bounds feed pagination per run. Ten pages × ~5 items
// cover weeks of news; exhausting the cap with full pages and no post
// older than the anchor is treated as a source anomaly (see Check).
const maxFeedPages = 10

// httpTimeout bounds one feed-page request.
const httpTimeout = 30 * time.Second

// aoe2Title matches "Age of Empires II" without matching III: the
// Roman-numeral trap makes a plain substring check wrong.
var aoe2Title = regexp.MustCompile(`Age of Empires II([^I]|$)`)

// patchTitle matches the patch-post wording of the changelog regimen:
// Update / Minor Update / Hotfix / Update Preview — "update" covers all.
var patchTitle = regexp.MustCompile(`(?i)\b(update|hotfix)\b`)

// seedSection matches the newest changelog section header,
// e.g. "## Update 177723 — 2026-06-02".
var seedSection = regexp.MustCompile(`^## Update (\S+) — (\d{4}-\d{2}-\d{2})\s*$`)

// rssFeed is the subset of RSS 2.0 the detector consumes.
type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
}

// SeedFromChangelog returns the cold-start anchor: the newest verified
// release with XS/RMS changes, taken from the top section of the
// changelog document.
func SeedFromChangelog(changelogPath string) (Post, error) {
	data, err := os.ReadFile(changelogPath)
	if err != nil {
		return Post{}, fmt.Errorf("news: read changelog %s: %w", changelogPath, err)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		m := seedSection.FindStringSubmatch(strings.TrimRight(line, "\r"))

		if m == nil {
			continue
		}

		post, err := seedPost(changelogPath, m[1], m[2], string(data))
		if err != nil {
			return Post{}, err
		}

		return post, nil
	}

	return Post{}, fmt.Errorf("news: %s: no \"## Update <id> — YYYY-MM-DD\" section", changelogPath)
}

// seedPost builds the anchor Post from a parsed section header: the URL
// is the first non-empty line after the header.
func seedPost(path, id, date, data string) (Post, error) {
	lines := strings.Split(data, "\n")

	for i, line := range lines {
		if seedSection.MatchString(strings.TrimRight(line, "\r")) {
			for _, next := range lines[i+1:] {
				next = strings.TrimSpace(next)

				if next == "" {
					continue
				}

				if !strings.HasPrefix(next, "http") {
					return Post{}, fmt.Errorf("news: %s: expected a post URL after the top section, got %q", path, next)
				}

				return Post{
					Title:     "Update " + id,
					URL:       next,
					Published: date + "T00:00:00Z",
				}, nil
			}
		}
	}

	return Post{}, fmt.Errorf("news: %s: no URL line under the top section", path)
}

// Check runs the detector once: from the current anchor to new posts.
// The state file is never written here — the caller saves the new anchor
// with SaveState after the batch issue has been created.
func Check(ctx context.Context, feedURL, statePath, changelogPath string, log *slog.Logger) (Result, error) {
	if log == nil {
		log = slog.Default()
	}

	anchor, cold, err := anchor(statePath, changelogPath)
	if err != nil {
		return Result{}, err
	}

	if cold {
		log.InfoContext(ctx, "news: cold start, seeded from changelog", "anchor", anchor.Published)
	}

	anchorAt, err := time.Parse(time.RFC3339, anchor.Published)
	if err != nil {
		return Result{}, fmt.Errorf("news: anchor date %q: %w", anchor.Published, err)
	}

	posts, anchored, end, err := scanFeed(ctx, feedURL, anchorAt, log)
	if err != nil {
		return Result{}, err
	}

	if len(posts) == 0 {
		return Result{NewPosts: []Post{}, LastSeen: anchor}, nil
	}

	result := Result{NewPosts: []Post{}, LastSeen: posts[0].Post}

	for _, post := range posts {
		if post.at.After(anchorAt) && isPatchPost(post.Title) {
			result.NewPosts = append(result.NewPosts, post.Post)
		}
	}

	if !anchored && !end {
		return Result{}, fmt.Errorf(
			"news: paged %d feed pages without reaching a post not newer than the anchor %s — "+
				"the feed window may have shifted or the anchor state is stale; "+
				"raise maxFeedPages or refresh %s manually",
			maxFeedPages, anchor.Published, statePath,
		)
	}

	return result, nil
}

// anchor resolves the run's reference point: the state file when it
// exists, otherwise the changelog seed (cold start).
func anchor(statePath, changelogPath string) (post Post, cold bool, err error) {
	data, err := os.ReadFile(statePath)

	if errors.Is(err, fs.ErrNotExist) {
		seed, err := SeedFromChangelog(changelogPath)
		if err != nil {
			return Post{}, false, err
		}

		return seed, true, nil
	}

	if err != nil {
		return Post{}, false, fmt.Errorf("news: read state %s: %w", statePath, err)
	}

	if err := jsonUnmarshalPost(data, &post); err != nil {
		return Post{}, false, fmt.Errorf("news: state %s: %w", statePath, err)
	}

	return post, false, nil
}

// scanFeed pages the feed newest-first until a post not newer than the
// anchor shows up, the feed ends (empty page or a repeated post —
// WordPress redirects out-of-range pages back to page 1), or the page
// cap is hit. anchored reports the first stop, end the second.
func scanFeed(ctx context.Context, feedURL string, anchorAt time.Time, log *slog.Logger) (posts []feedPost, anchored bool, end bool, err error) {
	client := &http.Client{Timeout: httpTimeout}

	seen := map[string]bool{}

	for page := 1; page <= maxFeedPages; page++ {
		items, err := fetchPage(ctx, client, feedURL, page)
		if err != nil {
			return nil, false, false, err
		}

		if len(items) == 0 {
			return posts, anchored, true, nil
		}

		if seen[items[0].URL] {
			return posts, anchored, true, nil
		}

		for _, item := range items {
			if seen[item.URL] {
				continue
			}

			seen[item.URL] = true

			if !item.at.After(anchorAt) {
				return posts, true, false, nil
			}

			posts = append(posts, item)
		}

		log.DebugContext(ctx, "news: feed page scanned", "page", page, "items", len(items))
	}

	return posts, anchored, false, nil
}

// feedPost couples an RSS item with its parsed publication time.
type feedPost struct {
	Post
	at time.Time
}

// fetchPage retrieves and parses one feed page, mapping HTTP failures
// and unparseable XML to loud errors — a broken source must never look
// like an empty result.
func fetchPage(ctx context.Context, client *http.Client, feedURL string, page int) ([]feedPost, error) {
	u, err := url.Parse(feedURL)
	if err != nil {
		return nil, fmt.Errorf("news: feed URL %s: %w", feedURL, err)
	}

	q := u.Query()
	q.Set("paged", strconv.Itoa(page))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("news: feed page %d: %w", page, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("news: fetch feed page %d: %w", page, err)
	}

	// Best-effort close on the read path.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("news: feed page %d: HTTP %d", page, resp.StatusCode)
	}

	var feed rssFeed

	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("news: parse feed page %d: %w", page, err)
	}

	posts := make([]feedPost, 0, len(feed.Channel.Items))

	for _, item := range feed.Channel.Items {
		at, err := parsePubDate(item.PubDate)
		if err != nil {
			return nil, fmt.Errorf("news: feed page %d: item %q pubDate %q: %w", page, item.Title, item.PubDate, err)
		}

		posts = append(posts, feedPost{Post: Post{Title: item.Title, URL: item.Link, Published: at.UTC().Format(time.RFC3339)}, at: at})
	}

	return posts, nil
}

// parsePubDate reads an RSS pubDate (RFC 822 family, WordPress emits
// "+0000" offsets).
func parsePubDate(value string) (time.Time, error) {
	at, err := time.Parse(time.RFC1123Z, value)

	if err == nil {
		return at, nil
	}

	at, err = time.Parse(time.RFC1123, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("not an RFC 822 date: %w", err)
	}

	return at, nil
}

// isPatchPost reports whether a headline is an AoE2 DE patch post:
// the game must be AoE2 (not III — the numeral trap) and the wording
// must be Update/Hotfix family.
func isPatchPost(title string) bool {
	return aoe2Title.MatchString(title) && patchTitle.MatchString(title)
}
