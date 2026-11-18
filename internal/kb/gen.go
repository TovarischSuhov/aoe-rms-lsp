package kb

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// callNameRe extracts the callee identifier from a backticked signature.
var callNameRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

// renameRe parses one "### …: переименовано" line: `- `old` → `new`…`.
var renameRe = regexp.MustCompile("^-[ \t]*`([^`]+)`[ \t]*→[ \t]*`([^`]+)`")

// sigRe parses a changelog function signature: `<ret> <name>(<params>)`.
var sigRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^()]*)\)$`)

// paramRe parses one signature parameter: `<type> <name>[ = <default>]`.
var paramRe = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*=\s*([^,)]*))?$`)

// stubCategory marks records synthesized from the changelog alone: the
// guide does not carry the name yet (upstream lag). When the guide
// catches up, its record replaces the stub and since_update survives.
const stubCategory = "changelog"

// renameP is one changelog rename pair: the game rejects `from` and
// resolves `to`.
type renameP struct {
	lang string // "XS" or "RMS" — the heading the pair came from
	from string
	to   string
	id   string // the update that renamed the pair
}

// funcSource is the JSON shape of docs/ref/ugc-guide/xs/functions/functions.json.
type funcSource struct {
	Name       string `json:"name"`
	ReturnType string `json:"return_type"`
	Params     []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Required *bool  `json:"required"`
		Desc     string `json:"desc"`
	} `json:"params"`
	Desc string `json:"desc"`
}

// constSource is the JSON shape of docs/ref/ugc-guide/xs/constants/constants.json.
type constSource struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
	Desc  string          `json:"desc"`
}

// functionJSON mirrors the xs-functions.json schema from the kbdata annotation.
type functionJSON struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	ReturnType  string      `json:"return_type"`
	Params      []paramJSON `json:"params"`
	Desc        string      `json:"desc"`
	SinceUpdate string      `json:"since_update"`
	RenamedTo   string      `json:"renamed_to,omitempty"`
}

// paramJSON is one function parameter in xs-functions.json.
type paramJSON struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Desc     string `json:"desc"`
}

// constantJSON mirrors the xs-constants.json schema from the kbdata annotation.
type constantJSON struct {
	Name        string `json:"name"`
	Section     string `json:"section"`
	Value       string `json:"value"`
	Desc        string `json:"desc"`
	SinceUpdate string `json:"since_update"`
	RenamedTo   string `json:"renamed_to,omitempty"`
}

// commandJSON mirrors the rms-commands.json schema from the kbdata annotation.
type commandJSON struct {
	Name         string    `json:"name"`
	Section      string    `json:"section"`
	Args         []argJSON `json:"args"`
	Attributes   []argJSON `json:"attributes"`
	Desc         string    `json:"desc"`
	GameVersions string    `json:"game_versions"`
	SinceUpdate  string    `json:"since_update"`
	RenamedTo    string    `json:"renamed_to,omitempty"`
}

// argJSON is one command argument or attribute in rms-commands.json.
type argJSON struct {
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Range    ValueRange `json:"range"`
	Required bool       `json:"required"`
	Desc     string     `json:"desc"`
}

// overlayFile is the optional manual-desc overlay next to the guide
// sources: the descriptions the prose extraction cannot reach.
const overlayFile = "attribute-descs.json"

// overlaySource is the JSON shape of the overlay file: descriptions for
// attributes (by name) and positional arguments (by command, positional).
type overlaySource struct {
	Attributes  map[string]string   `json:"attributes"`
	CommandArgs map[string][]string `json:"command_args"`
}

// update is one parsed changelog update: its id and raw body lines.
type update struct {
	id    string
	lines []string
}

// GenKB regenerates the three kb data files (xs-functions.json,
// xs-constants.json, rms-commands.json) from the documentation sources
// under refDir, writing them to dataDir — see the kbdata annotation for
// the layout of both sides.
//
// The Zetnus guide is an optional source (it is not vendored — see
// guideFile): without it GenKB regenerates the XS files but leaves the
// committed rms-commands.json untouched.
//
// It is a one-shot data-build utility; the server never calls it at
// runtime. log receives the skip-WARNs of the extraction pass, the
// desc-coverage report and the summary line; nil selects slog.Default().
// Serialization is deterministic: rerunning on unchanged sources yields
// identical files.
func GenKB(refDir, dataDir string, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}

	changelog, err := os.ReadFile(filepath.Join(refDir, changelogFile))
	if err != nil {
		return fmt.Errorf("read changelog: %w", err)
	}

	updates := parseUpdates(string(changelog))

	functions, err := genFunctions(filepath.Join(refDir, "ugc-guide/xs/functions/functions.json"), updates)
	if err != nil {
		return err
	}

	constants, err := genConstants(filepath.Join(refDir, "ugc-guide/xs/constants/constants.json"), updates)
	if err != nil {
		return err
	}

	commands, err := extractGuideCommands(refDir, log)
	if err != nil {
		return err
	}

	if commands != nil {
		overlay, err := readOverlay(refDir, log)
		if err != nil {
			return err
		}

		applyOverlay(commands, overlay)
		applyCommandRenames(commands, updates)

		cov := descCoverage(commands)

		log.Info("desc coverage",
			"attributes_filled", cov.attrsFilled,
			"attributes_total", cov.attrsTotal,
			"args_filled", cov.argsFilled,
			"args_total", cov.argsTotal,
		)

		for _, name := range cov.emptyNames {
			log.Warn("attribute without desc", "attribute", name)
		}
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}

	if err := writeJSON(filepath.Join(dataDir, "xs-functions.json"), functions); err != nil {
		return err
	}

	if err := writeJSON(filepath.Join(dataDir, "xs-constants.json"), constants); err != nil {
		return err
	}

	if commands != nil {
		wire := make([]commandJSON, 0, len(commands))
		for _, cmd := range commands {
			wire = append(wire, commandJSON{
				Name:         cmd.Name,
				Section:      cmd.Section,
				Args:         argsToJSON(cmd.Args),
				Attributes:   argsToJSON(cmd.Attributes),
				Desc:         cmd.Desc,
				GameVersions: cmd.GameVersions,
				SinceUpdate:  cmd.SinceUpdate,
				RenamedTo:    cmd.RenamedTo,
			})
		}

		if err := writeJSON(filepath.Join(dataDir, "rms-commands.json"), wire); err != nil {
			return err
		}
	}

	log.Info("knowledge base generated",
		"functions", len(functions),
		"constants", len(constants),
		"commands", len(commands),
		"dir", dataDir,
	)

	return nil
}

// extractGuideCommands extracts the RMS commands from the local Zetnus-guide
// export under refDir. The export is not part of the repository (the guide
// ships no explicit license), so a missing file is a normal setup: nil
// commands tell GenKB to leave the committed rms-commands.json alone, and a
// WARN names the path.
func extractGuideCommands(refDir string, log *slog.Logger) ([]Command, error) {
	path := filepath.Join(refDir, guideFile)

	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat zetnus guide: %w", err)
		}

		log.Warn("zetnus guide not found, keeping existing rms-commands.json", "path", path)

		return nil, nil
	}

	commands, err := ExtractRmsCommands(path, log)
	if err != nil {
		return nil, fmt.Errorf("extract rms commands: %w", err)
	}

	return commands, nil
}

// coverageReport is the desc coverage of one generated kb pass. Attributes
// additionally carry the names still without a desc — those are the gaps an
// overlay entry can close by name, unlike the positional argument slots.
type coverageReport struct {
	attrsTotal  int
	attrsFilled int
	argsTotal   int
	argsFilled  int
	emptyNames  []string
}

// readOverlay reads the optional desc overlay of refDir. A missing file is
// a normal setup (guide-only descs), so it yields an empty overlay with a
// single WARN; a broken one aborts the build — silently dropping manual
// descriptions would look like an extraction regression.
func readOverlay(refDir string, log *slog.Logger) (overlaySource, error) {
	overlay := overlaySource{}

	path := filepath.Join(refDir, overlayFile)

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		log.Warn("overlay not found, guide-only descs", "path", path)

		return overlay, nil
	}

	if err != nil {
		return overlaySource{}, fmt.Errorf("read %s: %w", overlayFile, err)
	}

	if err := json.Unmarshal(raw, &overlay); err != nil {
		return overlaySource{}, fmt.Errorf("decode %s: %w", overlayFile, err)
	}

	return overlay, nil
}

// applyOverlay fills only the descs the extraction left empty: attributes
// by name, positional arguments by index with a bounds-checked lookup — a
// shorter overlay list fills its prefix and invents nothing for the rest.
// Every inserted desc goes through mineCommandArg, so bounds prose
// ("number (0-9)") lands in the structured Range exactly as it does on the
// load path and the two pipelines stay idempotent.
func applyOverlay(commands []Command, overlay overlaySource) {
	for ci := range commands {
		cmd := &commands[ci]
		argDescs := overlay.CommandArgs[cmd.Name]

		for i := range cmd.Attributes {
			fill := &cmd.Attributes[i]
			if fill.Desc != "" {
				continue
			}

			if desc, ok := overlay.Attributes[fill.Name]; ok {
				fill.Desc = desc
				mineCommandArg(fill)
			}
		}

		for i := range cmd.Args {
			fill := &cmd.Args[i]
			if fill.Desc != "" || i >= len(argDescs) {
				continue
			}

			fill.Desc = argDescs[i]
			mineCommandArg(fill)
		}
	}
}

// descCoverage counts the desc coverage of the generated data. Totals and
// filled count instances; emptyNames collects the unique attribute names
// without a desc, sorted — argument slots are positional and only counted.
func descCoverage(commands []Command) coverageReport {
	var cov coverageReport

	for _, cmd := range commands {
		for _, arg := range cmd.Attributes {
			cov.attrsTotal++
			if arg.Desc != "" {
				cov.attrsFilled++

				continue
			}

			cov.emptyNames = append(cov.emptyNames, arg.Name)
		}

		for _, arg := range cmd.Args {
			cov.argsTotal++
			if arg.Desc != "" {
				cov.argsFilled++
			}
		}
	}

	slices.Sort(cov.emptyNames)
	cov.emptyNames = slices.Compact(cov.emptyNames)

	return cov
}

// genFunctions adapts the guide's functions.json into the flat
// xs-functions.json shape with changelog enrichment.
func genFunctions(path string, updates []update) ([]functionJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read functions source: %w", err)
	}

	var byCategory map[string][]funcSource
	if err := json.Unmarshal(raw, &byCategory); err != nil {
		return nil, fmt.Errorf("decode functions source: %w", err)
	}

	since := sinceByFunction(updates)

	var out []functionJSON

	for _, category := range sortedKeys(byCategory) {
		fns := byCategory[category]
		for _, fn := range fns {
			params := make([]paramJSON, 0, len(fn.Params))

			for _, p := range fn.Params {
				required := true
				if p.Required != nil {
					required = *p.Required
				}

				params = append(params, paramJSON{
					Name:     p.Name,
					Type:     p.Type,
					Required: required,
					Desc:     p.Desc,
				})
			}

			out = append(out, functionJSON{
				Name:        fn.Name,
				Category:    category,
				ReturnType:  fn.ReturnType,
				Params:      params,
				Desc:        fn.Desc,
				SinceUpdate: since[fn.Name],
			})
		}
	}

	return applyFunctionChangelog(out, updates), nil
}

// applyFunctionChangelog enriches the guide-derived function list with
// the changelog's existence facts and rename pairs:
//
//   - every signature in an additive XS section names a function the
//     game has; a name the guide does not carry yet (upstream lag) gets
//     a stub record — empty desc, parsed signature, since_update from
//     the section. When the guide catches up, its record replaces the
//     stub (guide-wins) and since_update survives;
//   - each "### XS: переименовано" pair marks the old name (a record is
//     synthesized when even the stub list lacks it) with renamed_to —
//     the game rejects the old name — and makes sure the new name
//     exists.
//
// Appended records keep changelog order (deterministic); mutation of
// existing records is by name index, never by pointer, so the append
// growth cannot dangle.
func applyFunctionChangelog(out []functionJSON, updates []update) []functionJSON {
	pos := make(map[string]int, len(out))
	for i := range out {
		pos[out[i].Name] = i
	}

	add := func(fn functionJSON) {
		if _, dup := pos[fn.Name]; dup {
			return
		}

		out = append(out, fn)
		pos[fn.Name] = len(out) - 1
	}

	// existence stubs from the additive sections
	for _, u := range updates {
		inAdded := false

		for _, line := range u.lines {
			if strings.HasPrefix(line, "### ") {
				inAdded = strings.HasPrefix(line, "### XS: добавлено")

				continue
			}

			if !inAdded {
				continue
			}

			for _, span := range backtickRe.FindAllStringSubmatch(line, -1) {
				name, fn, ok := changelogFunction(span[1])
				if !ok {
					continue
				}

				fn.Name = name
				fn.SinceUpdate = u.id
				fn.Category = stubCategory
				add(fn)
			}
		}
	}

	// rename pairs
	for _, r := range parseRenames(updates) {
		if r.lang != "XS" {
			continue
		}

		if i, ok := pos[r.from]; ok {
			out[i].RenamedTo = r.to
			if out[i].SinceUpdate == "" {
				out[i].SinceUpdate = r.id
			}
		} else {
			add(functionJSON{Name: r.from, Category: stubCategory, SinceUpdate: r.id, RenamedTo: r.to})
		}

		// The new name appeared with the rename even when the guide
		// already carries it without a since marker.
		if i, ok := pos[r.to]; ok {
			if out[i].SinceUpdate == "" {
				out[i].SinceUpdate = r.id
			}
		} else {
			add(functionJSON{Name: r.to, Category: stubCategory, SinceUpdate: r.id})
		}
	}

	return out
}

// changelogFunction parses one backticked changelog span into a function
// stub: a full signature (`int hex(string str, int defaultValue)`) gives
// the return type and parameters (defaults mark optional). Bare name
// spans and non-signature spans (prose examples, AI-DSL syntax like
// `use (include "script.xs")`) stay out — only the full signature
// proves a function. ok=false when the span is not a signature.
func changelogFunction(sig string) (string, functionJSON, bool) {
	m := sigRe.FindStringSubmatch(strings.TrimSpace(sig))
	if m == nil {
		return "", functionJSON{}, false
	}

	fn := functionJSON{ReturnType: m[1]}

	if list := strings.TrimSpace(m[3]); list != "" {
		for _, p := range strings.Split(list, ",") {
			pm := paramRe.FindStringSubmatch(p)
			if pm == nil {
				return "", functionJSON{}, false
			}

			fn.Params = append(fn.Params, paramJSON{
				Name:     pm[2],
				Type:     pm[1],
				Required: pm[3] == "",
			})
		}
	}

	return m[2], fn, true
}

// genConstants adapts the guide's constants.json into the flat
// xs-constants.json shape with changelog enrichment.
func genConstants(path string, updates []update) ([]constantJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read constants source: %w", err)
	}

	var bySection map[string][]constSource
	if err := json.Unmarshal(raw, &bySection); err != nil {
		return nil, fmt.Errorf("decode constants source: %w", err)
	}

	since := sinceByConstant(updates, bySection)

	var out []constantJSON

	for _, section := range sortedKeys(bySection) {
		cs := bySection[section]
		for _, c := range cs {
			out = append(out, constantJSON{
				Name:        c.Name,
				Section:     section,
				Value:       rawValue(c.Value),
				Desc:        c.Desc,
				SinceUpdate: since[c.Name],
			})
		}
	}

	return applyConstantRenames(out, updates), nil
}

// applyCommandRenames marks guide commands an "### RMS: переименовано"
// pair covers, in place (no command stubs — the guide extraction owns
// the command set).
func applyCommandRenames(commands []Command, updates []update) {
	for _, r := range parseRenames(updates) {
		if r.lang != "RMS" {
			continue
		}

		for i := range commands {
			if commands[i].Name == r.from {
				commands[i].RenamedTo = r.to

				if commands[i].SinceUpdate == "" {
					commands[i].SinceUpdate = r.id
				}
			}
		}
	}
}

// applyConstantRenames marks guide constants a changelog rename pair
// covers. There are no constant stubs: the additive sections mention
// constants in prose without extractable name/value structure, so the
// additive scan only annotates names the guide already knows
// (sinceByConstant).
func applyConstantRenames(out []constantJSON, updates []update) []constantJSON {
	for _, r := range parseRenames(updates) {
		if r.lang != "XS" {
			continue
		}

		for i := range out {
			if out[i].Name == r.from {
				out[i].RenamedTo = r.to

				if out[i].SinceUpdate == "" {
					out[i].SinceUpdate = r.id
				}
			}
		}
	}

	return out
}

// rawValue renders the source value (number or string) as a string.
func rawValue(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}

	var n json.Number
	if err := json.Unmarshal(v, &n); err == nil {
		return n.String()
	}

	num, err := strconv.ParseFloat(strings.TrimSpace(string(v)), 64)
	if err != nil {
		return string(v)
	}

	return strconv.FormatFloat(num, 'f', -1, 64)
}

// parseUpdates splits the changelog into updates in file order
// (newest first).
func parseUpdates(changelog string) []update {
	var updates []update

	var current *update

	for line := range strings.SplitSeq(changelog, "\n") {
		if m := updateHeadingRe.FindStringSubmatch(line); m != nil {
			updates = append(updates, update{id: m[1]})
			current = &updates[len(updates)-1]

			continue
		}

		if current != nil {
			current.lines = append(current.lines, line)
		}
	}

	return updates
}

// parseRenames collects the "### …: переименовано" pairs across updates
// (newest first, in file order): the game rejects `from` and resolves
// `to`. The heading names the language (XS/RMS) the pairs belong to.
func parseRenames(updates []update) []renameP {
	var out []renameP

	for _, u := range updates {
		lang := ""

		for _, line := range u.lines {
			if strings.HasPrefix(line, "### ") {
				lang = ""

				head, tail, found := strings.Cut(strings.TrimPrefix(line, "### "), ":")
				if found && tail == " переименовано" {
					lang = strings.TrimSpace(head)
				}

				continue
			}

			if lang == "" {
				continue
			}

			if m := renameRe.FindStringSubmatch(line); m != nil {
				out = append(out, renameP{lang: lang, from: m[1], to: m[2], id: u.id})
			}
		}
	}

	return out
}

// sinceByFunction maps function names to the update that added them;
// updates are newest-first, so the last assignment wins (earliest update).
func sinceByFunction(updates []update) map[string]string {
	since := make(map[string]string)

	for _, u := range updates {
		inAdded := false

		for _, line := range u.lines {
			if strings.HasPrefix(line, "### ") {
				inAdded = strings.HasPrefix(line, "### XS: добавлено")

				continue
			}

			if !inAdded {
				continue
			}

			for _, span := range backtickRe.FindAllStringSubmatch(line, -1) {
				if m := callNameRe.FindStringSubmatch(span[1]); m != nil {
					since[m[1]] = u.id
				}
			}
		}
	}

	return since
}

// sinceByConstant maps constant names to the update that mentioned them in
// an additive section (XS additions, trigger/constant or modding notes).
func sinceByConstant(updates []update, bySection map[string][]constSource) map[string]string {
	known := make(map[string]bool)
	for _, cs := range bySection {
		for _, c := range cs {
			known[c.Name] = true
		}
	}

	since := make(map[string]string)

	for _, u := range updates {
		scannable := false

		for _, line := range u.lines {
			if strings.HasPrefix(line, "### ") {
				scannable = strings.Contains(line, "добавлено") ||
					strings.Contains(line, "Триггеры/константы") ||
					strings.Contains(line, "Modding")

				continue
			}

			if !scannable {
				continue
			}

			for _, span := range backtickRe.FindAllStringSubmatch(line, -1) {
				for _, word := range wordRe.FindAllString(span[1], -1) {
					if known[word] {
						since[word] = u.id
					}
				}
			}
		}
	}

	return since
}

// argsToJSON converts command argument specifications to the wire shape.
func argsToJSON(args []CommandArg) []argJSON {
	out := make([]argJSON, 0, len(args))
	for _, a := range args {
		out = append(out, argJSON(a))
	}

	return out
}

// sortedKeys returns the map keys in sorted order so that regeneration is
// deterministic despite Go map iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}

// writeJSON writes one generated file with stable formatting.
func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	raw = append(raw, '\n')

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}
