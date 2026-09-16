package news

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SaveState atomically writes the anchor post to the state file. The
// file set matches Post, so the format stays stable across versions.
func SaveState(statePath string, post Post) error {
	data, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		return fmt.Errorf("news: encode state %s: %w", statePath, err)
	}

	data = append(data, '\n')

	dir := filepath.Dir(statePath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("news: state dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(statePath)+".*")
	if err != nil {
		return fmt.Errorf("news: state temp %s: %w", statePath, err)
	}

	// Best-effort cleanup: after a successful rename the temp file is
	// already gone and Remove reports ENOENT.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("news: write state temp %s: %w", tmp.Name(), err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("news: close state temp %s: %w", tmp.Name(), err)
	}

	if err := os.Rename(tmp.Name(), statePath); err != nil {
		return fmt.Errorf("news: swap state %s: %w", statePath, err)
	}

	return nil
}

// jsonUnmarshalPost decodes a state file into a Post; a corrupt file is
// a loud error, never an implicit cold start.
func jsonUnmarshalPost(data []byte, post *Post) error {
	if err := json.Unmarshal(data, post); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if post.URL == "" || post.Published == "" {
		return fmt.Errorf("state has empty url or published date")
	}

	return nil
}
