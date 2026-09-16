// Package news detects new AoE2 DE patch posts in the Age of Empires
// news feed. It is a monitoring utility wired into cmd/newscheck and the
// kb-monitor workflow; the LSP server never calls it.
package news

// Post is a news-feed post in detector terms.
type Post struct {
	// Title is the post headline; the patch filter runs against it.
	Title string
	// URL is the permanent post address; a human-readable state key.
	URL string
	// Published is the publication date, RFC 3339, UTC. It is the
	// anchor comparison key: the feed is reverse-chronological, so
	// paging stops at the first post not newer than the anchor date.
	Published string
}

// Result is one detector run outcome.
type Result struct {
	// NewPosts lists AoE2 patch posts strictly newer than the anchor,
	// in feed order (freshest first). Empty means no issue to create.
	NewPosts []Post
	// LastSeen is the newest post examined — the anchor for the next
	// run. The caller passes it to SaveState after the batch issue has
	// been created successfully.
	LastSeen Post
}
