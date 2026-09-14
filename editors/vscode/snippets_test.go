package vscode

import (
	"aoe2-lsp/internal/common"
	"aoe2-lsp/internal/rms"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// snippet is one entry of snippets/aoe2rms.json — only the fields the
// guards below need.
type snippet struct {
	Prefix      string   `json:"prefix"`
	Body        []string `json:"body"`
	Description string   `json:"description"`
}

// TestSnippetsWellFormed keeps every snippet usable: a missing prefix,
// body or description silently drops it from IntelliSense instead of
// failing loudly somewhere in a user's editor.
func TestSnippetsWellFormed(t *testing.T) {
	t.Parallel()
	snippets := loadSnippets(t)
	var problems []string
	for name, s := range snippets {
		switch {
		case s.Prefix == "":
			problems = append(problems, name+": empty prefix")
		case len(s.Body) == 0:
			problems = append(problems, name+": empty body")
		case s.Description == "":
			problems = append(problems, name+": empty description")
		}
	}
	if len(problems) > 0 {
		t.Fatalf("broken snippets:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSkeletonParses expands the map skeleton snippet the way VS Code
// inserts it when the user accepts every placeholder default and runs
// the result through the RMS parser: the skeleton is the first thing a
// new map author inserts, so it must not teach broken syntax.
func TestSkeletonParses(t *testing.T) {
	t.Parallel()
	skeleton := loadSnippets(t)["New map skeleton"]
	expanded := expandDefaults(strings.Join(skeleton.Body, "\n"))
	_, diags := rms.Parse(expanded, "skeleton.rms")
	for _, d := range diags {
		if d.Severity == common.SeverityError {
			t.Fatalf("skeleton snippet does not parse: %s (%s) at %v",
				d.Message, d.Code, d.Range)
		}
	}
}

func loadSnippets(t *testing.T) map[string]snippet {
	t.Helper()
	raw, err := os.ReadFile("snippets/aoe2rms.json")
	if err != nil {
		t.Fatalf("read snippets: %v", err)
	}
	var snippets map[string]snippet
	if err := json.Unmarshal(raw, &snippets); err != nil {
		t.Fatalf("snippets are not valid JSON: %v", err)
	}
	return snippets
}

var (
	choiceRe  = regexp.MustCompile(`\$\{\d+\|([^}]*)\|`)
	defaultRe = regexp.MustCompile(`\$\{\d+:([^}]*)\}`)
	tabStopRe = regexp.MustCompile(`\$\d+`)
)

// expandDefaults renders a snippet body as VS Code inserts it when the
// user accepts every placeholder as-is: named defaults go in verbatim,
// the first choice wins, bare tab stops vanish.
func expandDefaults(body string) string {
	body = choiceRe.ReplaceAllStringFunc(body, func(m string) string {
		choices := choiceRe.FindStringSubmatch(m)[1]
		return strings.SplitN(choices, ",", 2)[0]
	})
	body = defaultRe.ReplaceAllString(body, "$1")
	return tabStopRe.ReplaceAllString(body, "")
}
