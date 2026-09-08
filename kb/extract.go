package kb

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// changelogFile is the sibling changelog used for since_update enrichment.
const changelogFile = "aoe2de-xs-rms-changelog.md"

// sectionHeadingRe matches RMS section headings like <LAND_GENERATION>.
var sectionHeadingRe = regexp.MustCompile(`^<([A-Z_]+)>$`)

// docMetadataRe matches the metadata lines that may precede a doc paragraph.
var docMetadataRe = regexp.MustCompile(`^(External reference|Mutually exclusive with|Requires|See also):`)

// argBulletRe matches one "   * Name - description" argument bullet.
var argBulletRe = regexp.MustCompile(`^\*\s+([A-Za-z][A-Za-z0-9_]*|%)\s+-\s+(.*)$`)

// updateHeadingRe matches changelog update headings.
var updateHeadingRe = regexp.MustCompile(`^## Update (\d+)`)

// snakeNameRe matches snake_case (or #-prefixed) identifiers: RMS syntax
// names, as opposed to prose words that start a metadata line.
var snakeNameRe = regexp.MustCompile(`^#?[a-z][a-z0-9_]*$`)

// backtickRe extracts all backticked spans from a line.
var backtickRe = regexp.MustCompile("`([^`]+)`")

// wordRe splits a span into identifier tokens.
var wordRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// ExtractRmsCommands extracts RMS commands from the text export of the
// Zetnus guide (docs/ref/zetnus-rms-guide.txt) into the rms-commands.json
// shape described by the kbdata annotation.
//
// It is a one-shot data-build utility; the server never calls it at runtime.
// Ambiguous fragments are skipped with a WARN to log instead of aborting the
// whole pass; a nil log selects slog.Default(). path points at the guide;
// the changelog used for since_update enrichment is looked up next to it.
func ExtractRmsCommands(path string, log *slog.Logger) ([]Command, error) {
	if log == nil {
		log = slog.Default()
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read zetnus guide: %w", err)
	}

	lines := strings.Split(string(raw), "\n")

	skeleton, err := parseSkeleton(lines, log)
	if err != nil {
		return nil, fmt.Errorf("parse syntax skeleton: %w", err)
	}

	names := make(map[string]bool, len(skeleton))
	for _, sk := range skeleton {
		names[sk.name] = true
	}

	docs := parseReferenceDocs(lines, names)
	since := parseChangelogSince(readChangelog(path, log), names)

	commands := make([]Command, 0, len(skeleton))
	seen := make(map[string]bool, len(skeleton))

	for _, sk := range skeleton {
		if seen[sk.name] {
			return nil, fmt.Errorf("duplicate command name %q", sk.name)
		}

		seen[sk.name] = true
		commands = append(commands, buildCommand(sk, docs[sk.name], since[sk.name], log))
	}

	return commands, nil
}

// skelAttr is one attribute alternative collected from a skeleton block.
type skelAttr struct {
	tokens []string
}

// skelCmd is one command alternative collected from the syntax skeleton.
type skelCmd struct {
	name    string
	section string
	args    []string
	attrs   []skelAttr
}

// parseSkeleton walks the Syntax Skeleton section of the guide and returns
// one skelCmd per command alternative, in file order.
func parseSkeleton(lines []string, log *slog.Logger) ([]skelCmd, error) {
	start := skeletonStart(lines)
	if start < 0 {
		return nil, fmt.Errorf("syntax skeleton heading not found")
	}

	var commands []skelCmd

	var pending []int

	section := ""
	inBlock := false

	for i := start; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if trimmed == "________________" || strings.HasPrefix(trimmed, "Syntax Reference") {
			break
		}

		if strings.HasPrefix(trimmed, "/*") {
			if strings.Contains(trimmed, "global syntax") {
				section = "global"
			}

			continue
		}

		if m := sectionHeadingRe.FindStringSubmatch(trimmed); m != nil {
			section = strings.ToLower(m[1])
			pending = nil
			inBlock = false

			continue
		}

		if trimmed == "{" {
			inBlock = true

			continue
		}

		if trimmed == "}" {
			inBlock = false
			pending = nil

			continue
		}

		if inBlock {
			for _, tokens := range splitAlternatives(strings.Fields(trimmed)) {
				for _, idx := range pending {
					commands[idx].attrs = append(commands[idx].attrs, skelAttr{tokens: tokens})
				}
			}

			continue
		}

		pending = pending[:0]

		for _, tokens := range splitAlternatives(strings.Fields(trimmed)) {
			cmd, ok := newSkelCmd(tokens, section, log)
			if !ok {
				continue
			}

			pending = append(pending, len(commands))
			commands = append(commands, cmd)
		}
	}

	if len(commands) == 0 {
		return nil, fmt.Errorf("no commands found")
	}

	return commands, nil
}

// newSkelCmd builds one command from its skeleton tokens, handling the
// rnd(N,N) functional form. ok=false marks an unparsable fragment.
func newSkelCmd(tokens []string, section string, log *slog.Logger) (skelCmd, bool) {
	if len(tokens) == 1 && strings.Contains(tokens[0], "(") {
		parts := strings.SplitN(strings.ReplaceAll(tokens[0], ")", ""), "(", 2)
		if parts[0] == "" || parts[1] == "" {
			log.Warn("skip ambiguous skeleton line", "line", tokens[0])

			return skelCmd{}, false
		}

		argv := strings.Split(parts[1], ",")

		return skelCmd{name: parts[0], section: section, args: argv}, true
	}

	return skelCmd{name: tokens[0], section: section, args: tokens[1:]}, true
}

// splitAlternatives splits a token stream on "/" separators into one token
// group per alternative (create_player_lands / create_land { ... }).
func splitAlternatives(tokens []string) [][]string {
	var groups [][]string

	current := make([]string, 0, len(tokens))

	for _, tok := range tokens {
		if tok == "/" {
			if len(current) > 0 {
				groups = append(groups, current)
				current = make([]string, 0, len(tokens))
			}

			continue
		}

		current = append(current, tok)
	}

	if len(current) > 0 {
		groups = append(groups, current)
	}

	return groups
}

// skeletonStart locates the Syntax Skeleton heading: the standalone
// "Syntax Skeleton" line followed by an exact <PLAYER_SETUP> line shortly
// after (the table of contents and prose mentions are skipped).
func skeletonStart(lines []string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) != "Syntax Skeleton" {
			continue
		}

		for j := i + 1; j <= i+25 && j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "<PLAYER_SETUP>" {
				return j
			}
		}
	}

	return -1
}

// refDoc is the documentation collected for one command from the Syntax
// Reference chapter.
type refDoc struct {
	gameVersions string
	desc         string
	args         []refArg
}

// refArg is one documented positional argument of a command.
type refArg struct {
	name     string
	desc     string
	required bool
}

// parseReferenceDocs scans the whole guide for command doc blocks: a
// column-0 line starting with a known command name whose next non-blank
// line is the "Game versions:" metadata. Paired commands documented in one
// block (min_/max_ pairs) share the doc of the last signature line.
func parseReferenceDocs(lines []string, known map[string]bool) map[string]refDoc {
	docs := make(map[string]refDoc, len(known))

	for i := range lines {
		if isIndented(lines[i]) {
			continue
		}

		fields := strings.Fields(lines[i])
		if len(fields) == 0 || !known[fields[0]] || !looksLikeSignature(fields[1:]) {
			continue
		}

		group := []int{i}

		for j := i + 1; j < len(lines) && len(group) < 4 && !isIndented(lines[j]); j++ {
			next := strings.Fields(lines[j])
			if len(next) == 0 || !snakeNameRe.MatchString(next[0]) ||
				!looksLikeSignature(next[1:]) || hasColonToken(next[1:]) {
				break
			}

			group = append(group, j)
		}

		sigEnd := group[len(group)-1] + 1
		if !nextNonBlankIsGameVersions(lines, sigEnd) {
			continue
		}

		doc := parseDocBlock(lines, group[len(group)-1])

		for _, idx := range group {
			docs[strings.Fields(lines[idx])[0]] = doc
		}
	}

	return docs
}

// hasColonToken reports whether any token ends with a colon, marking a
// metadata line rather than a signature continuation.
func hasColonToken(tokens []string) bool {
	for _, tok := range tokens {
		if strings.HasSuffix(tok, ":") {
			return true
		}
	}

	return false
}

// looksLikeSignature reports whether the tokens after a command name are
// argument placeholders, not example values. Placeholders are CamelCase
// words, %, block braces, single letters (X, Y) and the skeleton macro
// tokens; numeric literals and ALL-CAPS constants from code examples are
// rejected.
func looksLikeSignature(tokens []string) bool {
	for _, tok := range tokens {
		switch {
		case tok == "%", tok == "{", tok == "}",
			tok == "TYPE", tok == "CONDITION", tok == "FILENAME":
			continue
		case len(tok) == 1 && tok[0] >= 'A' && tok[0] <= 'Z':
			continue
		case strings.ContainsFunc(tok, func(r rune) bool { return r >= 'a' && r <= 'z' }):
			continue
		default:
			return false
		}
	}

	return true
}

// nextNonBlankIsGameVersions reports whether the first non-blank line at
// or after start begins with "Game versions:".
func nextNonBlankIsGameVersions(lines []string, start int) bool {
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}

		return strings.HasPrefix(trimmed, "Game versions:")
	}

	return false
}

// parseDocBlock reads one doc block: metadata lines, the Arguments bullet
// run, then the first plain paragraph as the description.
func parseDocBlock(lines []string, sig int) refDoc {
	var doc refDoc

	j := sig + 1

	for ; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		if trimmed == "" {
			continue
		}

		if v, ok := trimPrefixAny(trimmed, "Game versions:"); ok {
			doc.gameVersions = v

			continue
		}

		if strings.HasPrefix(trimmed, "Arguments:") {
			// parseArgBullets returns the first unconsumed line; step back
			// one so the loop increment lands on it again.
			args, next := parseArgBullets(lines, j+1)
			doc.args = args
			j = next - 1

			continue
		}

		if docMetadataRe.MatchString(trimmed) {
			continue
		}

		break
	}

	var para []string

	for ; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])

		if trimmed == "" {
			if len(para) > 0 {
				break
			}

			continue
		}

		if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "Example") {
			break
		}

		if len(para) > 0 && isIndented(lines[j]) {
			break
		}

		para = append(para, trimmed)
	}

	doc.desc = strings.Join(para, " ")

	return doc
}

// parseArgBullets consumes the "   * Name - description" bullet run after
// an "Arguments:" line. Detail sub-bullets that do not match the
// Name-pattern are skipped; the run ends at the first plain line. It
// returns the collected args and the index of the first unconsumed line.
func parseArgBullets(lines []string, start int) ([]refArg, int) {
	var args []refArg

	for j := start; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])

		if trimmed == "" {
			if len(args) > 0 {
				return args, j
			}

			continue
		}

		if !strings.HasPrefix(trimmed, "*") {
			return args, j
		}

		m := argBulletRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}

		args = append(args, refArg{
			name:     m[1],
			desc:     m[2],
			required: !strings.Contains(m[2], "(default"),
		})
	}

	return args, len(lines)
}

// buildCommand assembles the final Command from skeleton structure,
// reference documentation and changelog enrichment.
func buildCommand(sk skelCmd, doc refDoc, since string, log *slog.Logger) Command {
	cmd := Command{
		Name:         sk.name,
		Section:      sk.section,
		Desc:         doc.desc,
		GameVersions: doc.gameVersions,
		SinceUpdate:  since,
	}

	for i, tok := range sk.args {
		arg := CommandArg{
			Name:     fmt.Sprintf("arg%d", i+1),
			Kind:     kindOf(tok, sk.name, log),
			Required: true,
		}

		if i < len(doc.args) {
			arg.Name = doc.args[i].name
			arg.Desc = doc.args[i].desc
			arg.Required = doc.args[i].required
		}

		// Mining pass (Algorithm step 3): fill the empty Range from the
		// Desc prose; the structured skeleton kind wins over the mined
		// word. Same helper as the load path — idempotent by construction.
		mineCommandArg(&arg)

		cmd.Args = append(cmd.Args, arg)
	}

	for _, attr := range sk.attrs {
		arg := CommandArg{
			Name:     attr.tokens[0],
			Kind:     kindOf(firstOr(attr.tokens, 1, ""), sk.name, log),
			Required: false,
		}

		mineCommandArg(&arg)

		cmd.Attributes = append(cmd.Attributes, arg)
	}

	return cmd
}

// kindOf maps a skeleton placeholder token to the CommandArg kind.
func kindOf(tok string, owner string, log *slog.Logger) string {
	switch tok {
	case "N":
		return "number"
	case "F":
		return "float"
	case "%":
		return "percent"
	case "TYPE":
		return "const"
	case "CONDITION":
		return "condition"
	case "FILENAME":
		return "filename"
	case "":
		return ""
	default:
		log.Warn("unknown argument placeholder", "placeholder", tok, "command", owner)

		return strings.ToLower(tok)
	}
}

// readChangelog reads the changelog sibling of the guide; an empty string
// is returned (with a WARN) when the file is missing.
func readChangelog(guidePath string, log *slog.Logger) string {
	path := filepath.Join(filepath.Dir(guidePath), changelogFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		log.Warn("changelog not found, since_update stays empty", "path", path)

		return ""
	}

	return string(raw)
}

// parseChangelogSince indexes command names to the update that added them.
// Only RMS-section bullets that clearly announce a new command count; the
// changelog is ordered newest-first, so the last assignment wins and yields
// the earliest introducing update.
func parseChangelogSince(changelog string, known map[string]bool) map[string]string {
	since := make(map[string]string)

	update := ""
	inRms := false

	for line := range strings.SplitSeq(changelog, "\n") {
		if m := updateHeadingRe.FindStringSubmatch(line); m != nil {
			update = m[1]
			inRms = false

			continue
		}

		if strings.HasPrefix(line, "### ") {
			inRms = strings.HasPrefix(line, "### RMS")

			continue
		}

		if !inRms || !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			continue
		}

		if !strings.Contains(strings.ToLower(line), "new") {
			continue
		}

		for _, span := range backtickRe.FindAllStringSubmatch(line, -1) {
			for _, word := range wordRe.FindAllString(span[1], -1) {
				if known[word] {
					since[word] = update
				}
			}
		}
	}

	return since
}

// trimPrefixAny trims one of the known prefixes and reports a hit.
func trimPrefixAny(s string, prefixes ...string) (string, bool) {
	for _, p := range prefixes {
		if rest, ok := strings.CutPrefix(s, p); ok {
			return strings.TrimSpace(rest), true
		}
	}

	return "", false
}

// isIndented reports whether the line starts with whitespace.
func isIndented(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t')
}

// firstOr returns the element at idx or fallback when out of range.
func firstOr[T any](items []T, idx int, fallback T) T {
	if idx >= 0 && idx < len(items) {
		return items[idx]
	}

	return fallback
}
