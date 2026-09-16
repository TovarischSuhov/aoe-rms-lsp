// Command newscheck runs the AoE2 DE patch-post detector for the
// kb-monitor workflow.
//
// Default mode: run news.Check and print the result as JSON
// ({"new_posts": [...], "last_seen": {...}}) to stdout; -out also
// writes the same JSON to a file.
//
// The -save-from mode reads a result JSON written earlier and persists
// its last_seen anchor via news.SaveState. The workflow calls it only
// after the batch issue has been created, so a failed issue create
// never loses posts.
//
// Exit code 0 marks a successful check (even with no new posts); any
// source failure exits 1 — silent degradation is forbidden.
package main

import (
	"aoe2-lsp/internal/news"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// wire is the workflow-facing result format.
type wire struct {
	NewPosts []news.Post `json:"new_posts"`
	LastSeen news.Post   `json:"last_seen"`
}

func main() {
	feed := flag.String("feed", "https://www.ageofempires.com/news/feed/", "RSS feed address")
	state := flag.String("state", "docs/ref/.newscheck-state.json", "state file path")
	changelog := flag.String("changelog", "docs/ref/aoe2de-xs-rms-changelog.md", "changelog document path (cold-start seed)")
	out := flag.String("out", "", "also write the result JSON to this file")
	saveFrom := flag.String("save-from", "", "persist last_seen from this result JSON and exit")

	flag.Parse()

	if err := run(*feed, *state, *changelog, *out, *saveFrom); err != nil {
		fmt.Fprintf(os.Stderr, "newscheck: %s\n", err)
		os.Exit(1)
	}
}

func run(feed, state, changelog, out, saveFrom string) error {
	if saveFrom != "" {
		return saveAnchor(saveFrom, state)
	}

	result, err := news.Check(context.Background(), feed, state, changelog, nil)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(wire{NewPosts: result.NewPosts, LastSeen: result.LastSeen}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}

	if out != "" {
		if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
	}

	_, err = os.Stdout.Write(append(data, '\n'))

	return err
}

// saveAnchor persists the anchor from a previous check result.
func saveAnchor(saveFrom, state string) error {
	data, err := os.ReadFile(saveFrom)
	if err != nil {
		return fmt.Errorf("read %s: %w", saveFrom, err)
	}

	var result wire

	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode %s: %w", saveFrom, err)
	}

	return news.SaveState(state, result.LastSeen)
}
