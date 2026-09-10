package highlight

import (
	"aoe2-lsp/internal/kb"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Committed grammar artifacts, relative to the package directory.
const (
	rmsGrammarPath = "../../editors/vscode/syntaxes/aoe2rms.tmLanguage.json"
	xsGrammarPath  = "../../editors/vscode/syntaxes/aoe2xs.tmLanguage.json"
)

// TestGenTmLanguageMatchesCommitted is the no-drift gate: regenerating
// from the current kb data must reproduce the committed artifact
// byte-for-byte.
func TestGenTmLanguageMatchesCommitted(t *testing.T) {
	t.Parallel()

	store, err := kb.NewStore()
	if err != nil {
		t.Fatalf("load knowledge base: %v", err)
	}

	out := filepath.Join(t.TempDir(), "aoe2rms.tmLanguage.json")
	if err := GenTmLanguage(store, out, nil); err != nil {
		t.Fatalf("generate grammar: %v", err)
	}

	generated, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated grammar: %v", err)
	}

	committed, err := os.ReadFile(rmsGrammarPath)
	if err != nil {
		t.Fatalf("read committed grammar: %v", err)
	}

	if !bytes.Equal(generated, committed) {
		t.Errorf("generated grammar drifted from the committed artifact; regenerate with `go run ./cmd/tmgen` and commit the result")
	}
}

// TestGrammarArtifactsValid checks both committed grammars (the
// generated RMS one and the hand-written XS one): valid JSON with a
// scopeName and every #include resolving to a repository entry.
func TestGrammarArtifactsValid(t *testing.T) {
	t.Parallel()

	for _, path := range []string{rmsGrammarPath, xsGrammarPath} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read grammar: %v", err)
			}

			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("grammar is not valid JSON: %v", err)
			}

			var problems []string

			if doc["scopeName"] == nil {
				problems = append(problems, "scopeName is missing")
			}

			repo, _ := doc["repository"].(map[string]any)
			patterns, _ := doc["patterns"].([]any)

			for _, p := range patterns {
				ref, _ := p.(map[string]any)["include"].(string)
				if strings.HasPrefix(ref, "#") && repo[ref[1:]] == nil {
					problems = append(problems, "unresolved include "+ref)
				}
			}

			if len(problems) > 0 {
				t.Errorf("%s is invalid: %s", path, strings.Join(problems, "; "))
			}
		})
	}
}

// TestKeywordRules checks the emitted keyword alternations against the
// contract requirements: kb names match case-insensitively with word
// boundaries that respect underscores and digits.
func TestKeywordRules(t *testing.T) {
	t.Parallel()

	store, err := kb.NewStore()
	if err != nil {
		t.Fatalf("load knowledge base: %v", err)
	}

	commands, attributes, constants := collectKeywords(store)

	for _, tc := range []struct {
		scope  string
		names  []string
		sample string
		want   bool
	}{
		{scopeCommand, commands, "create_object", true},
		{scopeCommand, commands, "  CREATE_OBJECT ", true},
		{scopeCommand, commands, "create_objectx", false},
		{scopeCommand, commands, "xcreate_object", false},
		{scopeAttribute, attributes, "number_of_objects", true},
		{scopeConstant, constants, "cColorBlue", true},
	} {
		rule := keywordRule(tc.scope, tc.names)
		if rule == nil {
			t.Fatalf("keyword class %q is empty", tc.scope)
		}

		re := regexp.MustCompile(rule.Match)
		if got := re.MatchString(tc.sample); got != tc.want {
			t.Errorf("%s: MatchString(%q) = %v, want %v", tc.scope, tc.sample, got, tc.want)
		}
	}
}

// TestBuildGrammarOmitsEmptyClasses: an empty keyword class drops its
// repository rule and its include — not an error.
func TestBuildGrammarOmitsEmptyClasses(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(buildGrammar(nil, nil, nil))
	if err != nil {
		t.Fatalf("marshal grammar: %v", err)
	}

	if strings.Contains(string(raw), "kb-") {
		t.Errorf("empty keyword classes must drop the kb-* rules, got %s", raw)
	}
}
