// Package highlight generates the static TextMate grammar for RMS from
// the knowledge base: a hand-written skeleton plus keyword alternations
// rebuilt from the committed kb data. See .usages/grammar-pipeline.md
// for the regeneration workflow; the generated artifact lives at
// editors/vscode/syntaxes/aoe2rms.tmLanguage.json and must never be
// edited by hand — change the skeleton here and regenerate.
package highlight

import (
	"aoe2-lsp/internal/kb"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Scope names of the generated rules. The section scope mirrors the
// server's semantic-token legend (token type "section"); the rest follow
// TextMate conventions so stock themes color them without a custom theme.
const (
	scopeComment     = "comment.block.aoe2rms"
	scopeLineSlash   = "comment.line.double-slash.aoe2rms"
	scopeLineHash    = "comment.line.number-sign.aoe2rms"
	scopeSection     = "entity.name.section.aoe2rms"
	scopeDirective   = "keyword.control.directive.aoe2rms"
	scopeString      = "string.quoted.double.aoe2rms"
	scopeNumber      = "constant.numeric.aoe2rms"
	scopeCommand     = "keyword.other.command.aoe2rms"
	scopeAttribute   = "entity.other.attribute-name.aoe2rms"
	scopeConstant    = "constant.other.aoe2rms"
	grammarSchemaURL = "https://raw.githubusercontent.com/martinring/tmlanguage/master/tmlanguage.json"
)

// directiveRe is the hand-written match for the #-directives; any other
// #-line is a comment (mirrors rms.directiveWords).
const directiveRe = `#(?:includeXS|include_drs|include|const|define)\b`

// GenTmLanguage writes the RMS TextMate grammar to outPath: the
// hand-written skeleton (comments, sections, #-directives, strings,
// numbers) plus keyword alternations for the store's commands,
// attributes and constants.
//
// It is a one-shot build utility; the server never calls it at runtime.
// log receives the summary line; nil selects slog.Default(). The store
// is read-only. Serialization is deterministic: rerunning on the same
// data yields a byte-identical file.
func GenTmLanguage(store *kb.Store, outPath string, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}

	commands, attributes, constants := collectKeywords(store)
	grammar := buildGrammar(commands, attributes, constants)

	raw, err := json.MarshalIndent(grammar, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal grammar: %w", err)
	}

	raw = append(raw, '\n')

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(outPath), err)
	}

	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	log.Info("rms grammar generated",
		"commands", len(commands),
		"attributes", len(attributes),
		"constants", len(constants),
		"path", outPath,
	)

	return nil
}

// collectKeywords gathers the three keyword classes from the store:
// command names, the attribute names of every command, and constant
// names. #-prefixed entries are directives — the skeleton's directives
// rule owns them (a \b anchor would not work in front of #), so they
// stay out of the alternations.
func collectKeywords(store *kb.Store) (commands []string, attributes []string, constants []string) {
	for _, cmd := range store.Commands("") {
		if !strings.HasPrefix(cmd.Name, "#") {
			commands = append(commands, cmd.Name)
		}

		for _, attr := range cmd.Attributes {
			attributes = append(attributes, attr.Name)
		}
	}

	attributes = dedupe(attributes)

	for _, c := range store.Constants("") {
		constants = append(constants, c.Name)
	}

	return commands, attributes, constants
}

// dedupe drops repeat names (the same attribute belongs to many
// commands) keeping first-seen order.
func dedupe(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))

	for _, name := range names {
		if seen[name] {
			continue
		}

		seen[name] = true
		out = append(out, name)
	}

	return out
}

// buildGrammar assembles the full grammar model: the fixed skeleton
// plus one rule per non-empty keyword class.
func buildGrammar(commands []string, attributes []string, constants []string) tmLanguage {
	g := tmLanguage{
		Schema:    grammarSchemaURL,
		Name:      "AoE2 Random Map Script",
		ScopeName: "source.aoe2rms",
		Repository: grammarRepo{
			Comments: commentPatterns{
				Patterns: []any{
					spanRule{Name: scopeComment, Begin: `/\*`, End: `\*/`},
					matchRule{Name: scopeLineSlash, Match: `//.*`},
					// A #-line that is not a directive is a comment.
					matchRule{Name: scopeLineHash, Match: `#(?!(?:includeXS|include_drs|include|const|define)\b).*$`},
				},
			},
			Sections:   matchRule{Name: scopeSection, Match: `(?i)</?[a-z_][a-z0-9_]*>`},
			Directives: matchRule{Name: scopeDirective, Match: directiveRe},
			Strings:    matchRule{Name: scopeString, Match: `"[^"\n]*"`},
			Numbers:    matchRule{Name: scopeNumber, Match: `-?\b\d+(?:\.\d+)?%?`},
		},
	}

	g.Patterns = []includeRule{
		{Include: "#comments"},
		{Include: "#directives"},
		{Include: "#strings"},
		{Include: "#sections"},
		{Include: "#numbers"},
	}

	for _, class := range []struct {
		rule *matchRule
		ref  string
		slot **matchRule
	}{
		{keywordRule(scopeCommand, commands), "#kb-commands", &g.Repository.KbCommands},
		{keywordRule(scopeAttribute, attributes), "#kb-attributes", &g.Repository.KbAttributes},
		{keywordRule(scopeConstant, constants), "#kb-constants", &g.Repository.KbConstants},
	} {
		if class.rule == nil {
			continue
		}

		*class.slot = class.rule
		g.Patterns = append(g.Patterns, includeRule{Include: class.ref})
	}

	return g
}

// keywordRule builds the alternation rule for one keyword class; nil
// when the class is empty — the rule and its include are then omitted.
// Names are case-insensitive and \b-anchored: _ and digits are word
// characters, so create_object does not match create_object_group.
func keywordRule(scope string, names []string) *matchRule {
	if len(names) == 0 {
		return nil
	}

	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}

	return &matchRule{
		Name:  scope,
		Match: `(?i)\b(?:` + strings.Join(quoted, "|") + `)\b`,
	}
}

// tmLanguage is the ordered JSON shape of a .tmLanguage.json document.
// Struct fields fix the key order, which is what makes the artifact
// diff-stable across regenerations.
type tmLanguage struct {
	Schema     string        `json:"$schema"`
	Name       string        `json:"name"`
	ScopeName  string        `json:"scopeName"`
	Patterns   []includeRule `json:"patterns"`
	Repository grammarRepo   `json:"repository"`
}

// includeRule references one repository entry from the patterns list.
type includeRule struct {
	Include string `json:"include"`
}

// matchRule is a single-regex repository rule.
type matchRule struct {
	Name  string `json:"name,omitempty"`
	Match string `json:"match"`
}

// spanRule is a begin/end repository rule (block comments).
type spanRule struct {
	Name  string `json:"name,omitempty"`
	Begin string `json:"begin"`
	End   string `json:"end"`
}

// commentPatterns bundles the comment sub-rules.
type commentPatterns struct {
	Patterns []any `json:"patterns"`
}

// grammarRepo holds the repository; the three kb-* entries drop out
// (with their includes) when their keyword class is empty.
type grammarRepo struct {
	Comments     commentPatterns `json:"comments"`
	Directives   matchRule       `json:"directives"`
	Strings      matchRule       `json:"strings"`
	Sections     matchRule       `json:"sections"`
	Numbers      matchRule       `json:"numbers"`
	KbCommands   *matchRule      `json:"kb-commands,omitempty"`
	KbAttributes *matchRule      `json:"kb-attributes,omitempty"`
	KbConstants  *matchRule      `json:"kb-constants,omitempty"`
}
