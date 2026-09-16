package news

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// changelogFixture mirrors the top of docs/ref/aoe2de-xs-rms-changelog.md.
const changelogFixture = `# AoE2 DE — XS/RMS changelog из release notes
# Регламент пополнения — docs/kb-refresh.md

## Update 177723 — 2026-06-02
https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-177723/

### XS: добавлено
- ` + "`int xsGetLocale()`" + `
`

// item is one fixture feed entry.
type item struct {
	title string
	url   string
	date  string // RFC 822, as WordPress emits
}

func feedPage(items ...item) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel>`)
	b.WriteString("<title>News</title>")

	for _, it := range items {
		b.WriteString("<item><title>")
		b.WriteString(it.title)
		b.WriteString("</title><link>")
		b.WriteString(it.url)
		b.WriteString("</link><pubDate>")
		b.WriteString(it.date)
		b.WriteString("</pubDate></item>")
	}

	b.WriteString("</channel></rss>")

	return b.String()
}

// feedServer serves fixture pages from a generator keyed by ?paged=.
func feedServer(t *testing.T, page func(int) string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("paged"))

		if n < 1 {
			n = 1
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(page(n)))
	}))
}

// pagesFrom adapts a fixed page map: unknown pages are empty (end of feed).
func pagesFrom(fixed map[int]string) func(int) string {
	return func(page int) string {
		if body, ok := fixed[page]; ok {
			return body
		}

		return feedPage()
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestSeedFromChangelog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		want    Post
		wantErr bool
	}{
		{
			name: "top section seeds the anchor",
			body: changelogFixture,
			want: Post{
				Title:     "Update 177723",
				URL:       "https://www.ageofempires.com/news/age-of-empires-ii-definitive-edition-update-177723/",
				Published: "2026-06-02T00:00:00Z",
			},
		},
		{
			name:    "no update section",
			body:    "# unrelated doc\n\n## Not A Section\n",
			wantErr: true,
		},
		{
			name:    "no URL under the section",
			body:    "## Update 177723 — 2026-06-02\n\n### XS\n- nothing\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "changelog.md")
			writeFile(t, path, tt.body)

			got, err := SeedFromChangelog(path)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSeedFromChangelog_Unreadable(t *testing.T) {
	t.Parallel()

	_, err := SeedFromChangelog(filepath.Join(t.TempDir(), "missing.md"))
	require.Error(t, err)
}

func TestCheck(t *testing.T) {
	t.Parallel()

	// Feed stock shared by the scenarios: page 1 is all newer than the
	// anchors used below, page 2 reaches past them.
	page1 := feedPage(
		item{"Age of Empires II: Definitive Edition – Update 178000", "https://x/178000", "Wed, 16 Sep 2026 10:00:00 +0000"},
		item{"Age of Empires III: Definitive Edition – Update 19.18309", "https://x/aoe3", "Tue, 15 Sep 2026 09:00:00 +0000"},
		item{"The Viking Sagas Campaign Preview", "https://x/sagas", "Mon, 14 Sep 2026 08:00:00 +0000"},
		item{"Age of Empires II: Definitive Edition – Hotfix 177990", "https://x/177990", "Sun, 13 Sep 2026 07:00:00 +0000"},
	)
	page2 := feedPage(
		item{"Varangians Civilization Deep Dive", "https://x/varangians", "Sat, 12 Sep 2026 06:00:00 +0000"},
		item{"Age of Empires II: Definitive Edition – Update 177900", "https://x/177900", "Fri, 05 Sep 2026 05:00:00 +0000"},
	)

	tests := []struct {
		name      string
		state     string           // "" — no state file (cold start)
		anchorDay string           // changelog anchor date override; "" — fixture default
		pages     map[int]string   // fixed pages; unknown → empty page
		gen       func(int) string // page generator; overrides pages when set
		wantPosts []string
		wantSeen  string
		wantErr   string
	}{
		{
			name:  "new patch posts with anchor reached on page 2",
			state: `{"Title":"Update 177723","URL":"https://x/177723","Published":"2026-09-10T00:00:00Z"}`,
			pages: map[int]string{1: page1, 2: page2},
			wantPosts: []string{
				"https://x/178000",
				"https://x/177990",
			},
			wantSeen: "https://x/178000",
		},
		{
			name:      "cold start seeds from changelog and scans the whole feed",
			state:     "",
			anchorDay: "",
			pages:     map[int]string{1: page1, 2: feedPage()},
			wantPosts: []string{
				"https://x/178000",
				"https://x/177990",
			},
			wantSeen: "https://x/178000",
		},
		{
			name:      "anchor newer than everything: no posts, anchor holds",
			state:     `{"Title":"Update","URL":"https://x/anchor","Published":"2026-09-20T00:00:00Z"}`,
			pages:     map[int]string{1: page1, 2: page2},
			wantPosts: []string{},
			wantSeen:  "https://x/anchor",
		},
		{
			name:  "page cap exhausted with full distinct pages: loud error",
			state: `{"Title":"Update","URL":"https://x/anchor","Published":"2020-01-01T00:00:00Z"}`,
			pages: nil, // generator below
			gen: func(page int) string {
				items := make([]item, 0, 5)

				for i := range 5 {
					items = append(items, item{
						title: "Age of Empires II: Definitive Edition – Update " + strconv.Itoa(page) + "0" + strconv.Itoa(i),
						url:   "https://x/p" + strconv.Itoa(page) + "-" + strconv.Itoa(i),
						date:  "Wed, 16 Sep 2026 10:00:00 +0000",
					})
				}

				return feedPage(items...)
			},
			wantErr: "paged 10 feed pages",
		},
		{
			name:    "broken XML: loud error",
			state:   `{"Title":"Update","URL":"https://x/anchor","Published":"2026-09-10T00:00:00Z"}`,
			pages:   map[int]string{1: "<rss>not really"},
			wantErr: "parse feed page 1",
		},
		{
			name:    "corrupt state file: loud error, not a cold start",
			state:   "{not json",
			pages:   map[int]string{1: page1},
			wantErr: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			statePath := filepath.Join(dir, ".newscheck-state.json")
			changelogPath := filepath.Join(dir, "changelog.md")
			changelog := changelogFixture

			if tt.anchorDay != "" {
				changelog = strings.Replace(changelogFixture, "2026-06-02", tt.anchorDay, 1)
			}

			writeFile(t, changelogPath, changelog)

			if tt.state != "" {
				writeFile(t, statePath, tt.state)
			}

			gen := tt.gen

			if gen == nil {
				gen = pagesFrom(tt.pages)
			}

			srv := feedServer(t, gen)
			defer srv.Close()

			got, err := Check(context.Background(), srv.URL, statePath, changelogPath, nil)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantSeen, got.LastSeen.URL)

			urls := make([]string, 0, len(got.NewPosts))

			for _, post := range got.NewPosts {
				urls = append(urls, post.URL)
			}

			require.Equal(t, tt.wantPosts, urls)

			// Check never writes state: the file content is the caller's business.
			if tt.state != "" {
				data, readErr := os.ReadFile(statePath)
				require.NoError(t, readErr)
				require.Equal(t, tt.state, string(data))
			} else {
				_, statErr := os.Stat(statePath)
				require.ErrorIs(t, statErr, os.ErrNotExist)
			}
		})
	}
}

func TestCheck_HTTPFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	changelogPath := filepath.Join(dir, "changelog.md")
	writeFile(t, changelogPath, changelogFixture)
	writeFile(t, statePath, `{"Title":"Update","URL":"https://x/anchor","Published":"2026-09-10T00:00:00Z"}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := Check(context.Background(), srv.URL, statePath, changelogPath, nil)
	require.ErrorContains(t, err, "HTTP 503")
}

func TestSaveState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", ".newscheck-state.json")
	post := Post{Title: "Update 178000", URL: "https://x/178000", Published: "2026-09-16T00:00:00Z"}

	require.NoError(t, SaveState(path, post))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), `"URL": "https://x/178000"`)

	var loaded Post
	require.NoError(t, jsonUnmarshalPost(data, &loaded))
	require.Equal(t, post, loaded)
}
