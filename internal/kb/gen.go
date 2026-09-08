package kb

import (
	"encoding/json"
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
}

// argJSON is one command argument or attribute in rms-commands.json.
type argJSON struct {
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Range    ValueRange `json:"range"`
	Required bool       `json:"required"`
	Desc     string     `json:"desc"`
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
// It is a one-shot data-build utility; the server never calls it at
// runtime. log receives the skip-WARNs of the extraction pass and the
// summary line; nil selects slog.Default(). Serialization is
// deterministic: rerunning on unchanged sources yields identical files.
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

	commands, err := ExtractRmsCommands(filepath.Join(refDir, "zetnus-rms-guide.txt"), log)
	if err != nil {
		return fmt.Errorf("extract rms commands: %w", err)
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
		})
	}

	if err := writeJSON(filepath.Join(dataDir, "rms-commands.json"), wire); err != nil {
		return err
	}

	log.Info("knowledge base generated",
		"functions", len(functions),
		"constants", len(constants),
		"commands", len(wire),
		"dir", dataDir,
	)

	return nil
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

	return out, nil
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

	return out, nil
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
