package highlight

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestDirectiveHighlightIncludesUndefineNotCost is the behavioral pin for
// the highlight layer of the include-drs task: #undefine is a directive the
// grammar must color, while #cost must stay a hash-comment (the grammar
// knowingly diverges from rms.directiveWords, which knows #cost as a #const
// typo). It pins both the in-process grammar and the committed artifact.
//
// Only the two directive rules are inspected — never the whole file: the
// kb-constants alternation legitimately contains the substring "cost"
// (terrain_cost).
func TestDirectiveHighlightIncludesUndefineNotCost(t *testing.T) {
	t.Parallel()

	g := buildGrammar(nil, nil, nil)

	artifact := loadCommittedRMSGrammar(t)

	for _, tc := range []struct {
		name  string
		match string
	}{
		{"in-process #directives", g.Repository.Directives.Match},
		{"in-process #comments hash-lookahead", g.Repository.Comments.Patterns[2].(matchRule).Match},
		{"artifact #directives", artifact.Repository.Directives.Match},
		{"artifact #comments hash-lookahead", artifact.Repository.Comments.Patterns[2].Match},
	} {
		if !strings.Contains(tc.match, "undefine") || strings.Contains(tc.match, "cost") {
			t.Errorf("%s: match %q must contain undefine and not cost", tc.name, tc.match)
		}
	}
}

// committedGrammar is the subset of the committed .tmLanguage.json the pin
// reads: the directives rule and the comment sub-rules (heterogeneous in
// the generator, so re-parsed here by their match field alone).
type committedGrammar struct {
	Repository struct {
		Directives struct {
			Match string `json:"match"`
		} `json:"directives"`
		Comments struct {
			Patterns []struct {
				Match string `json:"match"`
			} `json:"patterns"`
		} `json:"comments"`
	} `json:"repository"`
}

// loadCommittedRMSGrammar parses the committed RMS grammar artifact.
func loadCommittedRMSGrammar(t *testing.T) committedGrammar {
	t.Helper()

	raw, err := os.ReadFile(rmsGrammarPath)
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}

	var g committedGrammar
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parse grammar: %v", err)
	}

	return g
}
